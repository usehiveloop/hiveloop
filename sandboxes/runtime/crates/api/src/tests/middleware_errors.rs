use axum::body::Body;
use http::Request;
use tower::ServiceExt;

use crate::router::build_router;
use crate::tests::{app_with_request_id, body_json, test_state};

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
