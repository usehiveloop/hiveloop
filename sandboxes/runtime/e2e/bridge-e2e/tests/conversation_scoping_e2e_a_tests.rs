#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_e2e::{SseStream, TestHarness};
use std::time::Duration;

// ============================================================================
// Per-conversation tool/MCP scoping e2e tests
//
// These tests verify that the create_conversation endpoint accepts optional
// tool_names and mcp_server_names filters, and that conversations created
// with filters work correctly end-to-end.
// ============================================================================

// ── Backward compatibility: no body ───────────────────────────────────────

#[tokio::test]
async fn test_create_conversation_no_body_backward_compat() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Existing behavior: POST with no body should work
    let resp = harness
        .create_conversation("agent_mock_llm")
        .await
        .expect("create_conversation failed");

    assert_eq!(resp.status().as_u16(), 201);
    let body: serde_json::Value = resp.json().await.unwrap();
    assert!(body["conversation_id"].is_string());
    assert!(body["stream_url"].is_string());
}

#[tokio::test]
async fn test_create_conversation_empty_json_body_backward_compat() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .body("{}")
        .send()
        .await
        .expect("request failed");

    assert_eq!(resp.status().as_u16(), 201);
}

#[tokio::test]
async fn test_create_conversation_with_valid_tool_filter() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // agent_mock_llm has tools: [] which means all builtins are registered.
    // "bash" and "Read" are always-present builtin tools.
    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "tool_names": ["bash", "Read"]
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "should create conversation with filtered tools"
    );

    let body: serde_json::Value = resp.json().await.unwrap();
    assert!(body["conversation_id"].is_string());
}

#[tokio::test]
async fn test_scoped_conversation_responds_to_messages() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Create conversation with only "bash" tool
    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "tool_names": ["bash"]
        }))
        .send()
        .await
        .expect("create conversation failed");

    assert_eq!(resp.status().as_u16(), 201);
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    // Connect SSE stream
    let sse = SseStream::connect(harness.bridge_url(), conv_id)
        .await
        .expect("SSE connect failed");

    // Send a message
    harness
        .send_message(conv_id, "Hello from scoped conversation")
        .await
        .expect("send_message failed");

    // Wait for the response to complete
    let events = sse.wait_for_done_count(1, Duration::from_secs(15)).await;

    // The mock LLM should produce at least a message_start and done event.
    let event_types: Vec<&str> = events.iter().map(|e| e.event_type.as_str()).collect();
    assert!(
        event_types.contains(&"message_start"),
        "should have message_start event; got: {:?}",
        event_types
    );
    assert!(
        event_types.contains(&"done"),
        "should have done event; got: {:?}",
        event_types
    );
}

#[tokio::test]
async fn test_create_conversation_invalid_tool_name_returns_400() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "tool_names": ["bash", "totally_nonexistent_tool"]
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        400,
        "should return 400 for invalid tool name"
    );

    let body: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(body["error"]["code"], "invalid_request");
    assert!(
        body["error"]["message"]
            .as_str()
            .unwrap()
            .contains("totally_nonexistent_tool"),
        "error should name the invalid tool"
    );
}

#[tokio::test]
async fn test_create_conversation_invalid_mcp_server_returns_400() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "mcp_server_names": ["nonexistent_mcp_server"]
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        400,
        "should return 400 for invalid MCP server name"
    );

    let body: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(body["error"]["code"], "invalid_request");
    assert!(
        body["error"]["message"]
            .as_str()
            .unwrap()
            .contains("nonexistent_mcp_server"),
        "error should name the invalid MCP server"
    );
}

#[tokio::test]
async fn test_create_conversation_empty_tool_names_succeeds() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Empty array = zero tools, agent can only respond with text
    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "tool_names": []
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "empty tool_names array should succeed (zero-tool conversation)"
    );
}

#[tokio::test]
async fn test_create_conversation_empty_mcp_server_names_succeeds() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // Empty mcp_server_names = no MCP tools, only builtins remain
    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "mcp_server_names": []
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "empty mcp_server_names should succeed"
    );
}
