use bridge_core::conversation::{Message, Role};

/// Heuristic: is this stream error retryable? Mirrors the http-level retry
/// logic in `llm::providers::is_retryable_error`, but operates on a bare
/// error string because by the time we see it here the typed error has been
/// flattened to a ProviderError(String).
pub(super) fn is_retryable_stream_err(err_msg: &str) -> bool {
    let m = err_msg.to_ascii_lowercase();
    m.contains("429")
        || m.contains("too many requests")
        || m.contains("rate limit")
        || m.contains("rate-limit")
        || m.contains("502")
        || m.contains("503")
        || m.contains("504")
        || m.contains("upstream")
        || m.contains("overloaded")
        || m.contains("timeout")
        || m.contains("timed out")
        || m.contains("connection reset")
        || m.contains("connection closed")
        || m.contains("temporarily")
}

/// Extract tool names (in call order) from the assistant messages added to
/// `enriched` during the current turn (i.e. the tail starting at
/// `baseline_len`). Used by the tool-requirement enforcement pass to check
/// whether the turn called all required tools — and in what order for
/// `TurnStart`/`TurnEnd` position constraints.
pub(super) fn extract_tool_names_from_turn(
    enriched: &[rig::message::Message],
    baseline_len: usize,
) -> Vec<String> {
    use rig::completion::message::AssistantContent;
    let mut names = Vec::new();
    for msg in enriched[baseline_len..].iter() {
        if let rig::message::Message::Assistant { content, .. } = msg {
            for c in content.iter() {
                if let AssistantContent::ToolCall(tc) = c {
                    names.push(tc.function.name.clone());
                }
            }
        }
    }
    names
}

/// Check if any assistant messages in `enriched[baseline_len..]` contain tool calls.
pub(super) fn history_contains_tool_calls(
    enriched: &[rig::message::Message],
    baseline_len: usize,
) -> bool {
    use rig::completion::message::AssistantContent;
    enriched[baseline_len..].iter().any(|msg| {
        if let rig::message::Message::Assistant { content, .. } = msg {
            content
                .iter()
                .any(|c| matches!(c, AssistantContent::ToolCall(_)))
        } else {
            false
        }
    })
}

