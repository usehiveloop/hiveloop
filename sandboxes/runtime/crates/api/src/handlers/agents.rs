use axum::extract::{Path, State};
use axum::Json;
use bridge_core::BridgeError;
use serde::Serialize;
use serde_json::json;

use crate::state::AppState;

/// Response for getting agent details.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentDetailsResponse {
    /// Agent identifier.
    pub id: String,
    /// Agent display name.
    pub name: String,
    /// System prompt used by the agent.
    pub system_prompt: String,
    /// Agent version.
    pub version: Option<String>,
    /// Number of currently active conversations.
    pub active_conversations: usize,
}

/// GET /agents — list all loaded agents.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/agents",
    responses(
        (status = 200, description = "List of agent summaries", body = Vec<bridge_core::AgentSummary>)
    )
))]
pub async fn list_agents(State(state): State<AppState>) -> Json<serde_json::Value> {
    let agents = state.supervisor.list_agents().await;
    Json(json!(agents))
}

/// GET /agents/:agent_id — get agent details.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    responses(
        (status = 200, description = "Agent details", body = AgentDetailsResponse),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn get_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<Json<AgentDetailsResponse>, BridgeError> {
    let agent = state
        .supervisor
        .get_agent(&agent_id)
        .ok_or_else(|| BridgeError::AgentNotFound(agent_id.clone()))?;

    let def = agent.definition.read().await;
    Ok(Json(AgentDetailsResponse {
        id: def.id.clone(),
        name: def.name.clone(),
        system_prompt: def.system_prompt.clone(),
        version: def.version.clone(),
        active_conversations: agent.active_conversation_count(),
    }))
}
