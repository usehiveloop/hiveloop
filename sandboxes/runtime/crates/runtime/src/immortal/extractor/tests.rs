use rig::message::AssistantContent;
use rig::message::{Message, ToolCall, ToolFunction, ToolResult, ToolResultContent, UserContent};
use rig::OneOrMany;
use serde_json::json;

use super::extract;
use crate::immortal::summary::{SummaryMessage, SummaryRole, SummaryTool, TodoChangeKind};

fn user(text: &str) -> Message {
    Message::user(text)
}

fn assistant_text(text: &str) -> Message {
    Message::assistant(text)
}

fn assistant_tool(id: &str, name: &str, args: serde_json::Value) -> Message {
    Message::Assistant {
        id: None,
        content: OneOrMany::one(AssistantContent::ToolCall(ToolCall {
            id: id.to_string(),
            call_id: None,
            function: ToolFunction {
                name: name.to_string(),
                arguments: args,
            },
            signature: None,
            additional_params: None,
        })),
    }
}

fn tool_result(id: &str, body: &str) -> Message {
    Message::User {
        content: OneOrMany::one(UserContent::ToolResult(ToolResult {
            id: id.to_string(),
            call_id: Some(id.to_string()),
            content: OneOrMany::one(ToolResultContent::Text(rig::message::Text {
                text: body.to_string(),
            })),
        })),
    }
}

#[test]
fn empty_history_yields_empty_summary() {
    let s = extract(&[]);
    assert!(s.messages.is_empty());
}

#[test]
fn user_text_extracted_and_grouped() {
    let s = extract(&[user("hello"), user("world")]);
    assert_eq!(s.messages.len(), 1);
    assert_eq!(s.messages[0].role, SummaryRole::User);
    assert_eq!(s.messages[0].contents.len(), 2);
}

#[test]
fn role_alternation_creates_separate_blocks() {
    let s = extract(&[
        user("u1"),
        assistant_text("a1"),
        user("u2"),
        assistant_text("a2"),
    ]);
    assert_eq!(s.messages.len(), 4);
    assert_eq!(s.messages[0].role, SummaryRole::User);
    assert_eq!(s.messages[1].role, SummaryRole::Assistant);
    assert_eq!(s.messages[2].role, SummaryRole::User);
    assert_eq!(s.messages[3].role, SummaryRole::Assistant);
}

#[test]
fn consecutive_assistant_messages_grouped() {
    let s = extract(&[
        user("u"),
        assistant_text("a1"),
        assistant_tool("c1", "Read", json!({"path": "x"})),
        assistant_tool("c2", "bash", json!({"command": "ls"})),
    ]);
    assert_eq!(s.messages.len(), 2);
    assert_eq!(s.messages[1].role, SummaryRole::Assistant);
    assert_eq!(s.messages[1].contents.len(), 3);
}

#[test]
fn tool_results_link_success_status() {
    let s = extract(&[
        assistant_tool("c1", "Read", json!({"path": "ok.txt"})),
        tool_result("c1", "file content"),
        assistant_tool("c2", "Read", json!({"path": "missing.txt"})),
        tool_result("c2", "Toolset error: File not found: missing.txt"),
    ]);
    let block = &s.messages[0];
    let calls: Vec<_> = block
        .contents
        .iter()
        .filter_map(|c| match c {
            SummaryMessage::ToolCall(tc) => Some(tc),
            _ => None,
        })
        .collect();
    assert_eq!(calls.len(), 2);
    assert!(calls[0].is_success);
    assert!(
        !calls[1].is_success,
        "error result should mark is_success=false"
    );
}

