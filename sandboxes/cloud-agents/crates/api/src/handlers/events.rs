use axum::extract::{Query, State};
use axum::Json;
use bridge_core::event::BridgeEvent;
use bridge_core::BridgeError;
use serde::Deserialize;

use crate::state::AppState;

#[derive(Deserialize)]
pub struct EventsParams {
    /// Return events with sequence_number greater than this value.
    /// Clients use the last sequence_number they received from WS/SSE
    /// as the cursor for polling on reconnection.
    pub after: Option<u64>,
    /// Maximum number of events to return (default 100, max 1000).
    pub limit: Option<u32>,
    /// Authentication token (must match the control plane API key).
    pub token: Option<String>,
}

/// GET /events — poll for events from a point in time.
///
/// This endpoint is the fallback when WebSocket or SSE connections fail.
/// Clients pass `after=<sequence_number>` to fetch events they missed.
/// Returns the same `BridgeEvent` payload as WS and SSE.
pub async fn poll_events(
    State(state): State<AppState>,
    Query(params): Query<EventsParams>,
) -> Result<Json<Vec<BridgeEvent>>, BridgeError> {
    // Authenticate
    let token = params
        .token
        .ok_or_else(|| BridgeError::Unauthorized("missing token parameter".into()))?;

    if token != state.control_plane_api_key {
        return Err(BridgeError::Unauthorized("invalid token".into()));
    }

    let after = params.after.unwrap_or(0);
    let limit = params.limit.unwrap_or(100).min(1000);

    let backend = state
        .storage_backend
        .as_ref()
        .ok_or_else(|| BridgeError::InvalidRequest("storage not enabled".into()))?;

    let events = backend
        .load_events_since(after, limit)
        .await
        .map_err(|e| BridgeError::Internal(format!("failed to load events: {e}")))?;

    Ok(Json(events))
}
