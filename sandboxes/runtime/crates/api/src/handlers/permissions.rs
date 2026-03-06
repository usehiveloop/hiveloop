use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::permission::{ApprovalReply, ApprovalRequest, BulkApprovalReply};
use serde_json::json;

use crate::state::AppState;

/// List all pending approval requests for a conversation.
pub async fn list_approvals(
    State(state): State<AppState>,
    Path((_agent_id, conv_id)): Path<(String, String)>,
) -> Json<Vec<ApprovalRequest>> {
    let pending = state.permission_manager.list_pending(&conv_id);
    Json(pending)
}

/// Resolve a single pending approval request.
pub async fn resolve_approval(
    State(state): State<AppState>,
    Path((_agent_id, conv_id, request_id)): Path<(String, String, String)>,
    Json(body): Json<ApprovalReply>,
) -> Result<Json<serde_json::Value>, StatusCode> {
    // Find the SSE sender for this conversation so we can emit resolution events
    let sse_tx = state.sse_streams.get(&conv_id).map(|entry| {
        // We can't borrow the receiver's sender — but we stored receivers, not senders.
        // Resolution SSE events are sent via try_send in the manager itself,
        // which already has access if needed. For now, pass None.
        drop(entry);
    });
    let _ = sse_tx;

    let resolved =
        state
            .permission_manager
            .resolve(&request_id, body.decision, None, &state.webhook_ctx);

    if resolved {
        Ok(Json(json!({"status": "resolved", "request_id": request_id})))
    } else {
        Err(StatusCode::NOT_FOUND)
    }
}

/// Bulk resolve multiple pending approval requests.
pub async fn bulk_resolve_approvals(
    State(state): State<AppState>,
    Path((_agent_id, _conv_id)): Path<(String, String)>,
    Json(body): Json<BulkApprovalReply>,
) -> Json<serde_json::Value> {
    let mut resolved = Vec::new();
    let mut not_found = Vec::new();

    for request_id in &body.request_ids {
        if state.permission_manager.resolve(
            request_id,
            body.decision.clone(),
            None,
            &state.webhook_ctx,
        ) {
            resolved.push(request_id.clone());
        } else {
            not_found.push(request_id.clone());
        }
    }

    Json(json!({
        "resolved": resolved,
        "not_found": not_found,
    }))
}
