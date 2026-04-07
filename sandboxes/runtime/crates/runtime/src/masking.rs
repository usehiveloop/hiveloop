use rig::message::{Message, ToolResultContent, UserContent};
use rig::one_or_many::OneOrMany;

/// Protection budget: tool outputs within this byte budget (most recent first) are preserved.
/// Outputs beyond this window are replaced with compact placeholders.
const DEFAULT_PROTECTION_BYTES: usize = 30 * 1024; // 30KB

/// Minimum size for a tool result to be considered for masking.
/// Tiny results aren't worth masking — the placeholder would be nearly as large.
const MIN_MASKABLE_BYTES: usize = 200;

/// Tools whose output should never be masked.
/// These are semantic/metadata tools, not data-producing tools.
const EXEMPT_TOOLS: &[&str] = &["journal_read", "journal_write", "todoread", "todowrite"];

/// Mask old tool outputs in conversation history to reduce context pressure.
///
/// Walks backward through history, protecting the most recent `protection_budget`
/// bytes of tool output. Older tool outputs beyond the protection window are
/// replaced with compact placeholders like `[Tool output masked — 12345 bytes]`.
///
/// This runs before each turn's budget check (compaction or immortal chain) to
/// reduce unnecessary context consumption from stale tool results.
pub fn mask_old_tool_outputs(history: &mut [Message], protection_budget: usize) {
    // Phase 1: Walk backward, identify which message indices to mask
    let mut protected_bytes: usize = 0;
    let mut should_mask: std::collections::HashSet<usize> = std::collections::HashSet::new();

    for msg_idx in (0..history.len()).rev() {
        let content = match &history[msg_idx] {
            Message::User { content } => content,
            _ => continue,
        };

        for part in content.iter() {
            let tr = match part {
                UserContent::ToolResult(tr) => tr,
                _ => continue,
            };

            let text_bytes = tool_result_byte_count(tr);

            if text_bytes < MIN_MASKABLE_BYTES {
                continue;
            }

            if is_exempt(&tr.id) {
                continue;
            }

            if protected_bytes + text_bytes <= protection_budget {
                protected_bytes += text_bytes;
            } else {
                should_mask.insert(msg_idx);
            }
        }
    }

    // Phase 2: Rebuild masked messages
    for msg_idx in &should_mask {
        let new_msg = {
            let content = match &history[*msg_idx] {
                Message::User { content } => content,
                _ => continue,
            };

            let new_parts: Vec<UserContent> = content
                .iter()
                .map(|part| match part {
                    UserContent::ToolResult(tr) => {
                        let bytes = tool_result_byte_count(tr);
                        if bytes >= MIN_MASKABLE_BYTES && !is_exempt(&tr.id) {
                            UserContent::ToolResult(rig::message::ToolResult {
                                id: tr.id.clone(),
                                call_id: tr.call_id.clone(),
                                content: OneOrMany::one(ToolResultContent::Text(
                                    rig::message::Text {
                                        text: format!("[Tool output masked — {} bytes]", bytes),
                                    },
                                )),
                            })
                        } else {
                            part.clone()
                        }
                    }
                    other => other.clone(),
                })
                .collect();

            match OneOrMany::many(new_parts) {
                Ok(new_content) => Some(Message::User {
                    content: new_content,
                }),
                Err(_) => None,
            }
        };

        if let Some(msg) = new_msg {
            history[*msg_idx] = msg;
        }
    }
}

/// Mask old tool outputs using the default protection budget.
pub fn mask_old_tool_outputs_default(history: &mut [Message]) {
    mask_old_tool_outputs(history, DEFAULT_PROTECTION_BYTES);
}

/// Count the total bytes in a ToolResult's text content.
fn tool_result_byte_count(tr: &rig::message::ToolResult) -> usize {
    tr.content
        .iter()
        .map(|c| match c {
            ToolResultContent::Text(t) => t.text.len(),
            other => format!("{:?}", other).len(),
        })
        .sum()
}

/// Check if a tool result is from an exempt tool.
/// We check the tool result `id` field which typically contains the tool name
/// or call identifier.
fn is_exempt(tool_id: &str) -> bool {
    EXEMPT_TOOLS.iter().any(|name| tool_id.contains(name))
}

#[cfg(test)]
mod tests {
    use super::*;
    use rig::message::Text;

