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
            "reason": parsed.reason,
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
            synthetic_prompt: synthesize_reprompt(&parsed.reason),
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
/// more work. Public for testing.
pub(super) fn synthesize_reprompt(reason: &str) -> String {
    format!(
        "The verifier flagged this turn as potentially incomplete: {}\nRe-check your work. \
        If you've genuinely finished, say so explicitly. Otherwise continue the task.",
        reason.trim()
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
    User {
        text: String,
    },
    Assistant {
        text: Option<String>,
        tool_intents: Vec<ToolIntent>,
    },
}

#[derive(Serialize, Debug, Clone)]
pub(super) struct ToolIntent {
    pub action: ToolAction,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target: Option<String>,
}

#[derive(Serialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub(super) enum ToolAction {
    Read,
    Write,
    Execute,
    Network,
    AskUser,
    Other,
}

const TARGET_MAX_LEN: usize = 32;

/// Project a rig history slice into the verifier wire format. Only the
/// suffix from `history_baseline_len` is included — that's just-this-turn.
/// User messages from the broader conversation matter less for terminal
/// verdict; the most recent user message is the one that comes through
/// `commit_user_turn`, which writes into history *before* the baseline,
/// so we walk the whole history but project all user/assistant text.
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
                let mut text_parts: Vec<String> = Vec::new();
                for part in content.iter() {
                    if let UserContent::Text(t) = part {
                        if !t.text.is_empty() {
                            text_parts.push(t.text.clone());
                        }
                    }
                    // ToolResult intentionally dropped.
                }
                if !text_parts.is_empty() {
                    messages.push(ProjectedMessage::User {
                        text: text_parts.join("\n"),
                    });
                }
            }
            rig::message::Message::Assistant { content, .. } => {
                let mut text_parts: Vec<String> = Vec::new();
                let mut intents: Vec<ToolIntent> = Vec::new();
                for part in content.iter() {
                    match part {
                        AssistantContent::Text(t) if !t.text.is_empty() => {
                            text_parts.push(t.text.clone());
                        }
                        AssistantContent::ToolCall(call) => {
                            intents.push(intent_for_tool(
                                &call.function.name,
                                &call.function.arguments,
                            ));
                        }
                        _ => {}
                    }
                }
                let text = if text_parts.is_empty() {
                    None
                } else {
                    Some(text_parts.join("\n"))
                };
                if text.is_some() || !intents.is_empty() {
                    messages.push(ProjectedMessage::Assistant {
                        text,
                        tool_intents: intents,
                    });
                }
            }
            rig::message::Message::System { .. } => {} // never projected here
        }
    }

    VerifierProjection {
        system_prompt_excerpt: agent_system_prompt,
        messages,
    }
}

/// Map a tool call to a coarse intent. Never includes tool name, full
/// arguments, or output. Target is a single short noun phrase, capped to
/// [`TARGET_MAX_LEN`] chars.
fn intent_for_tool(name: &str, args: &serde_json::Value) -> ToolIntent {
    let lower = name.to_ascii_lowercase();
    // Order matters: Execute is checked before Read so "killshell" is not
    // misclassified by a substring "ls" match.
    let action = if matches_token(
        &lower,
        &["bash", "bashoutput", "killshell", "shell", "exec"],
    ) {
        ToolAction::Execute
    } else if matches_token(
        &lower,
        &["write", "edit", "multiedit", "notebookedit", "create_file"],
    ) {
        ToolAction::Write
    } else if matches_token(
        &lower,
        &["read", "ls", "glob", "ripgrep", "rip_grep", "grep"],
    ) {
        ToolAction::Read
    } else if matches_token(&lower, &["webfetch", "websearch", "fetch", "http"]) {
        ToolAction::Network
    } else if matches_token(&lower, &["askuser", "ask_user_question"]) {
        ToolAction::AskUser
    } else {
        ToolAction::Other
    };

    let target = match action {
        ToolAction::Read | ToolAction::Write => path_target(args),
        ToolAction::Execute => command_target(args),
        ToolAction::Network => url_or_query_target(args),
        ToolAction::AskUser | ToolAction::Other => None,
    };
    let target = target.map(|s| sanitize_target(&s));

    ToolIntent { action, target }
}

/// Match `haystack` (already lowercased) against any of `tokens` either as
/// the whole identifier or as an underscore-separated component. Avoids
/// substring false positives like "killshell" matching "ls".
fn matches_token(haystack: &str, tokens: &[&str]) -> bool {
    tokens
        .iter()
        .any(|t| haystack == *t || haystack.split(['_', '-']).any(|p| p == *t))
}

