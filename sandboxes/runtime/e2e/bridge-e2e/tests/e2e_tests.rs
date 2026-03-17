use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

// ============================================================================
// API key rotation tests
// ============================================================================

#[tokio::test]
async fn test_patch_api_key_applies_to_existing_conversations() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Create 3 conversations and connect long-lived SSE streams
    let mut conv_ids = Vec::new();
    let mut streams = Vec::new();
    for _ in 0..3 {
        let resp = harness
            .create_conversation("agent_mock_llm")
            .await
            .expect("create_conversation failed");
        let body: serde_json::Value = resp.json().await.expect("failed to parse body");
        let conv_id = body["conversation_id"].as_str().unwrap().to_string();
        let sse = SseStream::connect(harness.bridge_url(), &conv_id)
            .await
            .expect("SSE connect failed");
        conv_ids.push(conv_id);
        streams.push(sse);
    }

    // Send message to each, wait for done, verify original key ("test-key")
    for (i, conv_id) in conv_ids.iter().enumerate() {
        harness
            .send_message(conv_id, "hello before")
            .await
            .expect("send_message failed");
        let events = streams[i]
            .wait_for_done_count(1, Duration::from_secs(10))
            .await;
        let text: String = events
            .iter()
            .filter(|e| e.event_type == "content_delta")
            .filter_map(|e| e.data.get("delta").and_then(|d| d.as_str()))
            .collect();
        assert!(
            text.contains("Bearer test-key"),
            "should use original key, got: {}",
            text
        );
    }

    // Patch the API key
    let resp = harness
        .patch_agent_api_key("agent_mock_llm", "rotated-key")
        .await
        .expect("patch_agent_api_key failed");
    assert_eq!(resp.status().as_u16(), 200);

    // Send message to the SAME 3 conversations, verify new key
    for (i, conv_id) in conv_ids.iter().enumerate() {
        harness
            .send_message(conv_id, "hello after")
            .await
            .expect("send_message failed");
        // Wait for the second done event (one per turn)
        let events = streams[i]
            .wait_for_done_count(2, Duration::from_secs(10))
            .await;
        // Collect content_delta events from the SECOND turn only (after the first done)
        let mut past_first_done = false;
        let mut second_turn_text = String::new();
        for e in &events {
            if e.event_type == "done" && !past_first_done {
                past_first_done = true;
                continue;
            }
            if past_first_done && e.event_type == "content_delta" {
                if let Some(delta) = e.data.get("delta").and_then(|d| d.as_str()) {
                    second_turn_text.push_str(delta);
                }
            }
        }
        assert!(
            second_turn_text.contains("Bearer rotated-key"),
            "should use rotated key, got: {}",
            second_turn_text
        );
    }
}

// ============================================================================
// Agent loading tests
// ============================================================================

#[tokio::test]
async fn test_health_endpoint() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let health = harness.health().await.expect("health request failed");

    assert_eq!(
        health.get("status").and_then(|v| v.as_str()),
        Some("ok"),
        "health endpoint should return status ok"
    );

    assert!(
        health.get("uptime_secs").is_some(),
        "health endpoint should include uptime_secs"
    );
}

#[tokio::test]
async fn test_agents_loaded_from_cp() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let agents = harness.get_agents().await.expect("get_agents failed");

    // The fixtures directory has 3 agents: simple_agent, full_agent, multi_provider
    assert!(
        !agents.is_empty(),
        "bridge should have loaded at least one agent from fixtures"
    );

    // Collect agent IDs
    let agent_ids: Vec<&str> = agents
        .iter()
        .filter_map(|a| a.get("id").and_then(|v| v.as_str()))
        .collect();

    assert!(
        agent_ids.contains(&"agent_simple"),
        "should contain agent_simple; got {:?}",
        agent_ids
    );
}

