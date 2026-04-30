//! Verifier projection + dispatch.
//!
//! At end-of-turn, project the conversation into a stripped JSON payload
//! and ask a small classifier model whether the agent really finished.
//! `users_turn` and `completed` proceed to finalize as today; `needs_work +
//! high` triggers a synthetic user re-prompt that resumes the same turn.
//!
//! Tool projection deliberately omits names, arguments, outputs, exit
//! codes, and metadata — the verifier sees only the agent's narrative plus
//! a coarse intent log so it can't be fooled by raw tool-output noise.

use std::sync::Arc;
use std::time::Duration;

use bridge_core::agent::VerifierAgentConfig;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use llm::{Confidence, ParsedVerdict, Verdict, VerifierBackend, VerifierClient, VerifierRequest};
use serde::Serialize;
use serde_json::json;
use webhooks::EventBus;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(super) enum VerifierAction {
    /// Verifier said the turn is fine (or errored / disabled / cap hit).
    /// Caller should proceed to `emit_turn_complete_events` and finalize.
    Proceed,
    /// Verifier said `needs_work + high` and the cap is not yet exhausted.
    /// Caller should append a synthetic user message to history and resume
    /// the same turn with `synthetic_prompt` as the rig prompt.
    Reprompt { synthetic_prompt: String },
}

/// Public entry — call once after enforcement, before
/// `emit_turn_complete_events`. Returns [`VerifierAction::Proceed`] when
/// the verifier is disabled, when the cap is exhausted, when the verdict
/// is anything other than `needs_work + high`, or when the verifier
/// errored. Errors never block the turn.
#[allow(clippy::too_many_arguments)]
pub(super) async fn run_if_enabled(
    cfg: Option<&VerifierAgentConfig>,
    client: Option<&VerifierClient>,
    agent_system_prompt: &str,
    enriched_history: &[rig::message::Message],
    history_baseline_len: usize,
    reprompt_count: u32,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
) -> VerifierAction {
    let Some(cfg) = cfg else {
        return VerifierAction::Proceed;
    };
    if !cfg.enabled {
        return VerifierAction::Proceed;
    }
    let Some(client) = client else {
        return VerifierAction::Proceed;
    };

    if reprompt_count >= cfg.max_reprompts_per_turn {
        tracing::info!(
            agent_id = %agent_id,
            conversation_id = %conversation_id,
            reprompt_count,
            cap = cfg.max_reprompts_per_turn,
            "verifier_cap_exhausted — finalizing turn without further verification"
        );
        return VerifierAction::Proceed;
    }

    let projection =
        project_conversation(agent_system_prompt, enriched_history, history_baseline_len);
    let projection_json = match serde_json::to_string(&projection) {
        Ok(s) => s,
        Err(e) => {
            emit_error(
                event_bus,
                agent_id,
                conversation_id,
                &format!("projection serialize: {e}"),
            );
            return VerifierAction::Proceed;
        }
    };

    let user_payload = format!(
        "## Agent system prompt\n{}\n\n## Conversation\n{}",
        agent_system_prompt, projection_json
    );

    let schema_value: serde_json::Value =
        serde_json::from_str(llm::VERIFIER_VERDICT_SCHEMA).expect("frozen verdict schema parses");

    event_bus.emit(BridgeEvent::new(
        BridgeEventType::VerifierStarted,
        agent_id,
        conversation_id,
        json!({ "reprompt_count": reprompt_count }),
    ));

    let req = VerifierRequest {
        system: llm::VERIFIER_SYSTEM_PROMPT,
        schema: &schema_value,
        user: &user_payload,
    };

    let raw = match client.verify(req).await {
        Ok(r) => r,
        Err(e) => {
            emit_error(event_bus, agent_id, conversation_id, &e.to_string());
            return VerifierAction::Proceed;
        }
    };

    let parsed = match ParsedVerdict::parse(&raw.raw_json) {
        Ok(p) => p,
        Err(e) => {
            emit_error(event_bus, agent_id, conversation_id, &format!("parse: {e}"));
            return VerifierAction::Proceed;
        }
    };

    event_bus.emit(BridgeEvent::new(
        BridgeEventType::VerifierVerdict,
        agent_id,
        conversation_id,
        json!({
            "verdict": match parsed.verdict {
                Verdict::UsersTurn => "users_turn",
                Verdict::Completed => "completed",
                Verdict::NeedsWork => "needs_work",
            },
            "confidence": match parsed.confidence {
                Confidence::High => "high",
                Confidence::Low => "low",
            },
            "instruction": parsed.instruction,
            "model": raw.model_used,
            "input_tokens": raw.input_tokens,
            "cached_input_tokens": raw.cached_input_tokens,
            "output_tokens": raw.output_tokens,
            "latency_ms": raw.latency_ms,
            "reprompt_count": reprompt_count,
            "prefix_hash": client.prefix_hash(),
        }),
    ));

    let should_reprompt = matches!(parsed.verdict, Verdict::NeedsWork)
        && (!cfg.require_high_confidence || parsed.confidence == Confidence::High);

    if should_reprompt {
        VerifierAction::Reprompt {
            synthetic_prompt: synthesize_reprompt(&parsed.instruction),
        }
    } else {
        VerifierAction::Proceed
    }
}