#[test]
fn tool_extraction_per_known_tool() {
    let s = extract(&[
        assistant_tool("c1", "Read", json!({"path": "src/a.rs"})),
        assistant_tool("c2", "write", json!({"path": "out.txt"})),
        assistant_tool("c3", "edit", json!({"path": "x.rs"})),
        assistant_tool("c4", "bash", json!({"command": "cargo test"})),
        assistant_tool("c5", "Glob", json!({"pattern": "**/*.rs"})),
        assistant_tool("c6", "RipGrep", json!({"pattern": "TODO", "path": "src/"})),
        assistant_tool("c7", "LS", json!({"path": "."})),
        assistant_tool("c8", "todoread", json!({})),
        assistant_tool("c9", "weird_unknown", json!({"x": 1})),
    ]);
    let calls: Vec<&SummaryTool> = s.messages[0]
        .contents
        .iter()
        .filter_map(|c| match c {
            SummaryMessage::ToolCall(tc) => Some(&tc.tool),
            _ => None,
        })
        .collect();
    assert!(matches!(calls[0], SummaryTool::FileRead { path } if path == "src/a.rs"));
    // write/edit/multiedit unify under FileUpdate (forgecode parity).
    assert!(matches!(calls[1], SummaryTool::FileUpdate { path } if path == "out.txt"));
    assert!(matches!(calls[2], SummaryTool::FileUpdate { path } if path == "x.rs"));
    assert!(matches!(calls[3], SummaryTool::Shell { command } if command == "cargo test"));
    assert!(matches!(calls[4], SummaryTool::Glob { pattern } if pattern == "**/*.rs"));
    assert!(
        matches!(calls[5], SummaryTool::Search { pattern, path: Some(p) } if pattern == "TODO" && p == "src/")
    );
    assert!(matches!(calls[6], SummaryTool::List { path } if path == "."));
    assert!(matches!(calls[7], SummaryTool::TodoRead));
    // unknown tool falls through to Mcp{name}
    assert!(matches!(calls[8], SummaryTool::Mcp { name } if name == "weird_unknown"));
}

#[test]
fn todowrite_diffs_against_running_state() {
    let s = extract(&[
        assistant_tool(
            "c1",
            "todowrite",
            json!({"todos": [
                {"content": "task A", "status": "pending"},
                {"content": "task B", "status": "in_progress"},
            ]}),
        ),
        assistant_tool(
            "c2",
            "todowrite",
            json!({"todos": [
                {"content": "task A", "status": "completed"},
                {"content": "task B", "status": "in_progress"},
                {"content": "task C", "status": "pending"},
            ]}),
        ),
    ]);
    let calls: Vec<&SummaryTool> = s.messages[0]
        .contents
        .iter()
        .filter_map(|c| match c {
            SummaryMessage::ToolCall(tc) => Some(&tc.tool),
            _ => None,
        })
        .collect();
    if let SummaryTool::TodoWrite { changes } = calls[0] {
        // First call: 2 added.
        assert_eq!(changes.len(), 2);
        assert_eq!(changes[0].kind, TodoChangeKind::Added);
        assert_eq!(changes[1].kind, TodoChangeKind::Added);
    } else {
        panic!("expected TodoWrite");
    }
    if let SummaryTool::TodoWrite { changes } = calls[1] {
        // Second call: A completed (Updated), B unchanged (no diff), C added.
        let kinds: Vec<TodoChangeKind> = changes.iter().map(|c| c.kind).collect();
        assert!(kinds.contains(&TodoChangeKind::Updated));
        assert!(kinds.contains(&TodoChangeKind::Added));
    } else {
        panic!("expected TodoWrite");
    }
}

#[test]
fn system_messages_are_skipped() {
    let s = extract(&[
        Message::System {
            content: "you are a helpful agent".to_string(),
        },
        user("hi"),
    ]);
    assert_eq!(s.messages.len(), 1);
    assert_eq!(s.messages[0].role, SummaryRole::User);
}

#[test]
fn empty_text_blocks_skipped() {
    let s = extract(&[user("   \n  "), user("real content")]);
    assert_eq!(s.messages.len(), 1);
    assert_eq!(s.messages[0].contents.len(), 1);
    if let SummaryMessage::Text(t) = &s.messages[0].contents[0] {
        assert_eq!(t, "real content");
    } else {
        panic!("expected text");
    }
}