pub(super) fn text_message(role: Role, text: String) -> Message {
    Message {
        role,
        content: vec![bridge_core::conversation::ContentBlock::Text { text }],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

fn tool_result_message(tool_call_id: String, content: String) -> Message {
    Message {
        role: Role::Tool,
        content: vec![bridge_core::conversation::ContentBlock::ToolResult(
            bridge_core::conversation::ToolResult {
                tool_call_id,
                content,
                is_error: false,
            },
        )],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

fn tool_result_text(parts: &rig::OneOrMany<rig::message::ToolResultContent>) -> String {
    parts
        .iter()
        .map(|part| match part {
            rig::message::ToolResultContent::Text(text) => text.text.clone(),
            other => format!("{:?}", other),
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn convert_from_rig_message(msg: &rig::message::Message) -> Vec<Message> {
    use rig::completion::message::{AssistantContent, UserContent};

    match msg {
        rig::message::Message::User { content } => content
            .iter()
            .filter_map(|part| match part {
                UserContent::Text(text) if !text.text.is_empty() => {
                    Some(text_message(Role::User, text.text.clone()))
                }
                UserContent::ToolResult(result) => Some(tool_result_message(
                    result.id.clone(),
                    tool_result_text(&result.content),
                )),
                _ => None,
            })
            .collect(),
        rig::message::Message::Assistant { content, .. } => {
            let mut blocks = Vec::new();
            for part in content.iter() {
                match part {
                    AssistantContent::Text(text) if !text.text.is_empty() => {
                        blocks.push(bridge_core::conversation::ContentBlock::Text {
                            text: text.text.clone(),
                        });
                    }
                    AssistantContent::ToolCall(call) => {
                        let arguments = call.function.arguments.clone();
                        blocks.push(bridge_core::conversation::ContentBlock::ToolCall(
                            bridge_core::conversation::ToolCall {
                                id: call.id.clone(),
                                name: call.function.name.clone(),
                                arguments,
                            },
                        ));
                    }
                    _ => {}
                }
            }

            if blocks.is_empty() {
                Vec::new()
            } else {
                vec![Message {
                    role: Role::Assistant,
                    content: blocks,
                    timestamp: chrono::Utc::now(),
                    system_reminder: None,
                }]
            }
        }
    }
}

pub(super) fn convert_from_rig_messages(messages: &[rig::message::Message]) -> Vec<Message> {
    messages.iter().flat_map(convert_from_rig_message).collect()
}

pub(super) fn apply_compaction_to_persisted_history(
    history: &mut Vec<Message>,
    summary_text: &str,
    messages_compacted: usize,
) {
    let split_at = messages_compacted.min(history.len());
    if split_at == 0 {
        return;
    }

    let mut compacted = Vec::with_capacity(1 + history.len().saturating_sub(split_at));
    compacted.push(text_message(
        Role::User,
        format!("[Conversation Summary]\n{}", summary_text),
    ));
    compacted.extend(history.drain(split_at..));
    *history = compacted;
}

pub fn normalize_messages_for_persistence(messages: &[Message]) -> Vec<Message> {
    let mut normalized = Vec::with_capacity(messages.len());

    for message in messages {
        if message.role == Role::Tool {
            let mut expanded = false;
            for block in &message.content {
                if let bridge_core::conversation::ContentBlock::ToolResult(result) = block {
                    expanded = true;
                    normalized.push(Message {
                        role: Role::Tool,
                        content: vec![bridge_core::conversation::ContentBlock::ToolResult(
                            result.clone(),
                        )],
                        timestamp: message.timestamp,
                        system_reminder: None,
                    });
                }
            }
            if !expanded {
                normalized.push(message.clone());
            }
        } else {
            normalized.push(message.clone());
        }
    }

    normalized
}

/// Convert a bridge_core Message into a rig message.
///
/// Handles all content block types so that hydrated conversations preserve
/// the full tool-call/tool-result exchange the LLM needs for context.
pub(super) fn convert_to_rig_message(msg: &Message) -> Option<rig::message::Message> {
    use bridge_core::conversation::ContentBlock;
    use rig::completion::message::AssistantContent;
    use rig::OneOrMany;

    match msg.role {
        Role::User => {
            let text = extract_text_content(msg);
            if text.is_empty() {
                return None;
            }
            Some(rig::message::Message::user(&text))
        }
        Role::Assistant => {
            let mut items: Vec<AssistantContent> = Vec::new();
            for block in &msg.content {
                match block {
                    ContentBlock::Text { text } if !text.is_empty() => {
                        items.push(AssistantContent::text(text));
                    }
                    ContentBlock::ToolCall(tc) => {
                        items.push(AssistantContent::tool_call(
                            &tc.id,
                            &tc.name,
                            tc.arguments.clone(),
                        ));
                    }
                    _ => {}
                }
            }
            let content = OneOrMany::many(items).ok()?;
            Some(rig::message::Message::Assistant { id: None, content })
        }
        Role::Tool => {
            for block in &msg.content {
                if let ContentBlock::ToolResult(tr) = block {
                    return Some(rig::message::Message::tool_result(
                        &tr.tool_call_id,
                        &tr.content,
                    ));
                }
            }
            None
        }
        Role::System => None,
    }
}

/// Convert a slice of bridge_core Messages into rig messages for history seeding.
///
/// Tool-role messages with multiple `ToolResult` blocks are expanded into
/// one rig message per result, since rig models each tool result as a
/// separate user message.
pub fn convert_messages(messages: &[Message]) -> Vec<rig::message::Message> {
    let mut result = Vec::with_capacity(messages.len());
    for msg in messages {
        if msg.role == Role::Tool {
            for block in &msg.content {
                if let bridge_core::conversation::ContentBlock::ToolResult(tr) = block {
                    result.push(rig::message::Message::tool_result(
                        &tr.tool_call_id,
                        &tr.content,
                    ));
                }
            }
        } else if let Some(rig_msg) = convert_to_rig_message(msg) {
            result.push(rig_msg);
        }
    }
    result
}

/// Extract text content from a Message for sending to the LLM.
pub(super) fn extract_text_content(message: &Message) -> String {
    message
        .content
        .iter()
        .filter_map(|block| match block {
            bridge_core::conversation::ContentBlock::Text { text } => Some(text.as_str()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("\n")
}
