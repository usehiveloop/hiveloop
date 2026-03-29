use axum::body::Body;
use axum::middleware as axum_mw;
use axum::Router;
use http::Request;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio_util::sync::CancellationToken;
use tower::ServiceExt;

use crate::middleware::request_id;
use crate::router::build_router;
use crate::state::AppState;

/// Build a test `AppState` backed by a real (but empty) `AgentSupervisor`.
fn test_state() -> AppState {
    let mcp_manager = Arc::new(McpManager::new());
    let cancel = CancellationToken::new();
    let supervisor = Arc::new(AgentSupervisor::new(mcp_manager, cancel));
    AppState::new(supervisor, "valid-control-plane-token".to_string(), None, None)
}

/// Build the application router with the request-id middleware applied,
/// using the given `AppState`.
fn app_with_request_id(state: AppState) -> Router {
    build_router(state).layer(axum_mw::from_fn(request_id))
}

/// Helper: read the full response body as bytes.
async fn body_bytes(response: axum::response::Response) -> Vec<u8> {
    axum::body::to_bytes(response.into_body(), usize::MAX)
        .await
        .expect("failed to read body")
        .to_vec()
}

/// Helper: read the full response body as a `serde_json::Value`.
async fn body_json(response: axum::response::Response) -> serde_json::Value {
    let bytes = body_bytes(response).await;
    serde_json::from_slice(&bytes).expect("body is not valid JSON")
}

// ── 1. GET /health → 200, body has "status": "ok" and "uptime_secs" ─────────

#[tokio::test]
async fn health_returns_200_with_status_ok_and_uptime() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert_eq!(json["status"], "ok");
    assert!(
        json["uptime_secs"].is_number(),
        "uptime_secs should be a number"
    );
}

// ── 2. GET /agents → 200, returns JSON array (empty) ────────────────────────

#[tokio::test]
async fn list_agents_returns_empty_array() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/agents")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert!(json.is_array(), "response should be an array");
    assert_eq!(json.as_array().unwrap().len(), 0, "array should be empty");
}

// ── 3. GET /agents/unknown → 404 ────────────────────────────────────────────

#[tokio::test]
async fn get_unknown_agent_returns_404() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/agents/unknown")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
}

// ── 4. POST /agents/unknown/conversations → error (agent not found) ─────────

#[tokio::test]
async fn create_conversation_for_unknown_agent_returns_error() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/agents/unknown/conversations")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
    assert!(
        json["error"]["message"]
            .as_str()
            .unwrap()
            .contains("unknown"),
        "error message should contain the agent id"
    );
}

// ── 5. POST /conversations/unknown/messages → error ─────────────────────────

#[tokio::test]
async fn send_message_to_unknown_conversation_returns_error() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/conversations/unknown/messages")
                .header("content-type", "application/json")
                .body(Body::from(r#"{"content":"hello"}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "conversation_not_found");
}

// ── 6. DELETE /conversations/unknown → error ────────────────────────────────

#[tokio::test]
async fn end_unknown_conversation_returns_error() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri("/conversations/unknown")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "conversation_not_found");
}

// ── 7. GET /metrics → 200, returns valid MetricsResponse JSON ───────────────

#[tokio::test]
async fn metrics_returns_valid_json() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/metrics")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;

    // Verify top-level structure
    assert!(
        json["timestamp"].is_string(),
        "timestamp should be a string"
    );
    assert!(json["agents"].is_array(), "agents should be an array");
    assert!(json["global"].is_object(), "global should be an object");

    // Verify global metrics
    let global = &json["global"];
    assert_eq!(global["total_agents"], 0);
    assert_eq!(global["total_active_conversations"], 0);
    assert!(
        global["uptime_secs"].is_number(),
        "uptime_secs should be a number"
    );

    // With no agents loaded, agents array should be empty
    assert_eq!(json["agents"].as_array().unwrap().len(), 0);
}

// ── 8. Request ID middleware adds X-Request-ID header ────────────────────────

