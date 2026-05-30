use std::collections::HashSet;

use domain::CompactionConfig;

use crate::primitives::{AgentMessage, AgentMessageRole, MessagePart};

const DEFAULT_CONTEXT_WINDOW: u64 = 128_000;
const DEFAULT_TOKEN_THRESHOLD: u64 = 100_000;

#[derive(Debug, Clone)]
pub struct CompactContext {
    pub estimated_tokens: u64,
    pub context_window: u64,
    pub user_turns: u32,
    pub message_count: u32,
    pub last_message_is_user: bool,
}

impl CompactContext {
    pub fn from_messages(messages: &[AgentMessage]) -> Self {
        let estimated_tokens = estimate_tokens(messages);
        let mut user_turns = 0u32;
        let last_message_is_user =
            matches!(messages.last(), Some(m) if m.role == AgentMessageRole::User);

        for msg in messages {
            if msg.role == AgentMessageRole::User {
                user_turns += 1;
            }
        }

        Self {
            estimated_tokens,
            context_window: 0,
            user_turns,
            message_count: messages.len() as u32,
            last_message_is_user,
        }
    }

    pub fn with_context_window(mut self, window: u64) -> Self {
        self.context_window = window;
        self
    }
}

pub fn should_compact(context: &CompactContext, config: &CompactionConfig) -> bool {
    config
        .token_threshold
        .map(|t| context.estimated_tokens >= t as u64)
        .unwrap_or(false)
        || config
            .token_threshold_percentage
            .map(|p| {
                let window = if context.context_window > 0 {
                    context.context_window
                } else {
                    DEFAULT_CONTEXT_WINDOW
                };
                context.estimated_tokens as f64 >= window as f64 * p
            })
            .unwrap_or(false)
        || config
            .turn_threshold
            .map(|t| context.user_turns >= t)
            .unwrap_or(false)
        || config
            .message_threshold
            .map(|t| context.message_count >= t)
            .unwrap_or(false)
        || config.on_turn_end.unwrap_or(false) && context.last_message_is_user
}

pub fn effective_token_threshold(config: &CompactionConfig) -> u64 {
    let from_absolute = config.token_threshold.map(|t| t as u64);
    let from_percentage = config
        .token_threshold_percentage
        .map(|p| (DEFAULT_CONTEXT_WINDOW as f64 * p) as u64);
    from_absolute
        .or(from_percentage)
        .unwrap_or(DEFAULT_TOKEN_THRESHOLD)
}

pub fn compact(messages: &mut Vec<AgentMessage>, config: &CompactionConfig) -> u64 {
    let total_before = estimate_tokens(messages);
    let retention = config.overlap_event_count.max(1) as usize;

    let Some(range) = find_eviction_range(messages, retention) else {
        return 0;
    };

    let summary = build_structured_summary(&messages[range.clone()]);
    messages.splice(range, std::iter::once(AgentMessage::user(summary)));

    let total_after = estimate_tokens(messages);
    total_before.saturating_sub(total_after)
}

fn find_eviction_range(
    messages: &[AgentMessage],
    retention: usize,
) -> Option<std::ops::Range<usize>> {
    let len = messages.len();
    if len <= 2 {
        return None;
    }

    let start = messages.iter().position(|msg| {
        msg.role == AgentMessageRole::Assistant || msg.role == AgentMessageRole::User
    })?;

    let end = len.saturating_sub(retention).saturating_sub(1);
    if start >= end || end == 0 {
        return None;
    }

    if messages[end].role == AgentMessageRole::Tool && end > start {
        let mut tool_start = end;
        while tool_start > start && messages[tool_start].role == AgentMessageRole::Tool {
            tool_start -= 1;
        }
        let adjusted = if tool_start + 1 > start {
            tool_start + 1
        } else {
            end
        };
        if adjusted > start {
            return Some(start..adjusted);
        }
    }

    Some(start..end)
}

fn build_structured_summary(messages: &[AgentMessage]) -> String {
    let entries: Vec<SummaryEntry> = messages.iter().filter_map(build_entry).collect();
    let deduped = deduplicate_entries(entries);
    render_summary_template(&deduped)
}

#[derive(Debug, Clone)]
enum SummaryEntry {
    UserText {
        text: String,
    },
    AssistantText {
        text: String,
    },
    ToolCall {
        name: String,
        args: String,
        success: Option<bool>,
    },
    SystemMsg {
        text: String,
    },
}

