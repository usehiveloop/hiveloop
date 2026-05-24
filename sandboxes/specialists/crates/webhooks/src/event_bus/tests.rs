use super::*;
use bridge_core::event::BridgeEventType;

fn make_event(conv_id: &str) -> BridgeEvent {
    BridgeEvent::new(
        BridgeEventType::ConversationCreated,
        "agent-1",
        conv_id,
        serde_json::json!({}),
    )
}

#[test]
fn test_emit_stamps_monotonic_sequence_numbers() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    let mut ws_rx = bus.subscribe_ws();

    bus.emit(make_event("conv-1"));
    bus.emit(make_event("conv-2"));
    bus.emit(make_event("conv-1"));

    let e1 = ws_rx.try_recv().unwrap();
    let e2 = ws_rx.try_recv().unwrap();
    let e3 = ws_rx.try_recv().unwrap();

    assert_eq!(e1.sequence_number, 1);
    assert_eq!(e2.sequence_number, 2);
    assert_eq!(e3.sequence_number, 3);
}

#[test]
fn test_ws_and_sse_receive_same_event() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    let mut ws_rx = bus.subscribe_ws();
    let mut sse_rx = bus.subscribe_sse("conv-1");

    bus.emit(make_event("conv-1"));

    let ws_event = ws_rx.try_recv().unwrap();
    let sse_event = sse_rx.try_recv().unwrap();

    assert_eq!(ws_event.event_id, sse_event.event_id);
    assert_eq!(ws_event.sequence_number, sse_event.sequence_number);
    assert_eq!(ws_event.event_type, sse_event.event_type);
    assert_eq!(ws_event.agent_id, sse_event.agent_id);
    assert_eq!(ws_event.conversation_id, sse_event.conversation_id);
    assert_eq!(ws_event.data, sse_event.data);
}

#[test]
fn test_sse_only_receives_matching_conversation() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    let mut sse_rx = bus.subscribe_sse("conv-1");

    bus.emit(make_event("conv-2")); // different conversation
    bus.emit(make_event("conv-1")); // matching conversation

    // Should only receive the conv-1 event
    let event = sse_rx.try_recv().unwrap();
    assert_eq!(event.conversation_id, "conv-1");
    assert_eq!(event.sequence_number, 2);

    // No more events
    assert!(sse_rx.try_recv().is_err());
}

#[test]
fn test_webhook_channel_receives_events() {
    let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
    let bus = EventBus::new(
        Some(webhook_tx),
        None,
        "https://example.com".to_string(),
        "secret".to_string(),
    );

    bus.emit(make_event("conv-1"));
    bus.emit(make_event("conv-2"));

    let e1 = webhook_rx.try_recv().unwrap();
    let e2 = webhook_rx.try_recv().unwrap();

    assert_eq!(e1.sequence_number, 1);
    assert_eq!(e2.sequence_number, 2);
    assert_eq!(e1.conversation_id, "conv-1");
    assert_eq!(e2.conversation_id, "conv-2");
}

#[test]
fn test_all_channels_get_identical_data() {
    let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
    let bus = EventBus::new(
        Some(webhook_tx),
        None,
        "https://example.com".to_string(),
        "secret".to_string(),
    );
    let mut ws_rx = bus.subscribe_ws();
    let mut sse_rx = bus.subscribe_sse("conv-1");

    let event = BridgeEvent::new(
        BridgeEventType::ResponseChunk,
        "agent-1",
        "conv-1",
        serde_json::json!({"delta": "Hello", "message_id": "msg-1"}),
    );
    bus.emit(event);

    let ws = ws_rx.try_recv().unwrap();
    let sse = sse_rx.try_recv().unwrap();
    let wh = webhook_rx.try_recv().unwrap();

    assert_eq!(ws.event_id, sse.event_id);
    assert_eq!(sse.event_id, wh.event_id);
    assert_eq!(ws.sequence_number, 1);
    assert_eq!(sse.sequence_number, 1);
    assert_eq!(wh.sequence_number, 1);
    assert_eq!(ws.data, sse.data);
    assert_eq!(sse.data, wh.data);
    assert_eq!(ws.event_type, sse.event_type);
    assert_eq!(sse.event_type, wh.event_type);
}

#[test]
fn test_remove_sse_stream() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    bus.register_sse_stream("conv-1".to_string());
    let _sub = bus.subscribe_sse("conv-1");
    assert_eq!(bus.sse_stream_count(), 1);

    bus.remove_sse_stream("conv-1");
    assert_eq!(bus.sse_stream_count(), 0);
}

#[test]
fn test_emitted_count() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    assert_eq!(bus.emitted_count(), 0);

    bus.emit(make_event("conv-1"));
    bus.emit(make_event("conv-2"));
    assert_eq!(bus.emitted_count(), 2);
}

