#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

// ============================================================================
// API key rotation tests
// ============================================================================

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

/// Test that the Anthropic native provider can create a conversation and get a response.
/// Requires ANTHROPIC_API_KEY environment variable.
#[tokio::test]
#[ignore]
async fn test_anthropic_native_provider() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_anthropic")
        .await
        .expect("create_conversation failed");
    assert_eq!(resp.status().as_u16(), 201);

    let body: serde_json::Value = resp.json().await.expect("failed to parse body");
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness
        .send_message(conv_id, "Reply with exactly: hello from claude")
        .await
        .expect("send_message failed");

    let (_events, text) = harness
        .stream_sse_until_done(conv_id, Duration::from_secs(30))
        .await
        .expect("stream_sse_until_done failed");

    assert!(
        !text.is_empty(),
        "Anthropic agent should return a non-empty response"
    );
}

/// Test that the Gemini native provider can create a conversation and get a response.
/// Requires GEMINI_API_KEY environment variable.
#[tokio::test]
#[ignore]
async fn test_gemini_native_provider() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_gemini")
        .await
        .expect("create_conversation failed");
    assert_eq!(resp.status().as_u16(), 201);

    let body: serde_json::Value = resp.json().await.expect("failed to parse body");
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness
        .send_message(conv_id, "Reply with exactly: hello from gemini")
        .await
        .expect("send_message failed");

    let (_events, text) = harness
        .stream_sse_until_done(conv_id, Duration::from_secs(30))
        .await
        .expect("stream_sse_until_done failed");

    assert!(
        !text.is_empty(),
        "Gemini agent should return a non-empty response"
    );
}

/// Test that the Cohere native provider can create a conversation and get a response.
/// Requires COHERE_API_KEY environment variable.
#[tokio::test]
#[ignore]
async fn test_cohere_native_provider() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .create_conversation("agent_cohere")
        .await
        .expect("create_conversation failed");
    assert_eq!(resp.status().as_u16(), 201);

    let body: serde_json::Value = resp.json().await.expect("failed to parse body");
    let conv_id = body["conversation_id"].as_str().unwrap();

    harness
        .send_message(conv_id, "Reply with exactly: hello from cohere")
        .await
        .expect("send_message failed");

    let (_events, text) = harness
        .stream_sse_until_done(conv_id, Duration::from_secs(30))
        .await
        .expect("stream_sse_until_done failed");

    assert!(
        !text.is_empty(),
        "Cohere agent should return a non-empty response"
    );
}
