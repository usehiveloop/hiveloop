use pretty_assertions::assert_eq;

use crate::conversation::{ContentBlock, Role, ToolCall, ToolResult};

#[test]
fn role_all_variants_roundtrip() {
    let roles = [Role::User, Role::Assistant, Role::System, Role::Tool];

    let expected_json = ["\"user\"", "\"assistant\"", "\"system\"", "\"tool\""];

    for (role, expected) in roles.iter().zip(expected_json.iter()) {
        let json = serde_json::to_string(role).expect("serialize Role");
        assert_eq!(&json, expected, "Role::{:?} serialization", role);

        let deserialized: Role = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(role, &deserialized);
    }
}

// ──────────────────────────────────────────────
// ContentBlock
// ──────────────────────────────────────────────

#[test]
fn content_block_text_roundtrip() {
    let block = ContentBlock::Text {
        text: "Hello, world!".to_string(),
    };

    let json = serde_json::to_string_pretty(&block).expect("serialize");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["type"], "text");
    assert_eq!(value["text"], "Hello, world!");

    let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(block, deserialized);
}

#[test]
fn content_block_tool_call_roundtrip() {
    let block = ContentBlock::ToolCall(ToolCall {
        id: "call-123".to_string(),
        name: "calculator".to_string(),
        arguments: serde_json::json!({"expression": "2 + 2"}),
    });

    let json = serde_json::to_string_pretty(&block).expect("serialize");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["type"], "tool_call");
    assert_eq!(value["id"], "call-123");
    assert_eq!(value["name"], "calculator");

    let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(block, deserialized);
}

#[test]
fn content_block_tool_result_roundtrip() {
    let block = ContentBlock::ToolResult(ToolResult {
        tool_call_id: "call-123".to_string(),
        content: "4".to_string(),
        is_error: false,
    });

    let json = serde_json::to_string_pretty(&block).expect("serialize");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["type"], "tool_result");
    assert_eq!(value["tool_call_id"], "call-123");
    assert_eq!(value["content"], "4");

    let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(block, deserialized);
}

#[test]
fn content_block_tool_result_with_error_roundtrip() {
    let block = ContentBlock::ToolResult(ToolResult {
        tool_call_id: "call-456".to_string(),
        content: "Division by zero".to_string(),
        is_error: true,
    });

    let json = serde_json::to_string_pretty(&block).expect("serialize");
    let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(block, deserialized);

    if let ContentBlock::ToolResult(result) = &deserialized {
        assert!(result.is_error);
    } else {
        panic!("Expected ToolResult variant");
    }
}

#[test]
fn content_block_tool_result_is_error_defaults_to_false() {
    let json = r#"{"type": "tool_result", "tool_call_id": "call-789", "content": "ok"}"#;
    let block: ContentBlock = serde_json::from_str(json).expect("deserialize");
    if let ContentBlock::ToolResult(result) = &block {
        assert!(!result.is_error);
    } else {
        panic!("Expected ToolResult variant");
    }
}

#[test]
fn content_block_image_roundtrip() {
    let block = ContentBlock::Image {
        media_type: "image/png".to_string(),
        data: "iVBORw0KGgoAAAANSUhEUg==".to_string(),
    };

    let json = serde_json::to_string_pretty(&block).expect("serialize");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["type"], "image");
    assert_eq!(value["media_type"], "image/png");

    let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(block, deserialized);
}

// ──────────────────────────────────────────────
// ToolCall & ToolResult (standalone)
// ──────────────────────────────────────────────

#[test]
fn tool_call_roundtrip() {
    let call = ToolCall {
        id: "tc-1".to_string(),
        name: "web_search".to_string(),
        arguments: serde_json::json!({"query": "Rust serde", "limit": 10}),
    };

    let json = serde_json::to_string_pretty(&call).expect("serialize");
    let deserialized: ToolCall = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(call, deserialized);
}

#[test]
fn tool_result_roundtrip() {
    let result = ToolResult {
        tool_call_id: "tc-1".to_string(),
        content: "Found 42 results".to_string(),
        is_error: false,
    };

    let json = serde_json::to_string_pretty(&result).expect("serialize");
    let deserialized: ToolResult = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(result, deserialized);
}

// ──────────────────────────────────────────────
// Message
// ──────────────────────────────────────────────