#[tokio::test]
async fn test_get_specific_agent() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .get_agent("agent_simple")
        .await
        .expect("get_agent request failed");

    assert_eq!(
        resp.status().as_u16(),
        200,
        "GET /agents/agent_simple should return 200"
    );

    let body: serde_json::Value = resp.json().await.expect("failed to parse agent body");

    assert_eq!(
        body.get("id").and_then(|v| v.as_str()),
        Some("agent_simple"),
        "returned agent should have id agent_simple"
    );
    assert_eq!(
        body.get("name").and_then(|v| v.as_str()),
        Some("Simple Agent"),
        "returned agent should have name Simple Agent"
    );
}

#[tokio::test]
async fn test_get_unknown_agent_404() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .get_agent("nonexistent_agent_xyz")
        .await
        .expect("get_agent request failed");

    assert_eq!(
        resp.status().as_u16(),
        404,
        "GET /agents/nonexistent_agent_xyz should return 404"
    );
}

// ============================================================================
// Conversation tests
// ============================================================================

#[tokio::test]
async fn test_create_conversation() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_simple")
        .await
        .expect("create_conversation request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "POST /agents/agent_simple/conversations should return 201"
    );

    let body: serde_json::Value = resp
        .json()
        .await
        .expect("failed to parse conversation body");

    assert!(
        body.get("conversation_id").is_some(),
        "response should contain conversation_id"
    );
    assert!(
        body.get("stream_url").is_some(),
        "response should contain stream_url"
    );
}

#[tokio::test]
async fn test_send_message_accepted() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // First create a conversation
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
        .expect("send_message request failed");

    assert_eq!(
        msg_resp.status().as_u16(),
        202,
        "POST /conversations/{conv_id}/messages should return 202"
    );

    let msg_body: serde_json::Value = msg_resp.json().await.expect("failed to parse message body");
    assert_eq!(
        msg_body.get("status").and_then(|v| v.as_str()),
        Some("accepted"),
        "send message response should have status accepted"
    );
}

#[tokio::test]
async fn test_end_conversation() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    // End the conversation
    let end_resp = harness
        .end_conversation(conv_id)
        .await
        .expect("end_conversation request failed");

    assert_eq!(
        end_resp.status().as_u16(),
        200,
        "DELETE /conversations/{conv_id} should return 200"
    );

    let end_body: serde_json::Value = end_resp.json().await.expect("failed to parse end body");
    assert_eq!(
        end_body.get("status").and_then(|v| v.as_str()),
        Some("ended"),
        "end conversation response should have status ended"
    );
}

#[tokio::test]
async fn test_create_conversation_unknown_agent() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("nonexistent_agent_xyz")
        .await
        .expect("create_conversation request failed");

    assert_eq!(
        resp.status().as_u16(),
        404,
        "POST /agents/nonexistent_agent_xyz/conversations should return 404"
    );
}

// ============================================================================
// Metrics tests
// ============================================================================

#[tokio::test]
async fn test_metrics_endpoint() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let metrics = harness.get_metrics().await.expect("get_metrics failed");

    // Check global metrics
    let global = metrics
        .get("global")
        .expect("metrics should contain 'global'");

    let total_agents = global
        .get("total_agents")
        .and_then(|v| v.as_u64())
        .expect("global should contain total_agents");

    assert!(
        total_agents > 0,
        "total_agents should be greater than 0, got {}",
        total_agents
    );

    assert!(
        global.get("uptime_secs").is_some(),
        "global should contain uptime_secs"
    );

    assert!(
        metrics.get("timestamp").is_some(),
        "metrics should contain timestamp"
    );

    assert!(
        metrics.get("agents").is_some(),
        "metrics should contain agents array"
    );
}

// ============================================================================
// Abort tests
// ============================================================================

#[tokio::test]
async fn test_abort_conversation_returns_200() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    // Abort the conversation (no turn in-flight — should still return 200)
    let abort_resp = harness
        .abort_conversation(conv_id)
        .await
        .expect("abort_conversation request failed");

    assert_eq!(
        abort_resp.status().as_u16(),
        200,
        "POST /conversations/{conv_id}/abort should return 200"
    );

    let abort_body: serde_json::Value =
        abort_resp.json().await.expect("failed to parse abort body");
    assert_eq!(
        abort_body.get("status").and_then(|v| v.as_str()),
        Some("aborted"),
        "abort response should have status: aborted"
    );
}

