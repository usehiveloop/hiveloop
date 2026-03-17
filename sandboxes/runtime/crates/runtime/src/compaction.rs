use bridge_core::agent::CompactionConfig;
use bridge_core::BridgeError;
use llm::create_agent_builder;
use rig::completion::Prompt;
use rig::message::{AssistantContent, Message, UserContent};
use tracing::debug;

/// Default system prompt for the summarization model.
const DEFAULT_SUMMARY_PROMPT: &str = "\
You are a conversation summarizer. Summarize the following conversation history \
concisely but completely. Preserve: key decisions, file paths, code changes made, \
tool results, errors encountered, and any important context the assistant will \
need to continue the conversation coherently. Do not include pleasantries or \
meta-commentary about the summarization itself.";

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
    let bpe = tiktoken_rs::cl100k_base().expect("cl100k_base encoding should load");

    let mut total = 0usize;
    for msg in history {
        let text = message_to_text(msg);
        total += bpe.encode_with_special_tokens(&text).len();
        // ~4 tokens per message for framing overhead
        total += 4;
    }
    total
}

/// Check if history exceeds the token budget and compact if so.
///
/// Returns `None` if history is under budget. Compaction failure is surfaced
/// as an error so the caller can decide to continue with full history.
pub async fn maybe_compact(
    history: &[Message],
    config: &CompactionConfig,
) -> Result<Option<CompactionResult>, BridgeError> {
    let pre_tokens = estimate_tokens(history);

    if pre_tokens <= config.token_budget as usize {
        return Ok(None);
    }

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

    let builder = create_agent_builder(&config.summary_provider)?;
    let summarizer = builder.preamble(preamble).build();

    // Serialize head into readable text for the summarizer
    let input = serialize_history_for_summary(head);
    let summary_text = summarizer
        .prompt(&input)
        .await
        .map_err(|e| BridgeError::ProviderError(format!("compaction summarizer error: {}", e)))?;

    // Build compacted history: summary as user message + tail
    let mut compacted = Vec::with_capacity(1 + tail.len());
    compacted.push(Message::user(&format!(
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
        let history = vec![
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
}
