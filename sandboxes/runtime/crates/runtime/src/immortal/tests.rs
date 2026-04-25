use rig::message::AssistantContent;
use rig::message::{Message, ToolCall, ToolFunction, ToolResult, ToolResultContent, UserContent};
use rig::OneOrMany;
use serde_json::json;

use super::handoff::{find_compaction_range, splice_summary, CompactionRange};
use super::*;

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
fn find_compaction_range_skips_when_no_assistant() {
    let history = vec![user("hi")];
    assert!(find_compaction_range(&history, 0, 100).is_none());
}

#[test]
fn find_compaction_range_anchors_at_first_assistant() {
    let history = vec![
        user("user prompt"),   // 0 — preserved before
        assistant_text("ack"), // 1 — start
        user("more"),          // 2
        assistant_text("ok"),  // 3
        assistant_text("ok2"), // 4
    ];
    // retention=1 → end candidate = 5 - 1 - 1 = 3
    let r = find_compaction_range(&history, 1, 100).unwrap();
    assert_eq!(r.start, 1);
    assert_eq!(r.end, 3);
}

#[test]
fn find_compaction_range_respects_retention() {
    let history = vec![
        user("u"),
        assistant_text("a"),
        assistant_text("b"),
        assistant_text("c"),
    ];
    assert!(find_compaction_range(&history, 10, 100).is_none());
}

#[test]
fn find_compaction_range_walks_back_around_tool_call() {
    // The proposed `end` is an assistant message with a pending tool_call
    // whose result lives at end+1. Algorithm must walk back so we don't
    // orphan the call.
    let history = vec![
        user("u"),                                          // 0
        assistant_text("ok"),                               // 1 — start
        assistant_tool("t1", "Read", json!({"path": "a"})), // 2 — would-be end
        tool_result("t1", "ok"),                            // 3
        assistant_text("done"),                             // 4
    ];
    // retention=2 → end candidate = 5 - 2 - 1 = 2 (a tool_call). Walk back to 1.
    let r = find_compaction_range(&history, 2, 100).unwrap();
    assert_eq!(r.start, 1);
    assert_eq!(r.end, 1);
}

#[test]
fn splice_summary_replaces_range_with_one_user_message() {
    let history = vec![
        user("user prompt"), // 0 — preserved
        assistant_text("a"), // 1 — compacted
        user("u2"),          // 2 — compacted
        assistant_text("b"), // 3 — compacted
        user("u3"),          // 4 — preserved
        assistant_text("c"), // 5 — preserved
    ];
    let new_history = splice_summary(
        history,
        CompactionRange { start: 1, end: 3 },
        "SUMMARY HERE".to_string(),
    );
    assert_eq!(new_history.len(), 4);
    if let Message::User { content } = &new_history[1] {
        match content.first() {
            UserContent::Text(t) => assert_eq!(t.text, "SUMMARY HERE"),
            _ => panic!("expected user-text"),
        }
    } else {
        panic!("spliced message should be user-text");
    }
}

#[test]
fn evict_msg_count_zero_for_zero_fraction() {
    let history = vec![user("u"), assistant_text("a")];
    assert_eq!(evict_msg_count(&history, 0.0), 0);
}

#[test]
fn evict_msg_count_returns_index_for_full_fraction() {
    let history = vec![
        user("u"),
        assistant_text("a"),
        user("u2"),
        assistant_text("a2"),
    ];
    let idx = evict_msg_count(&history, 1.0);
    assert!(idx > 0);
}