fn build_entry(msg: &AgentMessage) -> Option<SummaryEntry> {
    match msg.role {
        AgentMessageRole::User => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                    MessagePart::InlineData { .. } => "[inline]",
                })
                .collect::<Vec<_>>()
                .join("\n");
            if text.trim().is_empty() {
                None
            } else {
                Some(SummaryEntry::UserText {
                    text: text.trim().to_string(),
                })
            }
        }
        AgentMessageRole::Assistant if !msg.tool_calls.is_empty() => {
            let call = &msg.tool_calls[0];
            let args = serde_json::to_string(&call.arguments).unwrap_or_else(|_| "{}".to_string());
            let args_short = if args.len() > 200 {
                format!("{}...", &args[..200])
            } else {
                args
            };
            Some(SummaryEntry::ToolCall {
                name: call.name.clone(),
                args: args_short,
                success: None,
            })
        }
        AgentMessageRole::Assistant => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                    MessagePart::InlineData { .. } => "[inline]",
                })
                .collect::<Vec<_>>()
                .join("\n");
            if text.trim().is_empty() {
                None
            } else {
                Some(SummaryEntry::AssistantText {
                    text: text.trim().to_string(),
                })
            }
        }
        AgentMessageRole::Tool => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                    MessagePart::InlineData { .. } => "[inline]",
                })
                .collect::<Vec<_>>()
                .join("\n");
            let is_success = !text.contains("error") && !text.contains("Error");
            let text = format_tool_result(&text);
            Some(SummaryEntry::ToolCall {
                name: "tool_result".to_string(),
                args: text,
                success: Some(is_success),
            })
        }
        AgentMessageRole::System => {
            let text = msg
                .parts
                .iter()
                .map(|p| match p {
                    MessagePart::Text { text } => text.as_str(),
                    MessagePart::InlineData { .. } => "[inline]",
                })
                .collect::<Vec<_>>()
                .join("\n");
            let trimmed = text.trim().to_string();
            if trimmed.is_empty() || is_prompt_segment(&trimmed) {
                None
            } else {
                Some(SummaryEntry::SystemMsg { text: trimmed })
            }
        }
    }
}

fn is_prompt_segment(text: &str) -> bool {
    text.contains("## ") && text.len() > 500
}

fn format_tool_result(text: &str) -> String {
    if text.len() > 300 {
        format!("{}...", &text[..300])
    } else {
        text.to_string()
    }
}

fn deduplicate_entries(entries: Vec<SummaryEntry>) -> Vec<SummaryEntry> {
    let mut seen_paths: HashSet<String> = HashSet::new();
    let mut result = Vec::new();

    for entry in entries {
        let should_keep = match &entry {
            SummaryEntry::ToolCall { name, args, .. }
                if name == "read_file" || name == "write_file" || name == "edit_file" =>
            {
                if let Some(path) = extract_file_path(args) {
                    let key = format!("{name}:{path}");
                    !seen_paths.contains(&key)
                } else {
                    true
                }
            }
            _ => true,
        };

        if should_keep {
            if let SummaryEntry::ToolCall { name, args, .. } = &entry {
                if name == "read_file" || name == "write_file" || name == "edit_file" {
                    if let Some(path) = extract_file_path(args) {
                        seen_paths.insert(format!("{name}:{path}"));
                    }
                }
            }
            result.push(entry);
        }
    }

    result
}

fn extract_file_path(args: &str) -> Option<String> {
    let needle = "\"path\":";
    if let Some(pos) = args.find(needle) {
        let rest = &args[pos + needle.len()..].trim();
        let inner = rest.strip_prefix('"')?;
        let end = inner.find('"')?;
        Some(inner[..end].to_string())
    } else {
        None
    }
}