fn emit_error(event_bus: &Arc<EventBus>, agent_id: &str, conversation_id: &str, message: &str) {
    tracing::warn!(
        agent_id = %agent_id,
        conversation_id = %conversation_id,
        error = message,
        "verifier_error — proceeding as users_turn"
    );
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::VerifierError,
        agent_id,
        conversation_id,
        json!({ "error": message }),
    ));
}

/// Build a [`VerifierClient`] from config. Returns `None` when the config
/// itself is missing or disabled. Returns `Some(Err(...))` when the config
/// is broken (bad URL, etc.) — callers treat that as `None` (verifier off
/// for this conversation).
pub(super) fn build_client(cfg: &VerifierAgentConfig) -> Option<VerifierClient> {
    if !cfg.enabled {
        return None;
    }
    let backend = match cfg.primary.provider {
        bridge_core::agent::VerifierProvider::OpenAI => VerifierBackend::OpenAI {
            api_key: resolve_env(&cfg.primary.api_key),
            base_url: cfg
                .primary
                .base_url
                .clone()
                .unwrap_or_else(|| "https://api.openai.com/v1".to_string()),
            model: cfg.primary.model.clone(),
        },
        bridge_core::agent::VerifierProvider::Gemini => {
            // Phase 2: Gemini fallback. Phase 1 routes Gemini-as-primary
            // through OpenAI-compatible endpoint if user supplied a base_url
            // pointing at one; otherwise refuse to build.
            let base_url = match &cfg.primary.base_url {
                Some(u) => u.clone(),
                None => {
                    tracing::warn!(
                        "verifier: Gemini primary requires base_url in phase 1; verifier disabled"
                    );
                    return None;
                }
            };
            VerifierBackend::OpenAI {
                api_key: resolve_env(&cfg.primary.api_key),
                base_url,
                model: cfg.primary.model.clone(),
            }
        }
    };
    match VerifierClient::new(
        backend,
        Duration::from_millis(cfg.timeout_ms as u64),
        llm::VERIFIER_SYSTEM_PROMPT,
        llm::VERIFIER_VERDICT_SCHEMA,
    ) {
        Ok(c) => Some(c),
        Err(e) => {
            tracing::warn!(error = %e, "verifier_client_build_failed; verifier disabled for this conversation");
            None
        }
    }
}

/// Resolve `${ENV_VAR}` style placeholders. Anything else is returned as-is.
fn resolve_env(raw: &str) -> String {
    if let Some(rest) = raw.strip_prefix("${") {
        if let Some(name) = rest.strip_suffix('}') {
            return std::env::var(name).unwrap_or_default();
        }
    }
    raw.to_string()
}

/// Synthetic user message appended to history when the verifier asks for
/// more work. The verifier's `instruction` field is the agent-facing
/// directive — it's already phrased as actionable second-person guidance,
/// so we inject it verbatim with a brief framing line. Public for testing.
pub(super) fn synthesize_reprompt(instruction: &str) -> String {
    format!(
        "The previous turn was flagged as incomplete by the verifier.\n\n{}",
        instruction.trim()
    )
}

// ---------------------------------------------------------------------------
// Projection
// ---------------------------------------------------------------------------

#[derive(Serialize, Debug, Clone)]
pub(super) struct VerifierProjection<'a> {
    pub system_prompt_excerpt: &'a str,
    pub messages: Vec<ProjectedMessage>,
}

