use axum::extract::State;
use axum::Json;
use serde::Serialize;

use crate::state::AppState;

/// Health check response.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HealthResponse {
    /// Health status - always "ok" if the server is running.
    pub status: String,
    /// Server uptime in seconds.
    pub uptime_secs: u64,
}

/// GET /health — health check endpoint.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/health",
    responses(
        (status = 200, description = "Health check", body = HealthResponse)
    )
))]
pub async fn health(State(state): State<AppState>) -> Json<HealthResponse> {
    let uptime_secs = state.startup_time.elapsed().as_secs();
    Json(HealthResponse {
        status: "ok".to_string(),
        uptime_secs,
    })
}
