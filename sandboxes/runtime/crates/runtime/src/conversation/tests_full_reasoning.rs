use std::sync::Arc;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::metrics::ConversationMetrics;
use tokio::sync::mpsc;
use webhooks::EventBus;

use super::finalize::emit_turn_complete_events;
use super::stream_loop::{
    attempt_into_result, handle_reasoning_delta, handle_text_delta, maybe_close_reasoning,
    StreamAttempt,
};

fn fresh_attempt() -> StreamAttempt {
    StreamAttempt {
        accumulated_text: String::new(),
        accumulated_reasoning: String::new(),
        reasoning_active: false,
        final_usage: rig::completion::Usage::new(),
        final_history: None,
        had_error: None,
        any_progress: false,
        hook_cancellation: None,
    }
}

fn event_types(events: &[BridgeEvent]) -> Vec<&BridgeEventType> {
    events.iter().map(|e| &e.event_type).collect()
}

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
    let (bus, mut rx) = make_event_bus();
    let metrics = Arc::new(ConversationMetrics::new(
        "conv-2".into(),
        "agent-2".into(),
        "test-model".into(),
    ));

    emit_turn_complete_events(
        &bus,
        "agent-2",
        "conv-2",
        "msg-2",
        "ok",
        "",
        1,
        0,
        0,
        0,
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
        .expect("key must be present even when empty");
    assert_eq!(
        reasoning.as_str(),
        Some(""),
        "empty reasoning should be \"\", not null"
    );
}

#[test]
fn attempt_into_result_propagates_accumulated_reasoning() {
    let attempt = StreamAttempt {
        accumulated_text: "answer".into(),
        accumulated_reasoning: "thinking out loud".into(),
        reasoning_active: false,
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
    let attempt = StreamAttempt {
        accumulated_text: "partial".into(),
        accumulated_reasoning: "halfway through".into(),
        reasoning_active: false,
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

#[tokio::test]
async fn lifecycle_reasoning_then_text_emits_started_completed_in_order() {
    let (bus, mut rx) = make_event_bus();
    let mut state = fresh_attempt();

    handle_reasoning_delta(&mut state, &bus, "a", "c", "m", "thinking ".into());
    handle_reasoning_delta(&mut state, &bus, "a", "c", "m", "more.".into());
    handle_text_delta(&mut state, &bus, "a", "c", "m", "answer ".into());
    handle_text_delta(&mut state, &bus, "a", "c", "m", "here.".into());
    maybe_close_reasoning(&mut state, &bus, "a", "c", "m");

    let events = drain(&mut rx);
    let types = event_types(&events);
    assert_eq!(
        types,
        vec![
            &BridgeEventType::ReasoningStarted,
            &BridgeEventType::ReasoningDelta,
            &BridgeEventType::ReasoningDelta,
            &BridgeEventType::ReasoningCompleted,
            &BridgeEventType::ResponseChunk,
            &BridgeEventType::ResponseChunk,
        ],
        "reasoning lifecycle must close before any response chunk emits"
    );

    let completed = events
        .iter()
        .find(|e| matches!(e.event_type, BridgeEventType::ReasoningCompleted))
        .unwrap();
    assert_eq!(
        completed
            .data
            .get("full_reasoning")
            .and_then(|v| v.as_str()),
        Some("thinking more.")
    );
    assert!(!state.reasoning_active);
}

#[tokio::test]
async fn lifecycle_reasoning_only_closes_on_loop_end() {
    let (bus, mut rx) = make_event_bus();
    let mut state = fresh_attempt();

    handle_reasoning_delta(&mut state, &bus, "a", "c", "m", "thought.".into());
    maybe_close_reasoning(&mut state, &bus, "a", "c", "m");

    let events = drain(&mut rx);
    assert_eq!(
        event_types(&events),
        vec![
            &BridgeEventType::ReasoningStarted,
            &BridgeEventType::ReasoningDelta,
            &BridgeEventType::ReasoningCompleted,
        ]
    );
}

#[tokio::test]
async fn lifecycle_text_only_emits_no_reasoning_events() {
    let (bus, mut rx) = make_event_bus();
    let mut state = fresh_attempt();

    handle_text_delta(&mut state, &bus, "a", "c", "m", "hi".into());
    maybe_close_reasoning(&mut state, &bus, "a", "c", "m");

    let events = drain(&mut rx);
    assert_eq!(event_types(&events), vec![&BridgeEventType::ResponseChunk]);
}

#[tokio::test]
async fn lifecycle_double_close_is_idempotent() {
    let (bus, mut rx) = make_event_bus();
    let mut state = fresh_attempt();

    handle_reasoning_delta(&mut state, &bus, "a", "c", "m", "x".into());
    handle_text_delta(&mut state, &bus, "a", "c", "m", "y".into());
    handle_text_delta(&mut state, &bus, "a", "c", "m", "z".into());
    maybe_close_reasoning(&mut state, &bus, "a", "c", "m");

    let events = drain(&mut rx);
    assert_eq!(
        event_types(&events),
        vec![
            &BridgeEventType::ReasoningStarted,
            &BridgeEventType::ReasoningDelta,
            &BridgeEventType::ReasoningCompleted,
            &BridgeEventType::ResponseChunk,
            &BridgeEventType::ResponseChunk,
        ]
    );
}