#[test]
fn test_emit_without_any_subscribers_does_not_panic() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    bus.emit(make_event("conv-1"));
    assert_eq!(bus.emitted_count(), 1);
    assert_eq!(bus.current_sequence(), 1);
}

#[test]
fn test_emit_replayed_only_targets_webhook() {
    let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
    let bus = EventBus::new(Some(webhook_tx), None, String::new(), String::new());
    let mut ws_rx = bus.subscribe_ws();
    let mut sse_rx = bus.subscribe_sse("conv-1");

    let mut event = make_event("conv-1");
    event.sequence_number = 42;
    bus.emit_replayed(event);

    // Live channels do not receive replays.
    assert!(ws_rx.try_recv().is_err());
    assert!(sse_rx.try_recv().is_err());

    let wh = webhook_rx.try_recv().unwrap();
    assert_eq!(wh.sequence_number, 42);
    assert_eq!(bus.current_sequence(), 0);
}

#[test]
fn test_multiple_sse_streams_independent() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    let mut sse_a = bus.subscribe_sse("conv-a");
    let mut sse_b = bus.subscribe_sse("conv-b");

    bus.emit(make_event("conv-a"));
    bus.emit(make_event("conv-b"));
    bus.emit(make_event("conv-a"));

    let a1 = sse_a.try_recv().unwrap();
    let a2 = sse_a.try_recv().unwrap();
    assert!(sse_a.try_recv().is_err());
    assert_eq!(a1.sequence_number, 1);
    assert_eq!(a2.sequence_number, 3);

    let b1 = sse_b.try_recv().unwrap();
    assert!(sse_b.try_recv().is_err());
    assert_eq!(b1.sequence_number, 2);
}

#[test]
fn test_multiple_subscribers_same_conversation() {
    // The whole point of the broadcast refactor: multiple SSE clients
    // attached to the same conversation each receive every event.
    let bus = EventBus::new(None, None, String::new(), String::new());
    let mut sub_a = bus.subscribe_sse("conv-1");
    let mut sub_b = bus.subscribe_sse("conv-1");
    let mut sub_c = bus.subscribe_sse("conv-1");

    bus.emit(make_event("conv-1"));
    bus.emit(make_event("conv-1"));

    for sub in [&mut sub_a, &mut sub_b, &mut sub_c] {
        let e1 = sub.try_recv().unwrap();
        let e2 = sub.try_recv().unwrap();
        assert_eq!(e1.sequence_number, 1);
        assert_eq!(e2.sequence_number, 2);
        assert!(sub.try_recv().is_err());
    }
}

#[test]
fn test_late_subscriber_misses_prior_events() {
    // Standard broadcast semantics: subscribers only see events emitted
    // *after* they subscribed. Resume from a gap is the Last-Event-ID
    // path's job, not the live channel's.
    let bus = EventBus::new(None, None, String::new(), String::new());
    bus.register_sse_stream("conv-1".to_string());
    bus.emit(make_event("conv-1"));

    let mut late = bus.subscribe_sse("conv-1");
    bus.emit(make_event("conv-1"));

    let only = late.try_recv().unwrap();
    assert_eq!(only.sequence_number, 2);
    assert!(late.try_recv().is_err());
}

#[test]
fn test_no_secrets_on_event() {
    let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
    let bus = EventBus::new(
        Some(webhook_tx),
        None,
        "https://secret-url.com".to_string(),
        "webhook-signing-test-key".to_string(),
    );

    bus.emit(make_event("conv-1"));

    let event = webhook_rx.try_recv().unwrap();
    let json = serde_json::to_value(&event).unwrap();
    let obj = json.as_object().unwrap();

    assert!(!obj.contains_key("webhook_url"));
    assert!(!obj.contains_key("webhook_secret"));
}

#[test]
fn test_event_json_shape() {
    let bus = EventBus::new(None, None, String::new(), String::new());
    let mut ws_rx = bus.subscribe_ws();

    bus.emit(BridgeEvent::new(
        BridgeEventType::ToolCallStarted,
        "agent-5",
        "conv-99",
        serde_json::json!({"name": "bash", "arguments": {"command": "ls"}}),
    ));

    let event = ws_rx.try_recv().unwrap();
    let json = serde_json::to_value(&event).unwrap();

    assert_eq!(json["event_type"], "tool_call_started");
    assert_eq!(json["agent_id"], "agent-5");
    assert_eq!(json["conversation_id"], "conv-99");
    assert_eq!(json["sequence_number"], 1);
    assert!(json["event_id"].is_string());
    assert!(json["timestamp"].is_string());
    assert_eq!(json["data"]["name"], "bash");
}
