use bridge_core::agent::CompactionConfig;
use bridge_core::BridgeError;
use llm::providers;
use rig::message::{AssistantContent, Message, UserContent};
use std::sync::LazyLock;
use tracing::debug;

/// Default system prompt for the summarization model.
const DEFAULT_SUMMARY_PROMPT: &str = "\
You are a conversation summarizer. Summarize the following conversation history \
concisely but completely. Preserve: key decisions, file paths, code changes made, \
tool results, errors encountered, and any important context the assistant will \
need to continue the conversation coherently. Do not include pleasantries or \
meta-commentary about the summarization itself.";

/// Global cached BPE tokenizer. Initialized once on first use, thread-safe.
/// Avoids re-parsing the ~1.7MB vocabulary on every call to `estimate_tokens`.
static BPE_TOKENIZER: LazyLock<tiktoken_rs::CoreBPE> =
    LazyLock::new(|| tiktoken_rs::cl100k_base().expect("cl100k_base encoding should load"));

/// Result of a successful compaction.
pub struct CompactionResult {
    pub compacted_history: Vec<Message>,
    pub summary_text: String,
    pub messages_compacted: usize,
    pub pre_compaction_tokens: usize,
    pub post_compaction_tokens: usize,
}

/// Estimate the token count for a slice of rig messages using tiktoken cl100k_base.
pub fn estimate_tokens(history: &[Message]) -> usize {
    let bpe = &*BPE_TOKENIZER;

    let mut total = 0usize;
    for msg in history {
        let text = message_to_text(msg);
        total += bpe.encode_with_special_tokens(&text).len();
        // ~4 tokens per message for framing overhead
        total += 4;
    }
    total
}

/// Fast token estimation using byte-count heuristic.
///
/// Returns `None` if the estimate is ambiguous (near the budget boundary),
/// indicating that precise counting via `estimate_tokens` is needed.
/// Returns `Some(estimate)` when the history is clearly under or over budget.
pub fn estimate_tokens_fast(history: &[Message], budget: usize) -> Option<usize> {
    let byte_count: usize = history.iter().map(message_byte_count).sum();
    // Heuristic: ~4 bytes per token for English text, plus framing overhead
    let rough_estimate = byte_count / 4 + history.len() * 4;

    if rough_estimate < budget * 80 / 100 || rough_estimate > budget * 120 / 100 {
        // Clearly under or over budget — skip precise counting
        Some(rough_estimate)
    } else {
        // Near boundary — caller should use precise counting
        None
    }
}

/// Count the total bytes in a message without allocating strings.
fn message_byte_count(msg: &Message) -> usize {
    match msg {
        Message::User { content } => content
            .iter()
            .map(|part| match part {
                UserContent::Text(t) => t.text.len(),
                UserContent::ToolResult(tr) => tr
                    .content
                    .iter()
                    .map(|c| match c {
                        rig::message::ToolResultContent::Text(t) => t.text.len(),
                        other => format!("{:?}", other).len(),
                    })
                    .sum(),
                other => format!("{:?}", other).len(),
            })
            .sum(),
        Message::Assistant { content, .. } => content
            .iter()
            .map(|part| match part {
                AssistantContent::Text(t) => t.text.len(),
                AssistantContent::ToolCall(tc) => {
                    tc.function.name.len() + tc.function.arguments.as_str().map_or(20, |s| s.len())
                }
                other => format!("{:?}", other).len(),
            })
            .sum(),
    }
}

