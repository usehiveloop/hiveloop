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
    AppState::new(supervisor)
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
