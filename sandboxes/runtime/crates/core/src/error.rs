use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};

/// Central error type for the bridge runtime.
#[derive(Debug, thiserror::Error)]
pub enum BridgeError {
    /// The requested agent was not found
    #[error("agent not found: {0}")]
    AgentNotFound(String),

    /// The requested conversation was not found
    #[error("conversation not found: {0}")]
    ConversationNotFound(String),

    /// The conversation has already ended
    #[error("conversation ended: {0}")]
    ConversationEnded(String),

    /// The request was invalid
    #[error("invalid request: {0}")]
    InvalidRequest(String),

    /// An error occurred in the LLM provider
    #[error("provider error: {0}")]
    ProviderError(String),

    /// An error occurred in an MCP connection
    #[error("mcp error: {0}")]
    McpError(String),

    /// An error occurred during tool execution
    #[error("tool error: {0}")]
    ToolError(String),

    /// A configuration error
    #[error("config error: {0}")]
    ConfigError(String),

    /// A webhook delivery error
    #[error("webhook error: {0}")]
    WebhookError(String),

    /// An internal error
    #[error("internal error: {0}")]
    Internal(String),

    /// Rate limit exceeded
    #[error("rate limited")]
    RateLimited,

    /// Unauthorized access
    #[error("unauthorized: {0}")]
    Unauthorized(String),

    /// Conflict with current state
    #[error("conflict: {0}")]
    Conflict(String),

    /// Capacity exhausted (max conversations, task budget, etc.)
    #[error("capacity exhausted: {0}")]
    CapacityExhausted(String),

    /// The bridge harness adapter is not yet wired. Returned by stubbed
    /// supervisor methods during the harness migration.
    #[error("harness not yet available")]
    HarnessUnavailable,

    /// The underlying coding-agent harness returned an error.
    #[error("harness error: {0}")]
    HarnessError(String),
}

/// Convenience Result type alias for bridge operations.
pub type Result<T> = std::result::Result<T, BridgeError>;

impl IntoResponse for BridgeError {
    fn into_response(self) -> Response {
        let (status, code) = match &self {
            BridgeError::AgentNotFound(_) => (StatusCode::NOT_FOUND, "agent_not_found"),
            BridgeError::ConversationNotFound(_) => {
                (StatusCode::NOT_FOUND, "conversation_not_found")
            }
            BridgeError::ConversationEnded(_) => (StatusCode::BAD_REQUEST, "conversation_ended"),
            BridgeError::InvalidRequest(_) => (StatusCode::BAD_REQUEST, "invalid_request"),
            BridgeError::ProviderError(_) => (StatusCode::INTERNAL_SERVER_ERROR, "provider_error"),
            BridgeError::McpError(_) => (StatusCode::INTERNAL_SERVER_ERROR, "mcp_error"),
            BridgeError::ToolError(_) => (StatusCode::INTERNAL_SERVER_ERROR, "tool_error"),
            BridgeError::ConfigError(_) => (StatusCode::INTERNAL_SERVER_ERROR, "config_error"),
            BridgeError::WebhookError(_) => (StatusCode::INTERNAL_SERVER_ERROR, "webhook_error"),
            BridgeError::Internal(_) => (StatusCode::INTERNAL_SERVER_ERROR, "internal_error"),
            BridgeError::RateLimited => (StatusCode::TOO_MANY_REQUESTS, "rate_limited"),
            BridgeError::Unauthorized(_) => (StatusCode::UNAUTHORIZED, "unauthorized"),
            BridgeError::Conflict(_) => (StatusCode::CONFLICT, "conflict"),
            BridgeError::CapacityExhausted(_) => {
                (StatusCode::TOO_MANY_REQUESTS, "capacity_exhausted")
            }
            BridgeError::HarnessUnavailable => {
                (StatusCode::SERVICE_UNAVAILABLE, "harness_unavailable")
            }
            BridgeError::HarnessError(_) => (StatusCode::BAD_GATEWAY, "harness_error"),
        };

        let body = serde_json::json!({
            "error": {
                "code": code,
                "message": self.to_string(),
            }
        });

        (status, axum::Json(body)).into_response()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_display_for_each_variant() {
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
        assert_eq!(
            BridgeError::CapacityExhausted("max conversations reached".into()).to_string(),
            "capacity exhausted: max conversations reached"
        );
    }

    #[test]
    fn test_capacity_exhausted_returns_429() {
        let err = BridgeError::CapacityExhausted("max conversations".into());
        let response = err.into_response();
        assert_eq!(response.status(), StatusCode::TOO_MANY_REQUESTS);
    }

    #[test]
    fn test_harness_unavailable_returns_503() {
        let err = BridgeError::HarnessUnavailable;
        let response = err.into_response();
        assert_eq!(response.status(), StatusCode::SERVICE_UNAVAILABLE);
    }

    #[test]
    fn test_harness_error_returns_502() {
        let err = BridgeError::HarnessError("transport reset".into());
        assert_eq!(err.to_string(), "harness error: transport reset");
        let response = err.into_response();
        assert_eq!(response.status(), StatusCode::BAD_GATEWAY);
    }
}