fn render_summary_template(entries: &[SummaryEntry]) -> String {
    let mut lines = vec![
        "Conversation summary (compacted)".to_string(),
        String::new(),
    ];

    for (idx, entry) in entries.iter().enumerate() {
        let num = idx + 1;
        match entry {
            SummaryEntry::UserText { text } => {
                let short = if text.len() > 200 {
                    format!("{}...", &text[..200])
                } else {
                    text.clone()
                };
                lines.push(format!("{num}. [User] {short}"));
            }
            SummaryEntry::AssistantText { text } => {
                let short = if text.len() > 200 {
                    format!("{}...", &text[..200])
                } else {
                    text.clone()
                };
                lines.push(format!("{num}. [Assistant] {short}"));
            }
            SummaryEntry::ToolCall {
                name,
                args,
                success,
            } => {
                let status = match success {
                    Some(true) => " ok",
                    Some(false) => " failed",
                    None => "",
                };
                lines.push(format!("{num}. [{name}{status}] {args}"));
            }
            SummaryEntry::SystemMsg { text } => {
                let short = if text.len() > 200 {
                    format!("{}...", &text[..200])
                } else {
                    text.clone()
                };
                lines.push(format!("{num}. [System] {short}"));
            }
        }
    }

    lines.push(String::new());
    lines.push("Continue based on this context.".to_string());
    lines.join("\n")
}

fn estimate_tokens(messages: &[AgentMessage]) -> u64 {
    estimate_tokens_static(messages)
}

pub fn estimate_tokens_static(messages: &[AgentMessage]) -> u64 {
    let chars: usize = messages
        .iter()
        .flat_map(|msg| msg.parts.iter())
        .map(|part| match part {
            MessagePart::Text { text } => text.len(),
            MessagePart::InlineData { data, .. } => data.len(),
        })
        .sum();
    (chars as u64 / 4).max(1)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::primitives::AgentMessage;

    fn make_msg(role: AgentMessageRole, text: &str) -> AgentMessage {
        match role {
            AgentMessageRole::System => AgentMessage::system(text),
            AgentMessageRole::User => AgentMessage::user(text),
            AgentMessageRole::Assistant => AgentMessage::assistant(text),
            AgentMessageRole::Tool => AgentMessage::tool_result("t1", text),
        }
    }

    #[test]
    fn empty_messages_no_compaction() {
        let messages: Vec<AgentMessage> = vec![];
        let ctx = CompactContext::from_messages(&messages);
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(10),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 2,
            chars_per_token: 4,
            on_turn_end: None,
        };
        assert!(!should_compact(&ctx, &config));
    }

    #[test]
    fn token_threshold_triggers() {
        let mut messages: Vec<AgentMessage> = Vec::new();
        for _ in 0..50 {
            messages.push(make_msg(AgentMessageRole::User, "a".repeat(100).as_str()));
        }
        let ctx = CompactContext::from_messages(&messages);
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(100),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 2,
            chars_per_token: 4,
            on_turn_end: None,
        };
        assert!(should_compact(&ctx, &config));
    }

    #[test]
    fn message_threshold_triggers() {
        let messages: Vec<AgentMessage> = vec![
            make_msg(AgentMessageRole::System, "sys"),
            make_msg(AgentMessageRole::User, "hi"),
            make_msg(AgentMessageRole::Assistant, "hello"),
            make_msg(AgentMessageRole::User, "more"),
        ];
        let ctx = CompactContext::from_messages(&messages);
        let config = CompactionConfig {
            enabled: true,
            token_threshold: None,
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: Some(3),
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 2,
            chars_per_token: 4,
            on_turn_end: None,
        };
        assert!(should_compact(&ctx, &config));
    }

    #[test]
    fn compaction_reduces_message_count() {
        let mut messages: Vec<AgentMessage> = Vec::new();
        messages.push(make_msg(AgentMessageRole::System, "sys prompt"));
        messages.push(make_msg(AgentMessageRole::User, "build a habit tracker"));
        messages.push(make_msg(AgentMessageRole::Assistant, ""));
        messages[2].tool_calls = vec![crate::primitives::ToolCall {
            id: "c1".into(),
            name: "bash".into(),
            arguments: serde_json::json!({"command": "ls"}),
        }];
        messages.push(make_msg(AgentMessageRole::Tool, r#"{"status":"ok"}"#));
        messages.push(make_msg(AgentMessageRole::Assistant, "I'll build it"));
        messages.push(make_msg(AgentMessageRole::User, "go ahead"));

        let before = messages.len();
        let config = CompactionConfig {
            enabled: true,
            token_threshold: Some(10),
            token_threshold_percentage: None,
            turn_threshold: None,
            message_threshold: None,
            eviction_window: 0.2,
            retention_window: 0,
            overlap_event_count: 1,
            chars_per_token: 4,
            on_turn_end: None,
        };
        compact(&mut messages, &config);
        assert!(messages.len() < before);
    }
}
