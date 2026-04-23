use super::convert::convert_to_rig_message;
use bridge_core::conversation::{ContentBlock, Message, Role, ToolCall, ToolResult};
use serde_json::json;

fn make_message(role: Role, content: Vec<ContentBlock>) -> Message {
    Message {
        role,
        content,
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

#[test]
fn test_convert_user_text_message() {
    let msg = make_message(
        Role::User,
        vec![ContentBlock::Text {
            text: "hello".into(),
        }],
    );
    let rig_msg = convert_to_rig_message(&msg).unwrap();
    assert_eq!(rig_msg, rig::message::Message::user("hello"));
}

#[test]
fn test_convert_assistant_text_message() {
    let msg = make_message(
        Role::Assistant,
        vec![ContentBlock::Text {
            text: "hi there".into(),
        }],
    );
    let rig_msg = convert_to_rig_message(&msg).unwrap();
    assert_eq!(rig_msg, rig::message::Message::assistant("hi there"));
}

#[test]
fn test_convert_assistant_with_tool_call() {
    let msg = make_message(
        Role::Assistant,
        vec![
            ContentBlock::Text {
                text: "Let me read that file.".into(),
            },
            ContentBlock::ToolCall(ToolCall {
                id: "call_001".into(),
                name: "read_file".into(),
                arguments: json!({"path": "src/main.rs"}),
            }),
        ],
    );
    let rig_msg = convert_to_rig_message(&msg).unwrap();
    match &rig_msg {
        rig::message::Message::Assistant { content, .. } => {
            assert_eq!(content.iter().count(), 2);
        }
        _ => panic!("expected Assistant message"),
    }
}

#[test]
fn test_convert_assistant_tool_call_only() {
    let msg = make_message(
        Role::Assistant,
        vec![ContentBlock::ToolCall(ToolCall {
            id: "call_002".into(),
            name: "bash".into(),
            arguments: json!({"command": "ls"}),
        })],
    );
    let rig_msg = convert_to_rig_message(&msg).unwrap();
    match &rig_msg {
        rig::message::Message::Assistant { content, .. } => {
            assert_eq!(content.iter().count(), 1);
        }
        _ => panic!("expected Assistant message"),
    }
}

#[test]
fn test_convert_tool_result_message() {
    let msg = make_message(
        Role::Tool,
        vec![ContentBlock::ToolResult(ToolResult {
            tool_call_id: "call_001".into(),
            content: "file contents here".into(),
            is_error: false,
        })],
    );
    let rig_msg = convert_to_rig_message(&msg).unwrap();
    assert_eq!(
        rig_msg,
        rig::message::Message::tool_result("call_001", "file contents here")
    );
}

#[test]
fn test_convert_system_message_returns_none() {
    let msg = make_message(
        Role::System,
        vec![ContentBlock::Text {
            text: "system prompt".into(),
        }],
    );
    assert!(convert_to_rig_message(&msg).is_none());
}

#[test]
fn test_convert_empty_assistant_returns_none() {
    let msg = make_message(Role::Assistant, vec![]);
    assert!(convert_to_rig_message(&msg).is_none());
}

#[test]
fn test_convert_empty_user_returns_none() {
    let msg = make_message(Role::User, vec![]);
    assert!(convert_to_rig_message(&msg).is_none());
}

#[test]
fn test_convert_empty_tool_returns_none() {
    let msg = make_message(Role::Tool, vec![]);
    assert!(convert_to_rig_message(&msg).is_none());
}

#[test]
fn test_convert_assistant_multiple_tool_calls() {
    let msg = make_message(
        Role::Assistant,
        vec![
            ContentBlock::ToolCall(ToolCall {
                id: "call_a".into(),
                name: "read_file".into(),
                arguments: json!({"path": "a.rs"}),
            }),
            ContentBlock::ToolCall(ToolCall {
                id: "call_b".into(),
                name: "read_file".into(),
                arguments: json!({"path": "b.rs"}),
            }),
        ],
    );
    let rig_msg = convert_to_rig_message(&msg).unwrap();
    match &rig_msg {
        rig::message::Message::Assistant { content, .. } => {
            assert_eq!(content.iter().count(), 2);
        }
        _ => panic!("expected Assistant message"),
    }
}
