use rig::message::{AssistantContent, Message, UserContent};

/// Check if a rig message is a user text message (not a tool result).
pub(super) fn is_user_message(msg: &Message) -> bool {
    match msg {
        Message::User { content } => content.iter().any(|c| matches!(c, UserContent::Text(_))),
        _ => false,
    }
}

/// Convert a single rig message to its text representation for token counting.
pub(super) fn message_to_text(msg: &Message) -> String {
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
