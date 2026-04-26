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
async fn test_create_conversation_both_filters_valid() {
    let harness = TestHarness::start()
        .await
        .expect("failed to start test harness");

    // agent_mock_llm has no MCP servers, so mcp_server_names: [] is valid.
    // tool_names: ["bash"] restricts to only bash.
    let resp = harness
        .client()
        .post(format!(
            "{}/agents/agent_mock_llm/conversations",
            harness.bridge_url()
        ))
        .header("content-type", "application/json")
        .json(&serde_json::json!({
            "tool_names": ["bash"],
            "mcp_server_names": []
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "both filters together should succeed"
    );
}

#[tokio::test]
async fn test_create_conversation_with_api_key_override() {
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
            "api_key": "sk-custom-override-key"
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "should create conversation with API key override"
    );

    let body: serde_json::Value = resp.json().await.unwrap();
    assert!(body["conversation_id"].is_string());
}

#[tokio::test]
async fn test_api_key_override_conversation_responds_to_messages() {
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
            "api_key": "sk-custom-key-for-test"
        }))
        .send()
        .await
        .expect("create conversation failed");

    assert_eq!(resp.status().as_u16(), 201);
    let body: serde_json::Value = resp.json().await.unwrap();
    let conv_id = body["conversation_id"].as_str().unwrap();

    let sse = SseStream::connect(harness.bridge_url(), conv_id)
        .await
        .expect("SSE connect failed");

    harness
        .send_message(conv_id, "Hello with custom API key")
        .await
        .expect("send_message failed");

    let events = sse.wait_for_done_count(1, Duration::from_secs(15)).await;
    let event_types: Vec<&str> = events.iter().map(|e| e.event_type.as_str()).collect();
    assert!(
        event_types.contains(&"done"),
        "should complete with done event; got: {:?}",
        event_types
    );
}

#[tokio::test]
async fn test_create_conversation_with_api_key_and_tool_filter() {
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
            "api_key": "sk-custom-key",
            "tool_names": ["bash"]
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        201,
        "should create conversation with both API key override and tool filter"
    );
}

#[tokio::test]
async fn test_create_conversation_with_empty_api_key_returns_400() {
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
            "api_key": ""
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        400,
        "should return 400 for empty API key"
    );

    let body: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(body["error"]["code"], "invalid_request");
    assert!(
        body["error"]["message"]
            .as_str()
            .unwrap()
            .contains("api_key cannot be empty"),
        "error should mention empty api_key"
    );
}

#[tokio::test]
async fn test_create_conversation_with_invalid_subagent_name_returns_400() {
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
            "subagent_api_keys": {
                "nonexistent_subagent": "<provider-api-key>"
            }
        }))
        .send()
        .await
        .expect("request failed");

    assert_eq!(
        resp.status().as_u16(),
        400,
        "should return 400 for unknown subagent name"
    );

    let body: serde_json::Value = resp.json().await.unwrap();
    assert_eq!(body["error"]["code"], "invalid_request");
    assert!(
        body["error"]["message"]
            .as_str()
            .unwrap()
            .contains("nonexistent_subagent"),
        "error should name the invalid subagent"
    );
}
