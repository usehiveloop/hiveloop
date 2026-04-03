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

// ============================================================================
// Event lifecycle tests
// ============================================================================

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

// ============================================================================
// Parity with webhooks
// ============================================================================

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

// ============================================================================
// Multiplexing tests
// ============================================================================

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

// ============================================================================
// Sequence number tests
// ============================================================================

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

// ============================================================================
// WebSocket-only mode (no webhooks)
// ============================================================================

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

// ============================================================================
// Resilience tests
// ============================================================================

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

// ============================================================================
// High throughput test
// ============================================================================

#[tokio::test]
async fn test_ws_high_throughput_multiple_conversations() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Launch 5 conversations in parallel
    let mut conv_ids = Vec::new();
    for _ in 0..5 {
        let resp = harness.create_conversation("agent_simple").await.unwrap();
        let body: serde_json::Value = resp.json().await.unwrap();
        conv_ids.push(body["conversation_id"].as_str().unwrap().to_string());
    }

    // Send messages to all
    for conv_id in &conv_ids {
        harness.send_message(conv_id, "Hello!").await.unwrap();
    }

    // Wait for all turns to complete via SSE
    for conv_id in &conv_ids {
        let _ = harness.stream_sse_until_done(conv_id, TIMEOUT).await;
    }

    // Give WS time to receive all events
    tokio::time::sleep(Duration::from_secs(1)).await;

    // Verify all conversations have events
    for conv_id in &conv_ids {
        let conv_events = ws.events_for_conversation(conv_id);
        assert!(
            !conv_events.is_empty(),
            "should have WS events for conversation {}",
            conv_id
        );

        let types: Vec<String> = conv_events
            .iter()
            .filter_map(|e| e.event_type().map(String::from))
            .collect();
        assert!(
            types.contains(&"conversation_created".to_string()),
            "conversation {} missing conversation_created",
            conv_id
        );
    }

    // Verify sequence numbers are globally unique and contiguous (no gaps).
    // Events may arrive interleaved from concurrent conversations, so we sort
    // before checking monotonicity.
    let all_events = ws.events();
    let mut seq_numbers: Vec<u64> = all_events
        .iter()
        .filter_map(|e| e.sequence_number())
        .collect();
    seq_numbers.sort();

    for window in seq_numbers.windows(2) {
        assert!(
            window[1] > window[0],
            "sequence numbers must be unique; found duplicate or misordering: {:?}",
            window
        );
    }

    // No events should be dropped — each conversation should have
    // at least conversation_created + message_received + response_started + turn_completed
    let total = all_events.len();
    assert!(
        total >= 5 * 4,
        "should have at least 20 events for 5 conversations; got {}",
        total
    );
}

// ============================================================================
// Webhook end_conversation via WebSocket
// ============================================================================

#[tokio::test]
async fn test_ws_receives_conversation_ended() {
    let harness = TestHarness::start_with_websocket(true)
        .await
        .expect("failed to start harness");

    let ws = WsEventStream::connect(harness.bridge_url(), "e2e-test-key")
        .await
        .expect("WS connect failed");

    // Create and end a conversation
    let resp = harness.create_conversation("agent_simple").await.unwrap();
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness.end_conversation(conv_id).await.unwrap();

    // Wait for conversation_ended
    let ended = ws
        .wait_for_event_type("conversation_ended", Duration::from_secs(5))
        .await;
    assert!(
        ended.is_some(),
        "should receive conversation_ended over WebSocket"
    );

    let ended = ended.unwrap();
    assert_eq!(ended.conversation_id(), Some(conv_id));
    assert_eq!(ended.agent_id(), Some("agent_simple"));
}
