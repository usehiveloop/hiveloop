use pretty_assertions::assert_eq;

use crate::error::BridgeError;

#[test]
fn bridge_error_display_for_each_variant() {
    assert_eq!(
        BridgeError::AgentNotFound("a1".into()).to_string(),
        "agent not found: a1"
    );
    assert_eq!(
        BridgeError::ConversationNotFound("c1".into()).to_string(),
        "conversation not found: c1"
    );
    assert_eq!(
        BridgeError::ConversationEnded("c1".into()).to_string(),
        "conversation ended: c1"
    );
    assert_eq!(
        BridgeError::InvalidRequest("bad".into()).to_string(),
        "invalid request: bad"
    );
    assert_eq!(
        BridgeError::ProviderError("fail".into()).to_string(),
        "provider error: fail"
    );
    assert_eq!(
        BridgeError::McpError("fail".into()).to_string(),
        "mcp error: fail"
    );
    assert_eq!(
        BridgeError::ToolError("fail".into()).to_string(),
        "tool error: fail"
    );
    assert_eq!(
        BridgeError::ConfigError("fail".into()).to_string(),
        "config error: fail"
    );
    assert_eq!(
        BridgeError::WebhookError("fail".into()).to_string(),
        "webhook error: fail"
    );
    assert_eq!(
        BridgeError::Internal("fail".into()).to_string(),
        "internal error: fail"
    );
    assert_eq!(BridgeError::RateLimited.to_string(), "rate limited");
    assert_eq!(
        BridgeError::Unauthorized("bad token".into()).to_string(),
        "unauthorized: bad token"
    );
    assert_eq!(
        BridgeError::Conflict("active conversations".into()).to_string(),
        "conflict: active conversations"
    );
}

// ──────────────────────────────────────────────
// BridgeError IntoResponse status codes
// ──────────────────────────────────────────────

#[test]
fn bridge_error_into_response_agent_not_found_is_404() {
    use axum::response::IntoResponse;

    let err = BridgeError::AgentNotFound("x".into());
    let response = err.into_response();
    assert_eq!(response.status(), axum::http::StatusCode::NOT_FOUND);
}

#[test]
fn bridge_error_into_response_conversation_not_found_is_404() {
    use axum::response::IntoResponse;

    let err = BridgeError::ConversationNotFound("x".into());
    let response = err.into_response();
    assert_eq!(response.status(), axum::http::StatusCode::NOT_FOUND);
}

#[test]
fn bridge_error_into_response_conversation_ended_is_400() {
    use axum::response::IntoResponse;

    let err = BridgeError::ConversationEnded("x".into());
    let response = err.into_response();
    assert_eq!(response.status(), axum::http::StatusCode::BAD_REQUEST);
}

#[test]
fn bridge_error_into_response_invalid_request_is_400() {
    use axum::response::IntoResponse;

    let err = BridgeError::InvalidRequest("bad".into());
    let response = err.into_response();
    assert_eq!(response.status(), axum::http::StatusCode::BAD_REQUEST);
}

#[test]
fn bridge_error_into_response_provider_error_is_500() {
    use axum::response::IntoResponse;

    let err = BridgeError::ProviderError("fail".into());
    let response = err.into_response();
    assert_eq!(
        response.status(),
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    );
}

#[test]
fn bridge_error_into_response_mcp_error_is_500() {
    use axum::response::IntoResponse;

    let err = BridgeError::McpError("fail".into());
    let response = err.into_response();
    assert_eq!(
        response.status(),
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    );
}

#[test]
fn bridge_error_into_response_tool_error_is_500() {
    use axum::response::IntoResponse;

    let err = BridgeError::ToolError("fail".into());
    let response = err.into_response();
    assert_eq!(
        response.status(),
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    );
}

#[test]
fn bridge_error_into_response_config_error_is_500() {
    use axum::response::IntoResponse;

    let err = BridgeError::ConfigError("fail".into());
    let response = err.into_response();
    assert_eq!(
        response.status(),
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    );
}

#[test]
fn bridge_error_into_response_webhook_error_is_500() {
    use axum::response::IntoResponse;

    let err = BridgeError::WebhookError("fail".into());
    let response = err.into_response();
    assert_eq!(
        response.status(),
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    );
}

#[test]
fn bridge_error_into_response_internal_is_500() {
    use axum::response::IntoResponse;

    let err = BridgeError::Internal("fail".into());
    let response = err.into_response();
    assert_eq!(
        response.status(),
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    );
}

#[test]
fn bridge_error_into_response_rate_limited_is_429() {
    use axum::response::IntoResponse;

    let err = BridgeError::RateLimited;
    let response = err.into_response();
    assert_eq!(response.status(), axum::http::StatusCode::TOO_MANY_REQUESTS);
}

#[test]
fn bridge_error_into_response_body_contains_error_code() {
    use axum::body::to_bytes;
    use axum::response::IntoResponse;

    let err = BridgeError::AgentNotFound("agent-99".into());
    let response = err.into_response();

    let rt = tokio::runtime::Runtime::new().unwrap();
    let body_bytes = rt
        .block_on(to_bytes(response.into_body(), usize::MAX))
        .unwrap();
    let body: serde_json::Value = serde_json::from_slice(&body_bytes).unwrap();

    assert_eq!(body["error"]["code"], "agent_not_found");
    assert_eq!(body["error"]["message"], "agent not found: agent-99");
}

// ──────────────────────────────────────────────
