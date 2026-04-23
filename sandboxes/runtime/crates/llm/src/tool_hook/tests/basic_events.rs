//! Basic on_tool_call / on_tool_result event emission tests.

use bridge_core::event::BridgeEventType;
use rig::agent::{HookAction, PromptHook, ToolCallHookAction};

use super::support::{make_bus, make_emitter, TestModel};

#[tokio::test]
async fn test_emitter_sends_tool_call_start() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let emitter = make_emitter(bus);

    let action = PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "web_search",
        Some("call_123".to_string()),
        "int_456",
        r#"{"query":"test"}"#,
    )
    .await;

    assert_eq!(action, ToolCallHookAction::Continue);

    let event = ws_rx.try_recv().expect("should have received an event");
    assert_eq!(event.event_type, BridgeEventType::ToolCallStarted);
    assert_eq!(event.data["id"], "call_123");
    assert_eq!(event.data["name"], "web_search");
    assert_eq!(
        event.data["arguments"],
        serde_json::json!({"query": "test"})
    );
}

#[tokio::test]
async fn test_emitter_sends_tool_call_result() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let emitter = make_emitter(bus);

    let action = PromptHook::<TestModel>::on_tool_result(
        &emitter,
        "web_search",
        Some("call_123".to_string()),
        "int_456",
        r#"{"query":"test"}"#,
        r#"{"results": ["page1"]}"#,
    )
    .await;

    assert_eq!(action, HookAction::cont());

    let event = ws_rx.try_recv().expect("should have received an event");
    assert_eq!(event.event_type, BridgeEventType::ToolCallCompleted);
    assert_eq!(event.data["id"], "call_123");
    assert_eq!(event.data["result"], r#"{"results": ["page1"]}"#);
    assert_eq!(event.data["is_error"], false);
}

#[tokio::test]
async fn test_emitter_returns_continue() {
    let bus = make_bus();
    let emitter = make_emitter(bus);

    let tool_action =
        PromptHook::<TestModel>::on_tool_call(&emitter, "test_tool", None, "internal_1", "{}")
            .await;
    assert_eq!(tool_action, ToolCallHookAction::Continue);

    let result_action = PromptHook::<TestModel>::on_tool_result(
        &emitter,
        "test_tool",
        None,
        "internal_1",
        "{}",
        "ok",
    )
    .await;
    assert_eq!(result_action, HookAction::cont());
}

#[tokio::test]
async fn test_emitter_uses_internal_call_id_when_no_tool_call_id() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let emitter = make_emitter(bus);

    PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "my_tool",
        None, // no tool_call_id
        "internal_99",
        "{}",
    )
    .await;

    let event = ws_rx.try_recv().expect("should have received an event");
    assert_eq!(event.event_type, BridgeEventType::ToolCallStarted);
    assert_eq!(event.data["id"], "internal_99");
}

#[tokio::test]
async fn test_emitter_handles_invalid_json_args() {
    let bus = make_bus();
    let mut ws_rx = bus.subscribe_ws();
    let emitter = make_emitter(bus);

    PromptHook::<TestModel>::on_tool_call(
        &emitter,
        "my_tool",
        Some("call_1".to_string()),
        "int_1",
        "not valid json",
    )
    .await;

    let event = ws_rx.try_recv().expect("should have received an event");
    assert_eq!(event.event_type, BridgeEventType::ToolCallStarted);
    assert_eq!(event.data["arguments"], "not valid json");
}