#[derive(Serialize, Debug, Clone)]
#[serde(tag = "role", rename_all = "snake_case")]
pub(super) enum ProjectedMessage {
    /// Real user-typed text. Passes through verbatim — never elided.
    User { text: String },
    /// Assistant text. Head/tail elided when long.
    Assistant { text: String },
    /// Tool call requested by the agent. `arguments` is the args JSON
    /// serialized as a string and head/tail elided when long.
    ToolCall { name: String, arguments: String },
    /// Tool result. `content` is the result body head/tail elided when long.
    ToolResult { content: String },
}

/// Per-side character budget for head/tail elision applied to every
/// projected field except [`ProjectedMessage::User::text`]. A string of
/// length L is sent as `head[..N] + ELISION_MARKER + tail[L-N..]`
/// when L > 2N + ELISION_MARKER.len(); otherwise verbatim.
const HEAD_TAIL_CHARS: usize = 1000;
const ELISION_MARKER: &str = "\n\n... [middle elided] ...\n\n";

/// Walk the rig history and produce the verifier wire format. User text is
/// preserved verbatim. Assistant text, tool-call arguments, and tool
/// results are head/tail-elided. Order is preserved: each rig content
/// block becomes one projected message.
pub(super) fn project_conversation<'a>(
    agent_system_prompt: &'a str,
    history: &[rig::message::Message],
    _history_baseline_len: usize,
) -> VerifierProjection<'a> {
    use rig::completion::message::{AssistantContent, UserContent};

    let mut messages = Vec::with_capacity(history.len());

    for msg in history {
        match msg {
            rig::message::Message::User { content } => {
                for part in content.iter() {
                    match part {
                        UserContent::Text(t) if !t.text.is_empty() => {
                            messages.push(ProjectedMessage::User {
                                text: t.text.clone(),
                            });
                        }
                        UserContent::ToolResult(result) => {
                            let body = result
                                .content
                                .iter()
                                .map(|p| match p {
                                    rig::message::ToolResultContent::Text(t) => t.text.clone(),
                                    other => format!("{other:?}"),
                                })
                                .collect::<Vec<_>>()
                                .join("\n");
                            messages.push(ProjectedMessage::ToolResult {
                                content: head_tail_elide(&body, HEAD_TAIL_CHARS),
                            });
                        }
                        _ => {}
                    }
                }
            }
            rig::message::Message::Assistant { content, .. } => {
                for part in content.iter() {
                    match part {
                        AssistantContent::Text(t) if !t.text.is_empty() => {
                            messages.push(ProjectedMessage::Assistant {
                                text: head_tail_elide(&t.text, HEAD_TAIL_CHARS),
                            });
                        }
                        AssistantContent::ToolCall(call) => {
                            let args_str = serde_json::to_string(&call.function.arguments)
                                .unwrap_or_else(|_| String::from("\"<unserializable>\""));
                            messages.push(ProjectedMessage::ToolCall {
                                name: call.function.name.clone(),
                                arguments: head_tail_elide(&args_str, HEAD_TAIL_CHARS),
                            });
                        }
                        _ => {}
                    }
                }
            }
            rig::message::Message::System { .. } => {}
        }
    }

    VerifierProjection {
        system_prompt_excerpt: agent_system_prompt,
        messages,
    }
}

/// Return `s` verbatim when short enough; otherwise the first `n_per_side`
/// chars + [`ELISION_MARKER`] + the last `n_per_side` chars. Char-aware so
/// multi-byte UTF-8 codepoints are never split.
fn head_tail_elide(s: &str, n_per_side: usize) -> String {
    let total_chars = s.chars().count();
    let marker_chars = ELISION_MARKER.chars().count();
    if total_chars <= 2 * n_per_side + marker_chars {
        return s.to_string();
    }
    let head: String = s.chars().take(n_per_side).collect();
    let tail: String = s.chars().skip(total_chars - n_per_side).collect();
    format!("{head}{ELISION_MARKER}{tail}")
}

#[cfg(test)]
mod tests {
    use super::*;
    use rig::completion::message::{AssistantContent, UserContent};
    use rig::message::Message as RigMessage;
    use rig::OneOrMany;

    fn user(text: &str) -> RigMessage {
        RigMessage::User {
            content: OneOrMany::one(UserContent::text(text)),
        }
    }

    fn assistant_text(text: &str) -> RigMessage {
        RigMessage::Assistant {
            id: None,
            content: OneOrMany::one(AssistantContent::text(text)),
        }
    }

    fn assistant_tool_call(name: &str, args: serde_json::Value) -> RigMessage {
        RigMessage::Assistant {
            id: None,
            content: OneOrMany::one(AssistantContent::tool_call(
                "call-1".to_string(),
                name.to_string(),
                args,
            )),
        }
    }

