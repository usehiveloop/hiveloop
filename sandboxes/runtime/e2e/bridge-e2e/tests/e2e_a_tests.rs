#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
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
