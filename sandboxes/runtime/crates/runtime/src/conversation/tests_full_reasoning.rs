//! Verifies the contract that `ResponseCompleted` carries `full_reasoning`
//! alongside `full_response`, so downstream consumers can drop per-token
//! `reasoning_delta` events from persistent storage without losing reasoning.

use std::sync::Arc;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::metrics::ConversationMetrics;
use tokio::sync::mpsc;
use webhooks::EventBus;

use super::finalize::emit_turn_complete_events;
use super::stream_loop::{attempt_into_result, StreamAttempt};

fn make_event_bus() -> (Arc<EventBus>, mpsc::UnboundedReceiver<BridgeEvent>) {
    let (tx, rx) = mpsc::unbounded_channel();
    let bus = Arc::new(EventBus::new(
        Some(tx),
        None,
        "https://test.invalid/webhook".to_string(),
        "runtime-test-token".to_string(),
    ));
    (bus, rx)
}

fn drain(rx: &mut mpsc::UnboundedReceiver<BridgeEvent>) -> Vec<BridgeEvent> {
    let mut out = Vec::new();
    while let Ok(ev) = rx.try_recv() {
        out.push(ev);
    }
    out
}

#[tokio::test]
async fn response_completed_payload_includes_full_reasoning() {
    let (bus, mut rx) = make_event_bus();
    let metrics = Arc::new(ConversationMetrics::new(
        "conv-1".into(),
        "agent-1".into(),
        "test-model".into(),
    ));

    emit_turn_complete_events(
        &bus,
        "agent-1",
        "conv-1",
        "msg-1",
        "the answer is 42",
        "Step 1: parse the question. Step 2: compute. Step 3: emit.",
        123,
        100,
        50,
        25,
        &metrics,
        1,
        &[],
        &None,
    )
    .await;

    let events = drain(&mut rx);
    let completed = events
        .iter()
        .find(|e| matches!(e.event_type, BridgeEventType::ResponseCompleted))
        .expect("ResponseCompleted should be emitted");

    let reasoning = completed
        .data
        .get("full_reasoning")
        .and_then(|v| v.as_str())
        .expect("full_reasoning key should be present and a string");

    assert_eq!(
        reasoning, "Step 1: parse the question. Step 2: compute. Step 3: emit.",
        "full_reasoning must round-trip the assembled reasoning text"
    );

    let response = completed
        .data
        .get("full_response")
        .and_then(|v| v.as_str())
        .expect("full_response key still present");
    assert_eq!(response, "the answer is 42");
}

#[tokio::test]
async fn response_completed_with_empty_reasoning_emits_empty_string_not_null() {
    // Non-streaming providers and models that don't reason populate the field
    // with "" — downstream filters that drop reasoning_delta still get a
    // present-but-empty key, never a missing one.
    let (bus, mut rx) = make_event_bus();
    let metrics = Arc::new(ConversationMetrics::new(
        "conv-2".into(),
        "agent-2".into(),
        "test-model".into(),
    ));

    emit_turn_complete_events(
        &bus, "agent-2", "conv-2", "msg-2", "ok", "", 1, 0, 0, 0, &metrics, 1, &[], &None,
    )
    .await;

    let events = drain(&mut rx);
    let completed = events
        .iter()
        .find(|e| matches!(e.event_type, BridgeEventType::ResponseCompleted))
        .expect("ResponseCompleted should be emitted");

    let reasoning = completed
        .data
        .get("full_reasoning")
        .expect("key must be present even when empty");
    assert_eq!(
        reasoning.as_str(),
        Some(""),
        "empty reasoning should be \"\", not null"
    );
}

#[test]
fn attempt_into_result_propagates_accumulated_reasoning() {
    // The other half of the contract: StreamAttempt's accumulated reasoning
    // must survive the conversion into PromptResponse so the upper layers
    // (turn_classify → turn_success → emit_turn_complete_events) see it.
    let attempt = StreamAttempt {
        accumulated_text: "answer".into(),
        accumulated_reasoning: "thinking out loud".into(),
        final_usage: rig::completion::Usage::new(),
        final_history: Some(vec![]),
        had_error: None,
        any_progress: true,
        hook_cancellation: None,
    };

    let (result, _history) = attempt_into_result(attempt, &[], "user said");
    let response = result.expect("happy path should yield Ok");
    assert_eq!(response.reasoning, "thinking out loud");
    assert_eq!(response.output, "answer");
}

#[test]
fn attempt_into_result_propagates_reasoning_through_parse_error_recovery() {
    // The parse-error recovery branch (`no message or tool call`, untagged
    // enum) also funnels through the Ok(PromptResponse{...}) path. Reasoning
    // accumulated up to the parse failure must still be carried.
    let attempt = StreamAttempt {
        accumulated_text: "partial".into(),
        accumulated_reasoning: "halfway through".into(),
        final_usage: rig::completion::Usage::new(),
        final_history: Some(vec![]),
        had_error: Some("no message or tool call in response".into()),
        any_progress: true,
        hook_cancellation: None,
    };

    let (result, _history) = attempt_into_result(attempt, &[], "user said");
    let response = result.expect("recoverable parse error should still yield Ok");
    assert_eq!(response.reasoning, "halfway through");
    assert_eq!(response.output, "partial");
}
