#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{TestHarness, WsEventStream};
use std::time::Duration;

const TIMEOUT: Duration = Duration::from_secs(30);

// ============================================================================
// Authentication tests
// ============================================================================

#[tokio::test]
async fn test_ws_connection_with_valid_token() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key").await;
    assert!(ws.is_ok(), "should connect with valid token");
}

#[tokio::test]
async fn test_ws_connection_rejects_invalid_token() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "wrong-token").await;
    assert!(ws.is_err(), "should reject invalid token");
}

#[tokio::test]
async fn test_ws_connection_rejects_missing_token() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    // Connect with empty token
    let ws = WsEventStream::connect(harness.bridge_url(), "").await;
    assert!(ws.is_err(), "should reject empty token");
}

#[tokio::test]
async fn test_ws_receives_conversation_lifecycle_events() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Create a conversation and send a message
    let resp = harness
        .create_conversation("agent_simple")
        .await
        .expect("create_conversation failed");
    assert_eq!(resp.status().as_u16(), 201);

    let body: serde_json::Value = resp.json().await.expect("failed to parse body");
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness
        .send_message(conv_id, "Hello!")
        .await
        .expect("send_message failed");

    // Wait for SSE to finish (so we know the turn is done)
    let _ = harness
        .stream_sse_until_done(conv_id, TIMEOUT)
        .await
        .expect("SSE stream failed");

    // Wait for WS events to arrive (they're dispatched in parallel with SSE)
    let ws_events = ws.wait_for_event_type("turn_completed", TIMEOUT).await;
    assert!(
        ws_events.is_some(),
        "should receive turn_completed over WebSocket"
    );

    let all_events = ws.events();

    // Check that lifecycle events are present
    let event_types: Vec<String> = all_events
        .iter()
        .filter_map(|e| e.event_type().map(String::from))
        .collect();

    assert!(
        event_types.contains(&"conversation_created".to_string()),
        "should have conversation_created; got: {:?}",
        event_types
    );
    assert!(
        event_types.contains(&"message_received".to_string()),
        "should have message_received; got: {:?}",
        event_types
    );
    assert!(
        event_types.contains(&"response_started".to_string()),
        "should have response_started; got: {:?}",
        event_types
    );
    assert!(
        event_types.contains(&"turn_completed".to_string()),
        "should have turn_completed; got: {:?}",
        event_types
    );

    // Every event must have agent_id, conversation_id, and sequence_number
    for event in &all_events {
        if event.is_lagged() {
            continue;
        }
        assert!(
            event.agent_id().is_some(),
            "WS event should have agent_id: {:?}",
            event.data
        );
        assert!(
            event.conversation_id().is_some(),
            "WS event should have conversation_id: {:?}",
            event.data
        );
        assert!(
            event.sequence_number().is_some(),
            "WS event should have sequence_number: {:?}",
            event.data
        );
    }
}

#[tokio::test]
async fn test_ws_events_exclude_sensitive_fields() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Create a conversation to generate events
    let resp = harness
        .create_conversation("agent_simple")
        .await
        .expect("create_conversation failed");
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness.send_message(conv_id, "Hello!").await.unwrap();
    let _ = harness.stream_sse_until_done(conv_id, TIMEOUT).await;

    ws.wait_for_event_type("turn_completed", TIMEOUT).await;

    // Verify no webhook_url or webhook_secret in any event
    for event in &ws.events() {
        assert!(
            event.data.get("webhook_url").is_none(),
            "WS event must not contain webhook_url: {:?}",
            event.data
        );
        assert!(
            event.data.get("webhook_secret").is_none(),
            "WS event must not contain webhook_secret: {:?}",
            event.data
        );
    }
}

#[tokio::test]
async fn test_ws_events_match_webhook_events() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    harness.clear_webhook_log().await.unwrap();

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Run a conversation
    let resp = harness.create_conversation("agent_simple").await.unwrap();
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness.send_message(conv_id, "Hello!").await.unwrap();
    let _ = harness.stream_sse_until_done(conv_id, TIMEOUT).await;

    // Wait for both WS and webhook events
    ws.wait_for_event_type("turn_completed", TIMEOUT).await;
    let webhook_log = harness
        .wait_for_webhook_type("turn_completed", Duration::from_secs(10))
        .await
        .unwrap();

    // Collect event types from both
    let mut ws_types: Vec<String> = ws
        .events()
        .iter()
        .filter_map(|e| e.event_type().map(String::from))
        .collect();
    ws_types.sort();
    ws_types.dedup();

    let mut wh_types = webhook_log.unique_event_types();
    wh_types.sort();

    // Both should have the same set of event types
    assert_eq!(
        ws_types, wh_types,
        "WS and webhook should produce the same event types"
    );
}

#[tokio::test]
async fn test_ws_multiplexes_multiple_conversations() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Create 3 conversations
    let mut conv_ids = Vec::new();
    for _ in 0..3 {
        let resp = harness.create_conversation("agent_simple").await.unwrap();
        let body: serde_json::Value = resp.json().await.unwrap();
        conv_ids.push(body["conversation_id"].as_str().unwrap().to_string());
    }

    // Send a message to each
    for conv_id in &conv_ids {
        harness.send_message(conv_id, "Hello!").await.unwrap();
    }

    // Wait for all turns to complete via SSE
    for conv_id in &conv_ids {
        let _ = harness.stream_sse_until_done(conv_id, TIMEOUT).await;
    }

    // Give WS a moment to catch up
    tokio::time::sleep(Duration::from_millis(500)).await;

    // Verify WS received events for all 3 conversations
    for conv_id in &conv_ids {
        let conv_events = ws.events_for_conversation(conv_id);
        assert!(
            !conv_events.is_empty(),
            "WS should have events for conversation {}",
            conv_id
        );

        // Each conversation should have at least conversation_created
        let types: Vec<String> = conv_events
            .iter()
            .filter_map(|e| e.event_type().map(String::from))
            .collect();
        assert!(
            types.contains(&"conversation_created".to_string()),
            "conversation {} should have conversation_created; got: {:?}",
            conv_id,
            types
        );
    }
}
