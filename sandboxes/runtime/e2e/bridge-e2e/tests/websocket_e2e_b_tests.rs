#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{TestHarness, WsEventStream};
use std::time::Duration;

const TIMEOUT: Duration = Duration::from_secs(30);

// ============================================================================
// Authentication tests
// ============================================================================

#[tokio::test]
async fn test_ws_multiplexes_multiple_agents() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Create conversations on two different agents
    let resp1 = harness.create_conversation("agent_simple").await.unwrap();
    let body1: serde_json::Value = resp1.json().await.unwrap();
    let conv_id_1 = body1["conversation_id"].as_str().unwrap().to_string();

    let resp2 = harness.create_conversation("agent_mock_llm").await.unwrap();
    let body2: serde_json::Value = resp2.json().await.unwrap();
    let conv_id_2 = body2["conversation_id"].as_str().unwrap().to_string();

    // Send messages to both
    harness.send_message(&conv_id_1, "Hello!").await.unwrap();
    harness.send_message(&conv_id_2, "Hi!").await.unwrap();

    // Wait for turns to complete
    let _ = harness.stream_sse_until_done(&conv_id_1, TIMEOUT).await;
    let _ = harness.stream_sse_until_done(&conv_id_2, TIMEOUT).await;

    tokio::time::sleep(Duration::from_millis(500)).await;

    // Verify both agent IDs are present in WS events
    let agent_simple_events = ws.events_for_agent("agent_simple");
    let agent_mock_events = ws.events_for_agent("agent_mock_llm");

    assert!(
        !agent_simple_events.is_empty(),
        "should have events for agent_simple"
    );
    assert!(
        !agent_mock_events.is_empty(),
        "should have events for agent_mock_llm"
    );
}

#[tokio::test]
async fn test_ws_sequence_numbers_are_monotonic() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Generate events from 2 conversations to get interleaved events
    let resp1 = harness.create_conversation("agent_simple").await.unwrap();
    let body1: serde_json::Value = resp1.json().await.unwrap();
    let conv_id_1 = body1["conversation_id"].as_str().unwrap().to_string();

    let resp2 = harness.create_conversation("agent_simple").await.unwrap();
    let body2: serde_json::Value = resp2.json().await.unwrap();
    let conv_id_2 = body2["conversation_id"].as_str().unwrap().to_string();

    harness.send_message(&conv_id_1, "Hello!").await.unwrap();
    harness.send_message(&conv_id_2, "Hi!").await.unwrap();

    let _ = harness.stream_sse_until_done(&conv_id_1, TIMEOUT).await;
    let _ = harness.stream_sse_until_done(&conv_id_2, TIMEOUT).await;

    tokio::time::sleep(Duration::from_millis(500)).await;

    let events = ws.events();
    let seq_numbers: Vec<u64> = events.iter().filter_map(|e| e.sequence_number()).collect();

    assert!(
        seq_numbers.len() >= 4,
        "should have at least 4 events; got {}",
        seq_numbers.len()
    );

    // Sequence numbers should be strictly increasing
    for window in seq_numbers.windows(2) {
        assert!(
            window[1] > window[0],
            "sequence numbers must be strictly increasing: {} should be > {}",
            window[1],
            window[0]
        );
    }
}

#[tokio::test]
async fn test_ws_only_mode_no_webhooks() {
    // Start with WebSocket enabled but NO webhooks
    let harness = TestHarness::start_with_websocket(false)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Run a conversation
    let resp = harness.create_conversation("agent_simple").await.unwrap();
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness.send_message(conv_id, "Hello!").await.unwrap();
    let _ = harness.stream_sse_until_done(conv_id, TIMEOUT).await;

    // WS events should still arrive
    let turn = ws.wait_for_event_type("turn_completed", TIMEOUT).await;
    assert!(
        turn.is_some(),
        "should receive turn_completed even without webhooks"
    );

    // Webhook log should be empty (no webhook URL configured)
    let webhook_log = harness.get_webhook_log().await.unwrap();
    assert!(
        webhook_log.is_empty(),
        "webhook log should be empty in WS-only mode; got {} entries",
        webhook_log.len()
    );
}

#[tokio::test]
async fn test_ws_client_disconnect_does_not_crash_bridge() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    // Connect and immediately drop the WS client
    {
        let _ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
            .await
            .expect("WS connect failed");
        // _ws is dropped here
    }

    // Give bridge a moment to notice the disconnect
    tokio::time::sleep(Duration::from_millis(200)).await;

    // Bridge should still be healthy
    let health = harness.health().await.expect("health request failed");
    assert_eq!(
        health.get("status").and_then(|v| v.as_str()),
        Some("ok"),
        "bridge should be healthy after WS disconnect"
    );

    // Create a new conversation and verify it works
    let resp = harness.create_conversation("agent_simple").await.unwrap();
    assert_eq!(resp.status().as_u16(), 201);

    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();
    harness.send_message(conv_id, "Hello!").await.unwrap();

    let (events, _) = harness
        .stream_sse_until_done(conv_id, TIMEOUT)
        .await
        .unwrap();
    assert!(
        events.iter().any(|e| e.event_type == "done"),
        "SSE should still work after WS disconnect"
    );

    // New WS connection should also work
    let ws2 = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("should be able to reconnect");

    // Generate another event
    let resp2 = harness.create_conversation("agent_simple").await.unwrap();
    let body2: serde_json::Value = resp2.json().await.unwrap();
    let conv_id2 = body2["conversation_id"].as_str().unwrap();

    let created = ws2
        .wait_for_event_type("conversation_created", Duration::from_secs(5))
        .await;
    assert!(created.is_some(), "new WS connection should receive events");
    assert_eq!(
        created.unwrap().conversation_id(),
        Some(conv_id2),
        "should receive event for the new conversation"
    );
}

#[tokio::test]
async fn test_ws_multiple_clients_receive_same_events() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    // Connect two WS clients
    let ws1 = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS client 1 failed");
    let ws2 = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS client 2 failed");

    // Generate events
    let resp = harness.create_conversation("agent_simple").await.unwrap();
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness.send_message(conv_id, "Hello!").await.unwrap();
    let _ = harness.stream_sse_until_done(conv_id, TIMEOUT).await;

    // Wait for both clients to receive turn_completed
    ws1.wait_for_event_type("turn_completed", TIMEOUT).await;
    ws2.wait_for_event_type("turn_completed", TIMEOUT).await;

    let events1 = ws1.events();
    let events2 = ws2.events();

    // Both should have the same event types
    let mut types1: Vec<String> = events1
        .iter()
        .filter_map(|e| e.event_type().map(String::from))
        .collect();
    let mut types2: Vec<String> = events2
        .iter()
        .filter_map(|e| e.event_type().map(String::from))
        .collect();
    types1.sort();
    types2.sort();

    assert_eq!(
        types1, types2,
        "both WS clients should receive the same event types"
    );

    // Both should have the same sequence numbers
    let seqs1: Vec<u64> = events1.iter().filter_map(|e| e.sequence_number()).collect();
    let seqs2: Vec<u64> = events2.iter().filter_map(|e| e.sequence_number()).collect();

    assert_eq!(
        seqs1, seqs2,
        "both WS clients should receive the same sequence numbers"
    );
}
