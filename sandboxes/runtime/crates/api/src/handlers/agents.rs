use axum::extract::{Path, State};
use axum::Json;
use bridge_core::BridgeError;
use serde_json::json;

use crate::state::AppState;

/// GET /agents — list all loaded agents.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/agents",
    responses(
        (status = 200, description = "List of agent summaries", body = Vec<bridge_core::AgentSummary>)
    )
))]
pub async fn list_agents(State(state): State<AppState>) -> Json<serde_json::Value> {
    let agents = state.supervisor.list_agents();
    Json(json!(agents))
}

/// GET /agents/:agent_id — get agent details.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    responses(
        (status = 200, description = "Agent details", body = serde_json::Value),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn get_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<Json<serde_json::Value>, BridgeError> {
    let agent = state
        .supervisor
        .get_agent(&agent_id)
        .ok_or_else(|| BridgeError::AgentNotFound(agent_id.clone()))?;

    let def = agent.definition.read().unwrap();
    Ok(Json(json!({
        "id": def.id,
        "name": def.name,
        "system_prompt": def.system_prompt,
        "version": def.version,
        "active_conversations": agent.active_conversation_count(),
    })))
}
