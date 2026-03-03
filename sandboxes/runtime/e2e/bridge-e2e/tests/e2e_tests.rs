use bridge_e2e::TestHarness;

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

