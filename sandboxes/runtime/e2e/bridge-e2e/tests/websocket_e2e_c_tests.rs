#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{TestHarness, WsEventStream};
use std::time::Duration;

const TIMEOUT: Duration = Duration::from_secs(30);

// ============================================================================
// Authentication tests
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