#[tokio::test]
async fn request_id_middleware_generates_id_when_absent() {
    let app = app_with_request_id(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let header = response
        .headers()
        .get("x-request-id")
        .expect("x-request-id header should be present");

    let value = header.to_str().unwrap();
    assert!(!value.is_empty(), "x-request-id should not be empty");

    // Verify the generated value looks like a UUID (36 chars with hyphens)
    assert_eq!(value.len(), 36, "x-request-id should be a UUID");
}

#[tokio::test]
async fn request_id_middleware_preserves_existing_id() {
    let app = app_with_request_id(test_state());

    let custom_id = "my-custom-request-id-12345";

    let response = app
        .oneshot(
            Request::builder()
                .uri("/health")
                .header("x-request-id", custom_id)
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let header = response
        .headers()
        .get("x-request-id")
        .expect("x-request-id header should be present");

    assert_eq!(header.to_str().unwrap(), custom_id);
}

// ── 9. Error responses have correct JSON structure ──────────────────────────

#[tokio::test]
async fn error_response_has_correct_json_structure() {
    let app = build_router(test_state());

    // Use GET /agents/nonexistent to trigger a 404 error response
    let response = app
        .oneshot(
            Request::builder()
                .uri("/agents/nonexistent")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;

    // Top-level must have "error" object
    assert!(
        json["error"].is_object(),
        "response must have an 'error' object"
    );

    // "error" object must have "code" and "message" fields
    let error = &json["error"];
    assert!(error["code"].is_string(), "error.code should be a string");
    assert!(
        error["message"].is_string(),
        "error.message should be a string"
    );

    // Verify specific values for this case
    assert_eq!(error["code"], "agent_not_found");
    assert!(
        error["message"].as_str().unwrap().contains("nonexistent"),
        "error message should reference the missing agent ID"
    );
}

#[tokio::test]
async fn conversation_not_found_error_has_correct_structure() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri("/conversations/does-not-exist")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;

    let error = &json["error"];
    assert_eq!(error["code"], "conversation_not_found");
    assert!(
        error["message"]
            .as_str()
            .unwrap()
            .contains("does-not-exist"),
        "error message should reference the missing conversation ID"
    );
}

// ── 10. Push endpoint auth tests ─────────────────────────────────────────────

#[tokio::test]
async fn push_without_auth_returns_401() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .body(Body::from(r#"{"agents":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 401);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "unauthorized");
}

#[tokio::test]
async fn push_with_wrong_token_returns_401() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer invalid-control-plane-token")
                .body(Body::from(r#"{"agents":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 401);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "unauthorized");
}

#[tokio::test]
async fn push_with_correct_token_succeeds() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(r#"{"agents":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert_eq!(json["loaded"], 0);
}

// ── 11. Push endpoint validation tests ───────────────────────────────────────

#[tokio::test]
async fn upsert_agent_path_body_mismatch_returns_400() {
    let app = build_router(test_state());

    let body = serde_json::json!({
        "id": "bar",
        "name": "Test",
        "system_prompt": "test",
        "provider": {
            "provider_type": "anthropic",
            "model": "claude-sonnet-4-20250514",
            "api_key": "<provider-api-key>"
        }
    });

    let response = app
        .oneshot(
            Request::builder()
                .method("PUT")
                .uri("/push/agents/foo")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 400);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "invalid_request");
}

#[tokio::test]
async fn remove_nonexistent_agent_returns_404() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("DELETE")
                .uri("/push/agents/unknown")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
}

#[tokio::test]
async fn hydrate_unknown_agent_returns_404() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/agents/unknown/conversations")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(r#"{"conversations":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 404);

    let json = body_json(response).await;
    assert_eq!(json["error"]["code"], "agent_not_found");
}

#[tokio::test]
async fn push_diff_empty_succeeds() {
    let app = build_router(test_state());

    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/push/diff")
                .header("content-type", "application/json")
                .header("authorization", "Bearer valid-control-plane-token")
                .body(Body::from(r#"{"added":[],"updated":[],"removed":[]}"#))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), 200);

    let json = body_json(response).await;
    assert_eq!(json["added"], 0);
    assert_eq!(json["updated"], 0);
    assert_eq!(json["removed"], 0);
}

// ── Conversation tool/MCP scoping tests ───────────────────────────────────

/// Helper: push a minimal test agent and return its ID.
async fn push_test_agent(app: &Router, agent_id: &str) {
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
