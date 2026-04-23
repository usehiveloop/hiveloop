use axum::body::Body;
use axum::Router;
use http::Request;
use tower::ServiceExt;

use crate::router::build_router;
use crate::tests::{body_json, test_state};

// ── Conversation tool/MCP scoping tests ───────────────────────────────────

/// Helper: push a minimal test agent and return its ID.
pub(crate) async fn push_test_agent(app: &Router, agent_id: &str) {
    let agent_def = serde_json::json!({
        "agents": [{
            "id": agent_id,
            "name": "Test Agent",
            "system_prompt": "You are a test agent.",
            "provider": {
                "provider_type": "open_ai",
                "model": "gpt-4o",
                "api_key": "test-key",
                "base_url": "https://api.openai.com/v1"
            }
        }]
    });

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(serde_json::to_string(&agent_def).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200, "push agent should succeed");
}

#[tokio::test]
async fn create_conversation_no_body_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent").await;

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent/conversations")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
    let json = body_json(response).await;
    assert!(
        json["conversation_id"].is_string(),
        "should return conversation_id"
    );
    assert!(json["stream_url"].is_string(), "should return stream_url");
}

#[tokio::test]
async fn create_conversation_with_valid_tool_names_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-2").await;

    // Agent has builtin tools because tools: [] in definition means all builtins.
    // "bash" and "Read" are always registered as builtins.
    let body = serde_json::json!({
        "tool_names": ["bash", "Read"]
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-2/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
    let json = body_json(response).await;
    assert!(json["conversation_id"].is_string());
}

#[tokio::test]
async fn create_conversation_with_invalid_tool_name_returns_400() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-3").await;

    let body = serde_json::json!({
        "tool_names": ["bash", "totally_fake_tool"]
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-3/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 400);
    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "invalid_request");
    assert!(
        json["error"]["message"]
            .as_str()
            .unwrap()
            .contains("totally_fake_tool"),
        "error should name the invalid tool"
    );
}

#[tokio::test]
async fn create_conversation_with_invalid_mcp_server_returns_400() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-4").await;

    let body = serde_json::json!({
        "mcp_server_names": ["nonexistent-server"]
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-4/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 400);
    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "invalid_request");
    assert!(
        json["error"]["message"]
            .as_str()
            .unwrap()
            .contains("nonexistent-server"),
        "error should name the invalid MCP server"
    );
}

#[tokio::test]
async fn create_conversation_with_empty_json_body_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-5").await;

    // Empty JSON object = both filters are None = all tools
    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-5/conversations")
                .header("content-type", "application/json")
                .body(Body::from("{}"))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
}

#[tokio::test]
async fn create_conversation_with_empty_tool_names_array_succeeds() {
    let app = build_router(test_state());
    push_test_agent(&app, "scoping-agent-6").await;

    // Empty array means zero tools — should succeed (agent has no tools)
    let body = serde_json::json!({
        "tool_names": []
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/scoping-agent-6/conversations")
                .header("content-type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 201);
}