    fn user_tool_result(id: &str, body: &str) -> RigMessage {
        RigMessage::User {
            content: OneOrMany::one(UserContent::tool_result(
                id.to_string(),
                OneOrMany::one(rig::message::ToolResultContent::text(body)),
            )),
        }
    }

    #[test]
    fn project_includes_tool_calls_and_results_in_order() {
        let history = vec![
            user("read file foo.txt"),
            assistant_tool_call("Read", serde_json::json!({"path":"/tmp/foo.txt"})),
            user_tool_result("tr-1", "line a\nline b"),
            assistant_text("done"),
        ];
        let proj = project_conversation("sys", &history, 0);
        assert_eq!(proj.messages.len(), 4);
        assert!(matches!(&proj.messages[0], ProjectedMessage::User { .. }));
        match &proj.messages[1] {
            ProjectedMessage::ToolCall { name, arguments } => {
                assert_eq!(name, "Read");
                assert!(arguments.contains("/tmp/foo.txt"));
            }
            _ => panic!("expected ToolCall"),
        }
        match &proj.messages[2] {
            ProjectedMessage::ToolResult { content } => assert!(content.contains("line a")),
            _ => panic!("expected ToolResult"),
        }
        assert!(matches!(&proj.messages[3], ProjectedMessage::Assistant { .. }));
    }

    #[test]
    fn assistant_short_text_passes_through_verbatim() {
        let history = vec![assistant_text("done")];
        let proj = project_conversation("sys", &history, 0);
        match &proj.messages[0] {
            ProjectedMessage::Assistant { text } => assert_eq!(text, "done"),
            _ => panic!(),
        }
    }

    #[test]
    fn assistant_long_text_is_head_tail_elided() {
        let head = "H".repeat(HEAD_TAIL_CHARS);
        let middle = "M".repeat(5_000);
        let tail = "T".repeat(HEAD_TAIL_CHARS);
        let full = format!("{head}{middle}{tail}");
        let history = vec![assistant_text(&full)];
        let proj = project_conversation("sys", &history, 0);
        match &proj.messages[0] {
            ProjectedMessage::Assistant { text } => {
                assert!(text.starts_with(&head));
                assert!(text.ends_with(&tail));
                assert!(text.contains(ELISION_MARKER));
                assert!(!text.contains("MMMMMMMMMM"));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn tool_call_args_are_head_tail_elided() {
        let big = "x".repeat(5_000);
        let history = vec![assistant_tool_call(
            "Write",
            serde_json::json!({"path":"/repo/foo.rs","content":big}),
        )];
        let proj = project_conversation("sys", &history, 0);
        match &proj.messages[0] {
            ProjectedMessage::ToolCall { name, arguments } => {
                assert_eq!(name, "Write");
                assert!(arguments.starts_with('{'));
                assert!(arguments.contains(ELISION_MARKER));
                assert!(arguments.ends_with('}'));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn tool_result_content_is_head_tail_elided() {
        let big = "y".repeat(5_000);
        let history = vec![user_tool_result("id", &big)];
        let proj = project_conversation("sys", &history, 0);
        match &proj.messages[0] {
            ProjectedMessage::ToolResult { content } => {
                assert!(content.contains(ELISION_MARKER));
            }
            _ => panic!(),
        }
    }

    #[test]
    fn user_text_is_never_truncated() {
        let big = "U".repeat(100_000);
        let history = vec![user(&big)];
        let proj = project_conversation("sys", &history, 0);
        match &proj.messages[0] {
            ProjectedMessage::User { text } => assert_eq!(text.len(), big.len()),
            _ => panic!(),
        }
    }

    #[test]
    fn synthesize_reprompt_uses_instruction_verbatim() {
        let s = synthesize_reprompt(
            "Run the failing test in foo_test.rs and update the assertion to match.",
        );
        assert!(s.contains("foo_test.rs"));
        assert!(s.contains("update the assertion"));
        assert!(s.contains("flagged as incomplete"));
    }

    #[test]
    fn resolve_env_supports_placeholder() {
        std::env::set_var("BRIDGE_VERIFIER_TEST_KEY", "abc");
        assert_eq!(resolve_env("${BRIDGE_VERIFIER_TEST_KEY}"), "abc");
        assert_eq!(resolve_env("plain-value"), "plain-value");
    }
}