fn path_target(args: &serde_json::Value) -> Option<String> {
    let p = args
        .get("path")
        .or_else(|| args.get("file_path"))
        .or_else(|| args.get("pattern"))
        .and_then(|v| v.as_str())?;
    Some(basename(p).to_string())
}

fn basename(p: &str) -> &str {
    let trimmed = p.trim_end_matches('/');
    match trimmed.rsplit_once('/') {
        Some((_, last)) if !last.is_empty() => last,
        _ => trimmed,
    }
}

fn command_target(args: &serde_json::Value) -> Option<String> {
    let cmd = args.get("command").and_then(|v| v.as_str())?;
    Some(cmd.split_whitespace().next().unwrap_or("").to_string())
}

fn url_or_query_target(args: &serde_json::Value) -> Option<String> {
    if let Some(u) = args.get("url").and_then(|v| v.as_str()) {
        // Take host (between :// and the first / or end).
        if let Some(rest) = u.split("://").nth(1) {
            return Some(rest.split('/').next().unwrap_or("").to_string());
        }
        return Some(u.to_string());
    }
    args.get("query")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
}

fn sanitize_target(s: &str) -> String {
    let cleaned: String = s
        .chars()
        .filter(|c| !c.is_control())
        .take(TARGET_MAX_LEN)
        .collect();
    cleaned.trim().to_string()
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

    #[test]
    fn project_drops_tool_results_and_keeps_intents() {
        let history = vec![
            user("read file foo.txt"),
            assistant_tool_call("Read", serde_json::json!({"path":"/tmp/foo.txt"})),
            // tool result message would normally appear here as a User
            // ToolResult — we simulate by skipping it from history; the
            // important assertion is that even if present it's not
            // surfaced to the verifier.
            assistant_text("done"),
        ];
        let proj = project_conversation("sys", &history, 0);
        assert_eq!(proj.messages.len(), 3);
        match &proj.messages[1] {
            ProjectedMessage::Assistant { text, tool_intents } => {
                assert!(text.is_none());
                assert_eq!(tool_intents.len(), 1);
                assert_eq!(tool_intents[0].action, ToolAction::Read);
                assert_eq!(tool_intents[0].target.as_deref(), Some("foo.txt"));
            }
            _ => panic!("expected assistant message"),
        }
    }

    #[test]
    fn projection_serializes_without_tool_names_or_args() {
        let history = vec![assistant_tool_call(
            "Bash",
            serde_json::json!({"command":"rm -rf /tmp/secret","reasoning":"private"}),
        )];
        let proj = project_conversation("sys", &history, 0);
        let json = serde_json::to_string(&proj).unwrap();
        // The literal tool name "Bash" can appear lowercased as part of the
        // action mapping, but the args / sensitive path must not surface.
        assert!(!json.contains("/tmp/secret"), "full path leaked: {json}");
        assert!(!json.contains("private"), "extra args leaked: {json}");
        assert!(!json.contains("reasoning"), "extra args leaked: {json}");
        assert!(json.contains("execute"), "action missing: {json}");
        assert!(json.contains("rm"), "command basename present: {json}");
    }

    #[test]
    fn intent_mapping_table() {
        let cases: Vec<(&str, ToolAction)> = vec![
            ("Read", ToolAction::Read),
            ("rip_grep", ToolAction::Read),
            ("Write", ToolAction::Write),
            ("Edit", ToolAction::Write),
            ("MultiEdit", ToolAction::Write),
            ("Bash", ToolAction::Execute),
            ("KillShell", ToolAction::Execute),
            ("WebFetch", ToolAction::Network),
            ("ask_user_question", ToolAction::AskUser),
            ("InternalCustomMcpThing", ToolAction::Other),
        ];
        for (name, expected) in cases {
            let i = intent_for_tool(name, &serde_json::json!({}));
            assert_eq!(i.action, expected, "for tool {name}");
        }
    }

    #[test]
    fn target_is_capped() {
        let long = "a".repeat(200);
        let i = intent_for_tool(
            "Read",
            &serde_json::json!({ "path": format!("/x/{}", long) }),
        );
        assert!(i.target.unwrap().len() <= TARGET_MAX_LEN);
    }

    #[test]
    fn synthesize_reprompt_format() {
        let s = synthesize_reprompt("missing tests");
        assert!(s.contains("missing tests"));
        assert!(s.contains("Re-check"));
    }

    #[test]
    fn resolve_env_supports_placeholder() {
        std::env::set_var("BRIDGE_VERIFIER_TEST_KEY", "abc");
        assert_eq!(resolve_env("${BRIDGE_VERIFIER_TEST_KEY}"), "abc");
        assert_eq!(resolve_env("plain-value"), "plain-value");
    }
}
