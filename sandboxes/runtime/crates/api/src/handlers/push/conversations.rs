use axum::extract::State;
use axum::Json;
use bridge_core::BridgeError;

use crate::state::AppState;

use super::types::{PushDiffRequest, PushDiffResponse};

/// POST /push/diff — apply a diff of agent changes.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/push/diff",
    request_body = PushDiffRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Diff applied", body = PushDiffResponse),
        (status = 401, description = "Unauthorized")
    )
))]
pub async fn push_diff(
    State(state): State<AppState>,
    Json(body): Json<PushDiffRequest>,
) -> Result<Json<PushDiffResponse>, BridgeError> {
    let added = body.added.len();
    let updated = body.updated.len();
    let removed = body.removed.len();

    state
        .supervisor
        .apply_diff(body.added, body.updated, body.removed)
        .await?;

    Ok(Json(PushDiffResponse {
        added,
        updated,
        removed,
    }))
}
