use bridge_core::agent::CompactionConfig;
use bridge_core::BridgeError;
use llm::providers;
use rig::message::Message;
use tracing::debug;

mod serialize;
#[cfg(test)]
mod tests;
mod tokens;

pub use serialize::serialize_history_for_summary;
pub use tokens::{estimate_tokens, estimate_tokens_fast};

use serialize::is_user_message;

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
