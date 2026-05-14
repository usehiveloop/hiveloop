use pretty_assertions::assert_eq;

#[test]
fn bridge_event_type_all_variants_roundtrip() {
    use crate::event::BridgeEventType;

    let variants = vec![
        (
            BridgeEventType::ConversationCreated,
            "\"conversation_created\"",
        ),
        (BridgeEventType::MessageReceived, "\"message_received\""),
        (BridgeEventType::ResponseStarted, "\"response_started\""),
        (BridgeEventType::ResponseChunk, "\"response_chunk\""),
        (BridgeEventType::ResponseCompleted, "\"response_completed\""),
        (BridgeEventType::ToolCallStarted, "\"tool_call_started\""),
        (
            BridgeEventType::ToolCallCompleted,
            "\"tool_call_completed\"",
        ),
        (BridgeEventType::ConversationEnded, "\"conversation_ended\""),
        (BridgeEventType::AgentError, "\"agent_error\""),
        (BridgeEventType::TodoUpdated, "\"todo_updated\""),
        (BridgeEventType::TurnCompleted, "\"turn_completed\""),
        (
            BridgeEventType::ToolApprovalRequired,
            "\"tool_approval_required\"",
        ),
        (
            BridgeEventType::ToolApprovalResolved,
            "\"tool_approval_resolved\"",
        ),
        (
            BridgeEventType::BackgroundTaskCompleted,
            "\"background_task_completed\"",
        ),
        (BridgeEventType::ReasoningStarted, "\"reasoning_started\""),
        (BridgeEventType::ReasoningDelta, "\"reasoning_delta\""),
        (
            BridgeEventType::ReasoningCompleted,
            "\"reasoning_completed\"",
        ),
        (BridgeEventType::SubAgentStarted, "\"sub_agent_started\""),
        (
            BridgeEventType::SubAgentCompleted,
            "\"sub_agent_completed\"",
        ),
        (BridgeEventType::Done, "\"done\""),
    ];

    for (variant, expected_json) in variants {
        let json = serde_json::to_string(&variant).expect("serialize BridgeEventType");
        assert_eq!(
            json, expected_json,
            "BridgeEventType::{:?} serialization",
            variant
        );

        let deserialized: BridgeEventType = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(variant, deserialized);
    }
}

#[test]
fn bridge_event_roundtrip() {
    use crate::event::{BridgeEvent, BridgeEventType};

    let event = BridgeEvent::new(
        BridgeEventType::ResponseChunk,
        "agent-1",
        "conv-123",
        serde_json::json!({"delta": "Hello world", "message_id": "msg-1"}),
    );

    let json = serde_json::to_string_pretty(&event).expect("serialize BridgeEvent");
    let deserialized: BridgeEvent = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(event.event_type, deserialized.event_type);
    assert_eq!(event.agent_id, deserialized.agent_id);
    assert_eq!(event.conversation_id, deserialized.conversation_id);
    assert_eq!(event.data, deserialized.data);
    assert_eq!(event.event_id, deserialized.event_id);
}

#[test]
fn bridge_event_json_shape() {
    use crate::event::{BridgeEvent, BridgeEventType};

    let event = BridgeEvent::new(
        BridgeEventType::ToolCallStarted,
        "agent-5",
        "conv-999",
        serde_json::json!({"name": "bash", "arguments": {"command": "ls"}}),
    );

    let json = serde_json::to_string_pretty(&event).expect("serialize");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");

    assert_eq!(value["event_type"], "tool_call_started");
    assert_eq!(value["agent_id"], "agent-5");
    assert_eq!(value["conversation_id"], "conv-999");
    assert!(value["timestamp"].is_string());
    assert!(value["event_id"].is_string());
    assert!(value["data"].is_object());
    assert_eq!(value["data"]["name"], "bash");
    // No secrets on the event
    assert!(value.get("webhook_url").is_none());
    assert!(value.get("webhook_secret").is_none());
}

#[test]
fn bridge_event_no_secrets_in_serialized_form() {
    use crate::event::{BridgeEvent, BridgeEventType};

    let event = BridgeEvent::new(
        BridgeEventType::ConversationCreated,
        "agent-1",
        "conv-1",
        serde_json::json!({}),
    );

    let json = serde_json::to_string(&event).expect("serialize");
    assert!(
        !json.contains("webhook_url"),
        "BridgeEvent must not contain webhook_url"
    );
    assert!(
        !json.contains("webhook_secret"),
        "BridgeEvent must not contain webhook_secret"
    );
}