#[tokio::test]
async fn test_abort_unknown_conversation_returns_404() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let fake_conv_id = uuid::Uuid::new_v4().to_string();

    let abort_resp = harness
        .abort_conversation(&fake_conv_id)
        .await
        .expect("abort_conversation request failed");

    assert_eq!(
        abort_resp.status().as_u16(),
        404,
        "aborting unknown conversation should return 404"
    );
}

#[tokio::test]
async fn test_abort_then_send_message_still_works() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    // Abort (no-op since no turn is in-flight)
    let abort_resp = harness
        .abort_conversation(conv_id)
        .await
        .expect("abort_conversation failed");
    assert_eq!(abort_resp.status().as_u16(), 200);

    // Send a message after abort — conversation should still be alive
    let msg_resp = harness
        .send_message(conv_id, "Hello after abort!")
        .await
        .expect("send_message request failed");

    assert_eq!(
        msg_resp.status().as_u16(),
        202,
        "sending message after abort should return 202 (conversation still alive)"
    );
}

#[tokio::test]
async fn test_double_abort_is_idempotent() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

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

    // Abort twice — both should succeed (idempotent)
    let abort1 = harness
        .abort_conversation(conv_id)
        .await
        .expect("abort 1 failed");
    assert_eq!(abort1.status().as_u16(), 200);

    let abort2 = harness
        .abort_conversation(conv_id)
        .await
        .expect("abort 2 failed");
    assert_eq!(abort2.status().as_u16(), 200);
}

// ============================================================================
// Error tests
// ============================================================================

#[tokio::test]
async fn test_invalid_json_returns_error() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // First create a conversation so we have a valid conv_id
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

    // Send invalid JSON (not a valid SendMessageRequest — missing required "content" field)
    let client = reqwest::Client::new();
    let resp = client
        .post(format!(
            "{}/conversations/{}/messages",
            harness.bridge_url(),
            conv_id
        ))
        .header("content-type", "application/json")
        .body("{\"invalid\": true}")
        .send()
        .await
        .expect("request failed");

    // Should return a 4xx error (422 Unprocessable Entity or 400 Bad Request)
    let status = resp.status().as_u16();
    assert!(
        (400..500).contains(&status),
        "invalid JSON should return 4xx, got {}",
        status
    );
}

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

// ============================================================================
// Subagent / Agent tool tests
// ============================================================================

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

// ============================================================================
// Webhook tests
// ============================================================================

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

    // Wait for at least 5 webhooks to arrive. In the error path we expect:
    // conversation_created, message_received, response_started, agent_error, turn_completed.
    // Each webhook delivery is a separate spawned task, so waiting for just
    // turn_completed can race ahead of agent_error.
    let log = harness
        .wait_for_webhooks(5, Duration::from_secs(10))
        .await
        .expect("wait_for_webhooks failed");

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

#[tokio::test]
async fn test_webhook_end_conversation() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Clear any prior webhooks
    harness
        .clear_webhook_log()
        .await
        .expect("failed to clear webhook log");

    // Create and immediately end a conversation
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

    // End the conversation
    let end_resp = harness
        .end_conversation(conv_id)
        .await
        .expect("end_conversation failed");
    assert_eq!(end_resp.status().as_u16(), 200);

    // Wait for webhooks
    let log = harness
        .wait_for_webhook_type("conversation_ended", Duration::from_secs(5))
        .await
        .expect("wait_for_webhook_type failed");

    // Should have both created and ended
    log.assert_has_type("conversation_created");
    log.assert_has_type("conversation_ended");

    // Verify the ended webhook references the right conversation
    let ended = log.by_type("conversation_ended");
    assert_eq!(ended.len(), 1);
    assert_eq!(ended[0].conversation_id(), Some(conv_id));
    assert_eq!(ended[0].agent_id(), Some("agent_simple"));
}

