use axum::extract::State;
use axum::Json;
use serde_json::json;

use crate::state::AppState;

/// GET /health — health check endpoint.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/health",
    responses(
        (status = 200, description = "Health check", body = serde_json::Value)
    )
))]
pub async fn health(State(state): State<AppState>) -> Json<serde_json::Value> {
    let uptime_secs = state.startup_time.elapsed().as_secs();
    Json(json!({
        "status": "ok",
        "uptime_secs": uptime_secs,
    }))
}