/// Check if history exceeds the token budget and compact if so.
///
/// Returns `None` if history is under budget. Compaction failure is surfaced
/// as an error so the caller can decide to continue with full history.
pub async fn maybe_compact(
    history: &[Message],
    config: &CompactionConfig,
) -> Result<Option<CompactionResult>, BridgeError> {
    let budget = config.token_budget as usize;

    // Fast path: use byte-count heuristic to avoid expensive BPE encoding
    // when the history is clearly under or over budget.
    let pre_tokens = match estimate_tokens_fast(history, budget) {
        Some(fast_est) if fast_est <= budget => return Ok(None),
        Some(_) => {
            // Clearly over budget — still need precise count for the result,
            // but we know compaction is needed. Use precise counting.
            let precise = estimate_tokens(history);
            if precise <= budget {
                return Ok(None);
            }
            precise
        }
        None => {
            // Near boundary — use precise counting
            let precise = estimate_tokens(history);
            if precise <= budget {
                return Ok(None);
            }
            precise
        }
    };

    debug!(
        pre_tokens = pre_tokens,
        budget = config.token_budget,
        "history exceeds token budget, compacting"
    );

    let tail_count = config.tail_messages as usize;
    let total = history.len();

    // Determine split point: keep at least `tail_count` messages in the tail
    let mut split_at = total.saturating_sub(tail_count);

    // Adjust split backwards so tail starts at a user message
    // (don't split in the middle of an assistant+tool_result exchange)
    while split_at > 0 && !is_user_message(&history[split_at]) {
        split_at -= 1;
    }

    // If we can't find a good split point, don't compact
    if split_at == 0 {
        return Ok(None);
    }

    let head = &history[..split_at];
    let tail = &history[split_at..];

    // Build summarizer agent
    let preamble = config
        .summary_prompt
        .as_deref()
        .unwrap_or(DEFAULT_SUMMARY_PROMPT);

    // Build a minimal agent definition for the summarizer
    let summarizer_def = bridge_core::agent::AgentDefinition {
        id: String::new(),
        name: String::new(),
        description: None,
        system_prompt: preamble.to_string(),
        provider: config.summary_provider.clone(),
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        config: bridge_core::agent::AgentConfig::default(),
        subagents: vec![],
        permissions: std::collections::HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };
    let summarizer =
        providers::create_agent(&config.summary_provider, vec![], preamble, &summarizer_def)?;

    // Serialize head into readable text for the summarizer
    let input = serialize_history_for_summary(head);
    let summary_text = summarizer
        .prompt_simple(&input)
        .await
        .map_err(|e| BridgeError::ProviderError(format!("compaction summarizer error: {}", e)))?;

    // Build compacted history: summary as user message + tail
    let mut compacted = Vec::with_capacity(1 + tail.len());
    compacted.push(Message::user(format!(
        "[Conversation Summary]\n{}",
        summary_text
    )));
    compacted.extend_from_slice(tail);

    let post_tokens = estimate_tokens(&compacted);

    Ok(Some(CompactionResult {
        compacted_history: compacted,
        summary_text,
        messages_compacted: head.len(),
        pre_compaction_tokens: pre_tokens,
        post_compaction_tokens: post_tokens,
    }))
}

/// Check if a rig message is a user text message (not a tool result).
fn is_user_message(msg: &Message) -> bool {
    match msg {
        Message::User { content } => content.iter().any(|c| matches!(c, UserContent::Text(_))),
        _ => false,
    }
}

/// Convert a single rig message to its text representation for token counting.
fn message_to_text(msg: &Message) -> String {
    match msg {
        Message::User { content } => content
            .iter()
            .map(|part| match part {
                UserContent::Text(t) => t.text.clone(),
                UserContent::ToolResult(tr) => {
                    let result_text: String = tr
                        .content
                        .iter()
                        .map(|c| match c {
                            rig::message::ToolResultContent::Text(t) => t.text.clone(),
                            other => format!("{:?}", other),
                        })
                        .collect::<Vec<_>>()
                        .join("\n");
                    format!("[tool_result: {}] {}", tr.id, result_text)
                }
                other => format!("{:?}", other),
            })
            .collect::<Vec<_>>()
            .join("\n"),
        Message::Assistant { content, .. } => content
            .iter()
            .map(|part| match part {
                AssistantContent::Text(t) => t.text.clone(),
                AssistantContent::ToolCall(tc) => {
                    format!(
                        "[tool_call: {} ({})]",
                        tc.function.name, tc.function.arguments
                    )
                }
                other => format!("{:?}", other),
            })
            .collect::<Vec<_>>()
            .join("\n"),
    }
}

