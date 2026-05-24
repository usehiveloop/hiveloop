use axum::body::Body;
use http::Request;
use tower::ServiceExt;

use crate::router::build_router;
use crate::tests::{body_json, test_state};

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
