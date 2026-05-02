use pretty_assertions::assert_eq;

use crate::conversation::{ContentBlock, Message, Role, ToolCall, ToolResult};

#[test]
fn message_with_text_content_roundtrip() {
    let msg = Message {
        role: Role::User,
        content: vec![ContentBlock::Text {
            text: "What is Rust?".to_string(),
        }],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    };

    let json = serde_json::to_string_pretty(&msg).expect("serialize Message");
    let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(msg, deserialized);
}

#[test]
fn message_with_tool_call_content_roundtrip() {
    let msg = Message {
        role: Role::Assistant,
        content: vec![ContentBlock::ToolCall(ToolCall {
            id: "tc-100".to_string(),
            name: "read_file".to_string(),
            arguments: serde_json::json!({"path": "/etc/hosts"}),
        })],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    };

    let json = serde_json::to_string_pretty(&msg).expect("serialize");
    let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(msg, deserialized);
}

#[test]
fn message_with_tool_result_content_roundtrip() {
    let msg = Message {
        role: Role::Tool,
        content: vec![ContentBlock::ToolResult(ToolResult {
            tool_call_id: "tc-100".to_string(),
            content: "127.0.0.1 localhost".to_string(),
            is_error: false,
        })],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    };

    let json = serde_json::to_string_pretty(&msg).expect("serialize");
    let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(msg, deserialized);
}

#[test]
fn message_with_image_content_roundtrip() {
    let msg = Message {
        role: Role::User,
        content: vec![ContentBlock::Image {
            media_type: "image/jpeg".to_string(),
            data: "/9j/4AAQSkZJRg==".to_string(),
        }],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    };

    let json = serde_json::to_string_pretty(&msg).expect("serialize");
    let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(msg, deserialized);
}

#[test]
fn message_with_multiple_content_blocks_roundtrip() {
    let msg = Message {
        role: Role::Assistant,
        content: vec![
            ContentBlock::Text {
                text: "Let me check that file.".to_string(),
            },
            ContentBlock::ToolCall(ToolCall {
                id: "tc-200".to_string(),
                name: "read_file".to_string(),
                arguments: serde_json::json!({"path": "/tmp/test.txt"}),
            }),
        ],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    };

    let json = serde_json::to_string_pretty(&msg).expect("serialize");
    let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(msg, deserialized);
    assert_eq!(deserialized.content.len(), 2);
}

#[test]
fn message_with_empty_content_roundtrip() {
    let msg = Message {
        role: Role::System,
        content: vec![],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    };

    let json = serde_json::to_string_pretty(&msg).expect("serialize");
    let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(msg, deserialized);
    assert!(deserialized.content.is_empty());
}

// ──────────────────────────────────────────────
// MetricsSnapshot
// ──────────────────────────────────────────────
