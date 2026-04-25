//! Extract a structured `ContextSummary` from a slice of rig messages.
//!
//! Mirrors forgecode's `impl From<&Context> for ContextSummary`:
//! 1. Walk messages, accumulating same-role messages into a buffer.
//! 2. Flush buffer as a `SummaryBlock` whenever the role changes.
//! 3. Tool results are NOT rendered into the summary — they are collected
//!    into a `HashMap<call_id, &ToolResult>` and used to set the
//!    `is_success` flag on the matching tool calls in a post-processing pass.
//! 4. Reasoning blocks are dropped from the summary entirely (the rendered
//!    tool calls and text capture the conclusions; carry-forward of the
//!    most-recent reasoning block is handled separately by the caller).
//! 5. System messages are skipped.
//!
//! This is a pure function. No LLM calls, no allocation surprises, no
//! state besides the accumulator.

use std::collections::HashMap;

use rig::message::{
    AssistantContent, Message, ToolCall, ToolResult, ToolResultContent, UserContent,
};
use serde_json::Value;

use super::summary::{
    ContextSummary, SummaryBlock, SummaryMessage, SummaryRole, SummaryTool, SummaryToolCall,
    TodoChange, TodoChangeKind, TodoStatus,
};

/// Extract a `ContextSummary` from a contiguous slice of messages.
pub fn extract(messages: &[Message]) -> ContextSummary {
    let mut blocks: Vec<SummaryBlock> = Vec::new();
    let mut buffer: Vec<SummaryMessage> = Vec::new();
    let mut current_role: Option<SummaryRole> = None;
    // Pre-pass: collect tool results so we can stamp success/failure on the
    // matching tool calls in one shot at the end.
    let tool_results: HashMap<String, &ToolResult> = collect_tool_results(messages);
    // Running todo state used to compute TodoWrite diffs.
    let mut current_todos: Vec<(String, TodoStatus)> = Vec::new();

    for msg in messages {
        let Some((role, contents)) = render_message(msg, &mut current_todos) else {
            continue;
        };
        if Some(role) != current_role {
            if !buffer.is_empty() {
                if let Some(prev_role) = current_role {
                    blocks.push(SummaryBlock {
                        role: prev_role,
                        contents: std::mem::take(&mut buffer),
                    });
                }
            }
            current_role = Some(role);
        }
        buffer.extend(contents);
    }
    if !buffer.is_empty() {
        if let Some(role) = current_role {
            blocks.push(SummaryBlock {
                role,
                contents: buffer,
            });
        }
    }

    // Stamp success/failure on every tool call that has a matching result.
    for block in blocks.iter_mut() {
        for content in block.contents.iter_mut() {
            if let SummaryMessage::ToolCall(tc) = content {
                if let Some(call_id) = &tc.id {
                    if let Some(result) = tool_results.get(call_id) {
                        tc.is_success = !result_is_error(result);
                    }
                }
            }
        }
    }

    ContextSummary { messages: blocks }
}

/// First pass: build a lookup table from tool_call_id → ToolResult so we can
/// link calls to their outcomes without nested loops.
fn collect_tool_results(messages: &[Message]) -> HashMap<String, &ToolResult> {
    let mut map = HashMap::new();
    for msg in messages {
        if let Message::User { content } = msg {
            for part in content.iter() {
                if let UserContent::ToolResult(tr) = part {
                    let key = tr.call_id.clone().unwrap_or_else(|| tr.id.clone());
                    map.insert(key, tr);
                }
            }
        }
    }
    map
}

/// Heuristic: does a `ToolResult` look like an error? Bridge's tool layer
/// wraps errors with `Toolset error:` / `ToolCallError:` / specific phrases.
fn result_is_error(tr: &ToolResult) -> bool {
    for c in tr.content.iter() {
        if let ToolResultContent::Text(t) = c {
            let s = t.text.as_str();
            if s.starts_with("Toolset error")
                || s.starts_with("Tool error")
                || s.contains("File not found")
                || s.contains("Path does not exist")
            {
                return true;
            }
        }
    }
    false
}

/// Convert one rig `Message` into `(role, blocks)`. Returns `None` for
/// messages that should be skipped entirely from the summary (system,
/// tool-result-only user messages — the latter are captured via the
/// pre-pass and stamped onto their parent tool call).
fn render_message(
    msg: &Message,
    current_todos: &mut Vec<(String, TodoStatus)>,
) -> Option<(SummaryRole, Vec<SummaryMessage>)> {
    match msg {
        Message::User { content } => {
            let mut blocks = Vec::new();
            for part in content.iter() {
                match part {
                    UserContent::Text(t) => {
                        let s = t.text.trim();
                        if !s.is_empty() {
                            blocks.push(SummaryMessage::Text(s.to_string()));
                        }
                    }
                    // Tool results are linked to their tool call in the
                    // post-pass; they're not rendered as standalone summary
                    // blocks.
                    UserContent::ToolResult(_) => {}
                    _ => {}
                }
            }
            if blocks.is_empty() {
                None
            } else {
                Some((SummaryRole::User, blocks))
            }
        }
        Message::System { content } => {
            let s = content.trim();
            if s.is_empty() {
                None
            } else {
                Some((SummaryRole::User, vec![SummaryMessage::Text(s.to_string())]))
            }
        }
        Message::Assistant { content, .. } => {
            let mut blocks = Vec::new();
            for part in content.iter() {
                match part {
                    AssistantContent::Text(t) => {
                        let s = t.text.trim();
                        if !s.is_empty() {
                            blocks.push(SummaryMessage::Text(s.to_string()));
                        }
                    }
                    AssistantContent::ToolCall(tc) => {
                        let summary_tool = extract_tool(tc, current_todos);
                        // Update running todo state if this was a TodoWrite.
                        if let SummaryTool::TodoWrite { ref changes } = summary_tool {
                            apply_todo_changes(current_todos, changes);
                        }
                        blocks.push(SummaryMessage::ToolCall(SummaryToolCall {
                            id: Some(tc.id.clone()),
                            tool: summary_tool,
                            // Default to true; the post-pass downgrades to
                            // false if a matching error result is found.
                            is_success: true,
                        }));
                    }
                    AssistantContent::Reasoning(_) => {
                        // Reasoning blocks are dropped from the summary;
                        // the chain-handoff caller separately injects the
                        // most-recent reasoning into the first surviving
                        // assistant message after compaction.
                    }
                    _ => {}
                }
            }
            if blocks.is_empty() {
                None
            } else {
                Some((SummaryRole::Assistant, blocks))
            }
        }
    }
}

