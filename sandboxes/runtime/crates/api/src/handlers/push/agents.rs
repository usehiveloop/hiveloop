use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::{AgentDefinition, BridgeError};

use crate::state::AppState;

use super::helpers::{definitions_equivalent, restore_stored_conversations_for_agent};
use super::types::{
    PushAgentsRequest, PushAgentsResponse, RemoveAgentResponse, UpsertAgentResponse,
};

/// POST /push/agents — bulk seed agents.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/push/agents",
    request_body = PushAgentsRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Agents loaded", body = PushAgentsResponse),
        (status = 401, description = "Unauthorized")
    )
))]
pub async fn push_agents(
    State(state): State<AppState>,
    Json(body): Json<PushAgentsRequest>,
) -> Result<(StatusCode, Json<PushAgentsResponse>), BridgeError> {
    // Semantic validation before handing to the supervisor. Catches things
    // like tool_requirements ∩ disabled_tools conflicts up-front so the
    // caller sees a clear 400 instead of a silently-broken agent.
    for agent in &body.agents {
        agent
            .validate()
            .map_err(|msg| BridgeError::InvalidRequest(format!("agent '{}': {}", agent.id, msg)))?;
    }

    let count = body.agents.len();
    let agent_ids: Vec<String> = body.agents.iter().map(|agent| agent.id.clone()).collect();
    state.supervisor.load_agents(body.agents).await?;

    for agent_id in agent_ids {
        restore_stored_conversations_for_agent(&state, &agent_id).await?;
    }

    Ok((StatusCode::OK, Json(PushAgentsResponse { loaded: count })))
}

/// PUT /push/agents/{agent_id} — add if new, update if version differs, no-op if same version.
#[cfg_attr(feature = "openapi", utoipa::path(
    put,
    path = "/push/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body = AgentDefinition,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Agent unchanged or updated", body = UpsertAgentResponse),
        (status = 201, description = "Agent created", body = UpsertAgentResponse),
        (status = 400, description = "Invalid request"),
        (status = 401, description = "Unauthorized")
    )
))]
pub async fn upsert_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    Json(agent): Json<AgentDefinition>,
) -> Result<(StatusCode, Json<UpsertAgentResponse>), BridgeError> {
    if agent.id != agent_id {
        return Err(BridgeError::InvalidRequest(format!(
            "path agent_id '{}' does not match body id '{}'",
            agent_id, agent.id
        )));
    }

    // Semantic validation (tool_requirements ∩ disabled_tools, etc.)
    agent
        .validate()
        .map_err(|msg| BridgeError::InvalidRequest(format!("agent '{}': {}", agent.id, msg)))?;

    // Check if agent already exists
    if let Some(existing) = state.supervisor.get_agent(&agent_id) {
        let existing_def = existing.definition.read().await.clone();
        // Same version → no-op
        if definitions_equivalent(&existing_def, &agent) {
            return Ok((
                StatusCode::OK,
                Json(UpsertAgentResponse {
                    status: "unchanged".to_string(),
                }),
            ));
        }
        // Different version → update
        state
            .supervisor
            .apply_diff(vec![], vec![agent], vec![])
            .await?;
        restore_stored_conversations_for_agent(&state, &agent_id).await?;
        Ok((
            StatusCode::OK,
            Json(UpsertAgentResponse {
                status: "updated".to_string(),
            }),
        ))
    } else {
        // New agent → add
        state
            .supervisor
            .apply_diff(vec![agent], vec![], vec![])
            .await?;
        restore_stored_conversations_for_agent(&state, &agent_id).await?;
        Ok((
            StatusCode::CREATED,
            Json(UpsertAgentResponse {
                status: "created".to_string(),
            }),
        ))
    }
}

/// DELETE /push/agents/{agent_id} — remove an agent.
#[cfg_attr(feature = "openapi", utoipa::path(
    delete,
    path = "/push/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Agent removed", body = RemoveAgentResponse),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn remove_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<Json<RemoveAgentResponse>, BridgeError> {
    if state.supervisor.get_agent(&agent_id).is_none() {
        return Err(BridgeError::AgentNotFound(agent_id));
    }

    state
        .supervisor
        .apply_diff(vec![], vec![], vec![agent_id])
        .await?;
    Ok(Json(RemoveAgentResponse {
        status: "removed".to_string(),
    }))
}