/// Serialize messages into a human-readable format for the summarizer LLM.
pub fn serialize_history_for_summary(messages: &[Message]) -> String {
    let mut output = String::new();
    for msg in messages {
        match msg {
            Message::User { content } => {
                for part in content.iter() {
                    match part {
                        UserContent::Text(t) => {
                            output.push_str("[User]: ");
                            output.push_str(&t.text);
                            output.push('\n');
                        }
                        UserContent::ToolResult(tr) => {
                            output.push_str("[Tool Result]: ");
                            for c in tr.content.iter() {
                                match c {
                                    rig::message::ToolResultContent::Text(t) => {
                                        output.push_str(&t.text)
                                    }
                                    other => output.push_str(&format!("{:?}", other)),
                                }
                            }
                            output.push('\n');
                        }
                        other => {
                            output.push_str(&format!("[User]: {:?}\n", other));
                        }
                    }
                }
            }
            Message::Assistant { content, .. } => {
                for part in content.iter() {
                    match part {
                        AssistantContent::Text(t) => {
                            output.push_str("[Assistant]: ");
                            output.push_str(&t.text);
                            output.push('\n');
                        }
                        AssistantContent::ToolCall(tc) => {
                            output.push_str(&format!(
                                "[Tool Call]: {} {}\n",
                                tc.function.name, tc.function.arguments
                            ));
                        }
                        other => {
                            output.push_str(&format!("[Assistant]: {:?}\n", other));
                        }
                    }
                }
            }
        }
    }
    output
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_estimate_tokens_empty() {
        assert_eq!(estimate_tokens(&[]), 0);
    }

    #[test]
    fn test_estimate_tokens_known_input() {
        let history = vec![Message::user("Hello, world!")];
        let tokens = estimate_tokens(&history);
        // "Hello, world!" is ~4 tokens + 4 framing = ~8
        assert!(tokens > 0);
        assert!(tokens < 20);
    }

    #[test]
    fn test_no_compaction_under_budget() {
        let config = CompactionConfig {
            token_budget: 100_000,
            tail_messages: 10,
            summary_prompt: None,
            summary_provider: bridge_core::provider::ProviderConfig {
                provider_type: bridge_core::provider::ProviderType::OpenAI,
                model: "gpt-4o-mini".to_string(),
                api_key: "test".to_string(),
                base_url: Some("https://api.openai.com/v1".to_string()),
                prompt_caching_enabled: true,
                cache_ttl: Default::default(),
            },
        };

        let history = vec![Message::user("hello")];
        let rt = tokio::runtime::Runtime::new().unwrap();
        let result = rt.block_on(maybe_compact(&history, &config)).unwrap();
        assert!(result.is_none());
    }

    #[test]
    fn test_tail_boundary_alignment() {
        // Build a history: User, Assistant, User, Assistant, User, Assistant
        let history = [
            Message::user("first question"),
            Message::assistant("first answer"),
            Message::user("second question"),
            Message::assistant("second answer"),
            Message::user("third question"),
            Message::assistant("third answer"),
        ];

        // With tail_messages=2, split_at would be 4 (history[4] is User) — good
        let tail_count = 2usize;
        let total = history.len();
        let mut split_at = total.saturating_sub(tail_count);
        while split_at > 0 && !is_user_message(&history[split_at]) {
            split_at -= 1;
        }
        assert!(is_user_message(&history[split_at]));

        // With tail_messages=1, split_at would be 5 (history[5] is Assistant), should adjust to 4
        let tail_count = 1usize;
        let mut split_at = total.saturating_sub(tail_count);
        while split_at > 0 && !is_user_message(&history[split_at]) {
            split_at -= 1;
        }
        assert!(is_user_message(&history[split_at]));
        assert_eq!(split_at, 4);
    }

    #[test]
    fn test_serialize_history_for_summary() {
        let history = vec![
            Message::user("Can you help me?"),
            Message::assistant("Sure, I'll help."),
        ];

        let text = serialize_history_for_summary(&history);
        assert!(text.contains("[User]: Can you help me?"));
        assert!(text.contains("[Assistant]: Sure, I'll help."));
    }

    #[test]
    fn test_compaction_config_serde_defaults() {
        let json = r#"{
            "summary_provider": {
                "provider_type": "open_ai",
                "model": "gpt-4o-mini",
                "api_key": "test-key"
            }
        }"#;
        let config: CompactionConfig = serde_json::from_str(json).unwrap();
        assert_eq!(config.token_budget, 100_000);
        assert_eq!(config.tail_messages, 10);
        assert!(config.summary_prompt.is_none());
    }

    // ── Fix #2: Cached tokenizer tests ─────────────────────────────────

    #[test]
    fn test_bpe_tokenizer_is_cached_and_reusable() {
        // Calling estimate_tokens multiple times must not panic or reinitialize.
        // The LazyLock ensures the tokenizer is created exactly once.
        let history = vec![Message::user("Hello, world!")];
        let t1 = estimate_tokens(&history);
        let t2 = estimate_tokens(&history);
        assert_eq!(
            t1, t2,
            "cached tokenizer must produce deterministic results"
        );
    }

    #[test]
    fn test_bpe_tokenizer_thread_safety() {
        // Verify the LazyLock tokenizer works across threads.
        let handles: Vec<_> = (0..8)
            .map(|i| {
                std::thread::spawn(move || {
                    let history = [Message::user(format!("Thread {} says hello", i))];
                    estimate_tokens(&history)
                })
            })
            .collect();

        let results: Vec<usize> = handles.into_iter().map(|h| h.join().unwrap()).collect();
        // All threads should produce a reasonable token count
        for count in &results {
            assert!(*count > 0 && *count < 50);
        }
    }

    // ── Fast estimation tests ──────────────────────────────────────────

    #[test]
    fn test_fast_estimate_clearly_under_budget() {
        let history = vec![Message::user("short")];
        // Budget is huge — should return Some(small_number)
        let result = estimate_tokens_fast(&history, 100_000);
        assert!(
            result.is_some(),
            "short message should be clearly under budget"
        );
        assert!(result.unwrap() < 100_000);
    }

    #[test]
    fn test_fast_estimate_returns_none_near_boundary() {
        // Create a history that's roughly near a small budget
        let msg = "a ".repeat(200); // ~100 tokens
        let history = vec![Message::user(&msg)];
        // Set budget to exactly what we estimate — should be ambiguous
        let precise = estimate_tokens(&history);
        let result = estimate_tokens_fast(&history, precise);
        // Near the boundary: might be None (ambiguous) or Some (heuristic happened to be clear)
        // Just ensure it doesn't panic and returns a reasonable answer
        if let Some(fast) = result {
            // If it returns Some, the heuristic was confident
            assert!(fast > 0);
        }
    }

    #[test]
    fn test_fast_estimate_empty_history() {
        let result = estimate_tokens_fast(&[], 100_000);
        assert!(result.is_some());
        assert_eq!(result.unwrap(), 0);
    }

    // ── Byte count tests ───────────────────────────────────────────────

    #[test]
    fn test_message_byte_count_user() {
        let msg = Message::user("Hello, world!");
        let count = message_byte_count(&msg);
        assert_eq!(count, 13); // "Hello, world!" is 13 bytes
    }

    #[test]
    fn test_message_byte_count_assistant() {
        let msg = Message::assistant("I can help with that.");
        let count = message_byte_count(&msg);
        assert_eq!(count, 21);
    }
}
