use rig::message::{AssistantContent, Message, UserContent};
use std::sync::LazyLock;

/// Global cached BPE tokenizer. Initialized once on first use, thread-safe.
/// Avoids re-parsing the ~1.7MB vocabulary on every call to `estimate_tokens`.
static BPE_TOKENIZER: LazyLock<tiktoken_rs::CoreBPE> =
    LazyLock::new(|| tiktoken_rs::cl100k_base().expect("cl100k_base encoding should load"));

/// Estimate the token count for a slice of rig messages using tiktoken cl100k_base.
pub fn estimate_tokens(history: &[Message]) -> usize {
    let bpe = &*BPE_TOKENIZER;

    let mut total = 0usize;
    for msg in history {
        let text = super::serialize::message_to_text(msg);
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
pub(super) fn message_byte_count(msg: &Message) -> usize {
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
