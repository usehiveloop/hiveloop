use axum::extract::{Path, State};
use axum::Json;
use bridge_core::BridgeError;

use crate::state::AppState;

use super::types::{UpdateApiKeyRequest, UpdateApiKeyResponse};

/// PATCH /push/agents/{agent_id}/api-key — rotate an agent's LLM API key at runtime.
#[cfg_attr(feature = "openapi", utoipa::path(
    patch,
    path = "/push/agents/{agent_id}/api-key",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body = UpdateApiKeyRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "API key updated", body = UpdateApiKeyResponse),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn update_agent_api_key(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    Json(body): Json<UpdateApiKeyRequest>,
) -> Result<Json<UpdateApiKeyResponse>, BridgeError> {
    state
        .supervisor
        .update_agent_api_key(&agent_id, body.api_key)
        .await?;
    Ok(Json(UpdateApiKeyResponse {
        status: "updated".to_string(),
    }))
}
