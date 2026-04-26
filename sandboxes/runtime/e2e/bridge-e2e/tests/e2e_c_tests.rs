#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

// ============================================================================
// API key rotation tests
// ============================================================================

#[tokio::test]
async fn test_unknown_conversation_returns_error() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let fake_conv_id = uuid::Uuid::new_v4().to_string();

    // Try sending a message to a nonexistent conversation
    let msg_resp = harness
        .send_message(&fake_conv_id, "hello")
        .await
        .expect("send_message request failed");

    let status = msg_resp.status().as_u16();
    assert_eq!(
        status, 404,
        "sending message to unknown conversation should return 404, got {}",
        status
    );

    // Try ending a nonexistent conversation
    let end_resp = harness
        .end_conversation(&fake_conv_id)
        .await
        .expect("end_conversation request failed");

    let status = end_resp.status().as_u16();
    assert_eq!(
        status, 404,
        "ending unknown conversation should return 404, got {}",
        status
    );
}

#[tokio::test]
async fn test_agent_with_subagents_loads() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let agents = harness.get_agents().await.expect("get_agents failed");

    let agent_ids: Vec<&str> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()))
        .collect();

    assert!(
        agent_ids.contains(&"agent_delegator"),
        "should contain agent_delegator; got {:?}",
        agent_ids
    );
}

#[tokio::test]
async fn test_agent_with_subagents_creates_conversation() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_delegator")
        .await
        .expect("create_conversation request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "POST /agents/agent_delegator/conversations should return 201"
    );

    let body: serde_json::Value = resp
        .json()
        .await
        .expect("failed to parse conversation body");

    assert!(
        body.get("conversation_id").is_some(),
        "response should contain conversation_id"
    );
}

#[tokio::test]
async fn test_webhooks_dispatched_for_conversation() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Clear any prior webhooks
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    // Create a conversation
    let create_resp = harness
        .create_conversation("agent_simple")
        .await
        .expect("create_conversation failed");

    assert_eq!(create_resp.status().as_u16(), 201);

    let create_body: serde_json::Value = create_resp
        .json()
        .await
        .expect("failed to parse create body");
    let conv_id = create_body["conversation_id"]
        .as_str()
        .expect("missing conversation_id");

    // Send a message
    let msg_resp = harness
        .send_message(conv_id, "Hello, agent!")
        .await
        .expect("send_message failed");
    assert_eq!(msg_resp.status().as_u16(), 202);

    // Stream SSE until done — this ensures the turn completes
    let (_events, _response_text) = harness
        .stream_sse_until_done(conv_id, Duration::from_secs(30))
        .await
        .expect("stream_sse_until_done failed");

    // Wait for turn_completed — it is always the last event in the turn
    // lifecycle, so all prior events (response_chunk, response_completed,
    // agent_error, etc.) should already be in the log by the time it arrives.
    // With streaming, many response_chunk webhooks are sent incrementally
    // before turn_completed, so a fixed count no longer works reliably.
    let log = harness
        .wait_for_webhook_type("turn_completed", Duration::from_secs(10))
        .await
        .expect("wait_for_webhook_type(turn_completed) failed");

    // These lifecycle events are always emitted regardless of LLM success/failure:
    // - conversation_created: from the API handler
    // - message_received: from the API handler
    // - response_started: emitted before the LLM call
    // - turn_completed: always emitted at the end of a turn
    log.assert_has_type("conversation_created");
    log.assert_has_type("message_received");
    log.assert_has_type("response_started");
    log.assert_has_type("turn_completed");

    // The turn either succeeds (response_chunk + response_completed) or
    // errors (agent_error). In the mock environment the LLM call errors,
    // so we expect agent_error. Either path is valid.
    let has_success = log.has_type("response_completed");
    let has_error = log.has_type("agent_error");
    assert!(
        has_success || has_error,
        "turn should produce either response_completed or agent_error; got: {:?}",
        log.unique_event_types()
    );

    // HMAC signature and timestamp headers should be present
    log.assert_has_signature_header();
    log.assert_has_timestamp_header();

    // Every webhook payload must include agent_id and conversation_id
    log.assert_all_have_agent_id();
    log.assert_all_have_conversation_id();

    // Verify the conversation_id in payloads matches the one we created
    let conv_webhooks = log.by_conversation(conv_id);
    assert!(
        conv_webhooks.len() >= 4,
        "at least 4 webhooks should reference our conversation; got {} out of {}",
        conv_webhooks.len(),
        log.len()
    );

    // Verify agent_id is set to the agent we used
    let created = log.by_type("conversation_created");
    assert_eq!(
        created.len(),
        1,
        "should have exactly one conversation_created"
    );
    assert_eq!(created[0].agent_id(), Some("agent_simple"));

    // Verify message_received has content in its data
    let received = log.by_type("message_received");
    assert!(!received.is_empty(), "should have message_received");
    let data = received[0]
        .data()
        .expect("message_received should have data");
    assert_eq!(
        data.get("content").and_then(|v| v.as_str()),
        Some("Hello, agent!"),
        "message_received data should contain the message content"
    );
}

#[tokio::test]
async fn test_webhook_includes_abort_event() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Clear any prior webhooks
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    // Create a conversation
    let create_resp = harness
        .create_conversation("agent_simple")
        .await
        .expect("create_conversation failed");

    let create_body: serde_json::Value = create_resp
        .json()
        .await
        .expect("failed to parse create body");
    let conv_id = create_body["conversation_id"]
        .as_str()
        .expect("missing conversation_id");

    // Send a message
    let _msg_resp = harness
        .send_message(conv_id, "Hello!")
        .await
        .expect("send_message failed");

    // Brief delay to allow the turn to start
    tokio::time::sleep(Duration::from_millis(100)).await;

    // Abort the conversation
    let abort_resp = harness
        .abort_conversation(conv_id)
        .await
        .expect("abort_conversation failed");
    assert_eq!(abort_resp.status().as_u16(), 200);

    // Wait for the turn to finish — either via abort (agent_error) or
    // successful completion (turn_completed). With a fast mock LLM the
    // response often completes before the abort arrives.
    let log = harness
        .wait_for_webhook_type("turn_completed", Duration::from_secs(5))
        .await
        .expect("wait_for_webhook_type failed");

    // The turn either completed successfully or was aborted.
    let has_abort = log.by_type("agent_error").iter().any(|e| {
        e.data()
            .and_then(|d| d.get("code"))
            .and_then(|v| v.as_str())
            == Some("aborted")
    });
    let has_success = log.has_type("response_completed");

    assert!(
        has_abort || has_success,
        "turn should produce either an abort agent_error or a response_completed; got types: {:?}",
        log.unique_event_types()
    );

    // Verify the webhook has proper HMAC headers
    log.assert_has_signature_header();
}
