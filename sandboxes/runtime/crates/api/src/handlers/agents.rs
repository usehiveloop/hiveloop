use axum::extract::{Path, State};
use axum::Json;
use bridge_core::BridgeError;
use serde_json::json;

use crate::state::AppState;

/// GET /agents — list all loaded agents.
pub async fn list_agents(State(state): State<AppState>) -> Json<serde_json::Value> {
    let agents = state.supervisor.list_agents();
    Json(json!(agents))
}

/// GET /agents/:agent_id — get agent details.
pub async fn get_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<Json<serde_json::Value>, BridgeError> {
    let agent = state
        .supervisor
        .get_agent(&agent_id)
        .ok_or_else(|| BridgeError::AgentNotFound(agent_id.clone()))?;

    Ok(Json(json!({
        "id": agent.definition.id,
        "name": agent.definition.name,
        "system_prompt": agent.definition.system_prompt,
        "version": agent.definition.version,
        "active_conversations": agent.active_conversation_count(),
    })))
}
