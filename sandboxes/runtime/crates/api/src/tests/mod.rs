use axum::middleware as axum_mw;
use axum::Router;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio_util::sync::CancellationToken;
use webhooks::EventBus;

use crate::middleware::request_id;
use crate::router::build_router;
use crate::state::AppState;

/// Build a test `AppState` backed by a real (but empty) `AgentSupervisor`.
pub(crate) fn test_state() -> AppState {
    let mcp_manager = Arc::new(McpManager::new());
    let cancel = CancellationToken::new();
    let event_bus = Arc::new(EventBus::new(None, None, String::new(), String::new()));
    let supervisor = Arc::new(
        AgentSupervisor::new(mcp_manager, cancel.clone()).with_event_bus(Some(event_bus.clone())),
    );
    AppState::new(
        supervisor,
        "valid-control-plane-token".to_string(),
        None,
        cancel,
        event_bus,
    )
}

/// Build the application router with the request-id middleware applied,
/// using the given `AppState`.
pub(crate) fn app_with_request_id(state: AppState) -> Router {
    build_router(state).layer(axum_mw::from_fn(request_id))
}

/// Helper: read the full response body as bytes.
pub(crate) async fn body_bytes(response: axum::response::Response) -> Vec<u8> {
    axum::body::to_bytes(response.into_body(), usize::MAX)
        .await
        .expect("failed to read body")
        .to_vec()
}

/// Helper: read the full response body as a `serde_json::Value`.
pub(crate) async fn body_json(response: axum::response::Response) -> serde_json::Value {
    let bytes = body_bytes(response).await;
    serde_json::from_slice(&bytes).expect("body is not valid JSON")
}

mod agent_tools;
mod basic;
mod conversation_scoping;
mod middleware_errors;
mod push;
