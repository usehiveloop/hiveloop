#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

// ============================================================================
// API key rotation tests
// ============================================================================

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