    fn make_user_with_tool_result(id: &str, text_content: &str) -> Message {
        Message::User {
            content: OneOrMany::one(UserContent::ToolResult(rig::message::ToolResult {
                id: id.to_string(),
                call_id: None,
                content: OneOrMany::one(ToolResultContent::Text(Text {
                    text: text_content.to_string(),
                })),
            })),
        }
    }

    /// Extract the first tool result text from a User message.
    fn get_tool_result_text(msg: &Message) -> Option<&str> {
        if let Message::User { content } = msg {
            for part in content.iter() {
                if let UserContent::ToolResult(tr) = part {
                    for c in tr.content.iter() {
                        if let ToolResultContent::Text(t) = c {
                            return Some(&t.text);
                        }
                    }
                }
            }
        }
        None
    }

    #[test]
    fn test_masking_empty_history() {
        let mut history: Vec<Message> = vec![];
        mask_old_tool_outputs(&mut history, 1000);
        assert!(history.is_empty());
    }

    #[test]
    fn test_masking_protects_recent_outputs() {
        // 3 tool results of 10KB each, protection budget = 25KB
        // → oldest (first) should be masked, other two protected
        let big_content = "x".repeat(10_000);
        let mut history = vec![
            make_user_with_tool_result("call-1", &big_content),
            Message::assistant("Response 1"),
            make_user_with_tool_result("call-2", &big_content),
            Message::assistant("Response 2"),
            make_user_with_tool_result("call-3", &big_content),
            Message::assistant("Response 3"),
        ];

        mask_old_tool_outputs(&mut history, 25_000);

        // First tool result should be masked
        let text0 = get_tool_result_text(&history[0]).expect("should have tool result");
        assert!(
            text0.contains("masked"),
            "oldest result should be masked, got: {}",
            text0
        );

        // Third tool result should be preserved
        let text2 = get_tool_result_text(&history[4]).expect("should have tool result");
        assert!(
            !text2.contains("masked"),
            "most recent result should be preserved"
        );
        assert_eq!(text2.len(), 10_000);
    }

    #[test]
    fn test_masking_skips_small_outputs() {
        let mut history = vec![
            make_user_with_tool_result("call-1", "small output"),
            Message::assistant("Response"),
        ];

        mask_old_tool_outputs(&mut history, 0); // 0 budget = mask everything possible

        // Small output should NOT be masked (under MIN_MASKABLE_BYTES)
        let text = get_tool_result_text(&history[0]).expect("should have tool result");
        assert_eq!(text, "small output", "small output should not be masked");
    }

    #[test]
    fn test_masking_skips_exempt_tools() {
        let big_content = "x".repeat(10_000);
        let mut history = vec![
            make_user_with_tool_result("journal_read-call-1", &big_content),
            Message::assistant("Response"),
        ];

        mask_old_tool_outputs(&mut history, 0); // 0 budget = mask everything possible

        // Journal tool output should NOT be masked
        let text = get_tool_result_text(&history[0]).expect("should have tool result");
        assert!(
            !text.contains("masked"),
            "exempt tool output should not be masked"
        );
        assert_eq!(text.len(), 10_000);
    }

    #[test]
    fn test_masking_no_tool_results() {
        let mut history = vec![
            Message::user("Hello"),
            Message::assistant("Hi there"),
            Message::user("How are you?"),
            Message::assistant("Good!"),
        ];

        let original_len = history.len();
        mask_old_tool_outputs(&mut history, 1000);
        assert_eq!(
            history.len(),
            original_len,
            "history length should be unchanged"
        );
    }

    #[test]
    fn test_masking_all_within_budget() {
        let content = "x".repeat(5_000);
        let mut history = vec![
            make_user_with_tool_result("call-1", &content),
            Message::assistant("R1"),
            make_user_with_tool_result("call-2", &content),
            Message::assistant("R2"),
        ];

        // Budget of 15KB > total (10KB) → nothing masked
        mask_old_tool_outputs(&mut history, 15_000);

        let text0 = get_tool_result_text(&history[0]).expect("should have tool result");
        assert!(
            !text0.contains("masked"),
            "nothing should be masked when all within budget"
        );
        let text1 = get_tool_result_text(&history[2]).expect("should have tool result");
        assert!(
            !text1.contains("masked"),
            "nothing should be masked when all within budget"
        );
    }
}
