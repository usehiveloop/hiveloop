use axum::body::Body;
use http::Request;
use tower::ServiceExt;

use crate::router::build_router;
use crate::tests::{body_json, test_state};

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
        "harness": "open_code",
        "system_prompt": "test",
        "provider": {
            "provider_type": "open_ai",
            "model": "gpt-4o",
            "api_key": "provider-api-key"
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