/// Map a rig `ToolCall` to a `SummaryTool`. Per-tool extraction strips
/// long/noisy params and keeps only what the receiving agent needs to
/// know what was done.
fn extract_tool(tc: &ToolCall, current_todos: &[(String, TodoStatus)]) -> SummaryTool {
    let name = tc.function.name.as_str();
    let args = &tc.function.arguments;
    match name {
        "Read" => SummaryTool::FileRead {
            path: arg_path(args).unwrap_or_default(),
        },
        // forgecode unifies Write/Edit/MultiEdit/Patch under "Update".
        "write" | "Write" | "edit" | "Edit" | "multiedit" | "MultiEdit" => {
            SummaryTool::FileUpdate {
                path: arg_path(args).unwrap_or_default(),
            }
        }
        "bash" | "Bash" => SummaryTool::Shell {
            command: arg_str(args, "command").unwrap_or_default(),
        },
        "Glob" => SummaryTool::Glob {
            pattern: arg_str(args, "pattern").unwrap_or_default(),
        },
        "RipGrep" => SummaryTool::Search {
            pattern: arg_str(args, "pattern").unwrap_or_default(),
            path: arg_str(args, "path"),
        },
        "LS" | "Ls" => SummaryTool::List {
            path: arg_path(args).unwrap_or_default(),
        },
        "todowrite" | "TodoWrite" => {
            let changes = diff_todos(args, current_todos);
            SummaryTool::TodoWrite { changes }
        }
        "todoread" | "TodoRead" => SummaryTool::TodoRead,
        "lsp" | "LSP" => SummaryTool::Lsp {
            action: arg_str(args, "action").unwrap_or_default(),
            target: arg_str(args, "uri")
                .or_else(|| arg_str(args, "file"))
                .or_else(|| arg_str(args, "path")),
        },
        other => SummaryTool::Mcp {
            name: other.to_string(),
        },
    }
}

fn arg_str(args: &Value, key: &str) -> Option<String> {
    args.get(key)
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
}

fn arg_path(args: &Value) -> Option<String> {
    arg_str(args, "path")
        .or_else(|| arg_str(args, "file_path"))
        .or_else(|| arg_str(args, "file"))
}

/// Diff the new `todowrite` payload against the running snapshot. The diff
/// is what gets rendered (Added / Updated / Removed); the snapshot itself
/// is mutated by the caller via `apply_todo_changes` so subsequent
/// `todowrite`s diff against the new state.
fn diff_todos(args: &Value, current_todos: &[(String, TodoStatus)]) -> Vec<TodoChange> {
    let mut changes = Vec::new();
    let Some(arr) = args.get("todos").and_then(|v| v.as_array()) else {
        return changes;
    };
    let before: HashMap<&str, TodoStatus> = current_todos
        .iter()
        .map(|(c, s)| (c.as_str(), *s))
        .collect();
    for item in arr {
        let Some(content) = item.get("content").and_then(|v| v.as_str()) else {
            continue;
        };
        let status = parse_todo_status(item);
        match (before.get(content), status) {
            (Some(prev_status), TodoStatus::Cancelled) => {
                // Was tracked, now cancelled → Removed.
                changes.push(TodoChange {
                    content: content.to_string(),
                    status: *prev_status,
                    kind: TodoChangeKind::Removed,
                });
            }
            (Some(prev_status), new_status) if *prev_status != new_status => {
                changes.push(TodoChange {
                    content: content.to_string(),
                    status: new_status,
                    kind: TodoChangeKind::Updated,
                });
            }
            (None, TodoStatus::Cancelled) => {
                // Untracked, was cancelled — skip (forgecode behaviour).
            }
            (None, new_status) => {
                changes.push(TodoChange {
                    content: content.to_string(),
                    status: new_status,
                    kind: TodoChangeKind::Added,
                });
            }
            _ => {}
        }
    }
    changes
}

fn parse_todo_status(item: &Value) -> TodoStatus {
    match item.get("status").and_then(|v| v.as_str()) {
        Some("in_progress") => TodoStatus::InProgress,
        Some("completed") => TodoStatus::Completed,
        Some("cancelled") => TodoStatus::Cancelled,
        _ => TodoStatus::Pending,
    }
}

fn apply_todo_changes(current_todos: &mut Vec<(String, TodoStatus)>, changes: &[TodoChange]) {
    for change in changes {
        match change.kind {
            TodoChangeKind::Added => {
                current_todos.push((change.content.clone(), change.status));
            }
            TodoChangeKind::Updated => {
                if let Some(slot) = current_todos.iter_mut().find(|(c, _)| c == &change.content) {
                    slot.1 = change.status;
                }
            }
            TodoChangeKind::Removed => {
                current_todos.retain(|(c, _)| c != &change.content);
            }
        }
    }
}

#[cfg(test)]
mod tests;
