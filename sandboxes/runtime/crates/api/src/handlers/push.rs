use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::{AgentDefinition, BridgeError, ConversationRecord};
use serde::Deserialize;
use serde_json::json;

use crate::state::AppState;

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushAgentsRequest {
    pub agents: Vec<AgentDefinition>,
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HydrateConversationsRequest {
    pub conversations: Vec<ConversationRecord>,
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushDiffRequest {
    pub added: Vec<AgentDefinition>,
    pub updated: Vec<AgentDefinition>,
    pub removed: Vec<String>,
}

/// POST /push/agents — bulk seed agents.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/push/agents",
    request_body = PushAgentsRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Agents loaded", body = serde_json::Value),
        (status = 401, description = "Unauthorized")
    )
))]
pub async fn push_agents(
    State(state): State<AppState>,
    Json(body): Json<PushAgentsRequest>,
) -> Result<(StatusCode, Json<serde_json::Value>), BridgeError> {
    let count = body.agents.len();
    state.supervisor.load_agents(body.agents).await?;
    Ok((StatusCode::OK, Json(json!({"loaded": count}))))
}

/// PUT /push/agents/{agent_id} — add if new, update if version differs, no-op if same version.
#[cfg_attr(feature = "openapi", utoipa::path(
    put,
    path = "/push/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body = AgentDefinition,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Agent unchanged or updated", body = serde_json::Value),
        (status = 201, description = "Agent created", body = serde_json::Value),
        (status = 400, description = "Invalid request"),
        (status = 401, description = "Unauthorized")
    )
))]
pub async fn upsert_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    Json(agent): Json<AgentDefinition>,
) -> Result<(StatusCode, Json<serde_json::Value>), BridgeError> {
    if agent.id != agent_id {
        return Err(BridgeError::InvalidRequest(format!(
            "path agent_id '{}' does not match body id '{}'",
            agent_id, agent.id
        )));
    }

    // Check if agent already exists
    if let Some(existing) = state.supervisor.get_agent(&agent_id) {
        // Same version → no-op
        if existing.version().as_deref() == agent.version.as_deref() {
            return Ok((StatusCode::OK, Json(json!({"status": "unchanged"}))));
        }
        // Different version → update
        state
            .supervisor
            .apply_diff(vec![], vec![agent], vec![])
            .await?;
        Ok((StatusCode::OK, Json(json!({"status": "updated"}))))
    } else {
        // New agent → add
        state
            .supervisor
            .apply_diff(vec![agent], vec![], vec![])
            .await?;
        Ok((StatusCode::CREATED, Json(json!({"status": "created"}))))
    }
}

/// DELETE /push/agents/{agent_id} — remove an agent.
#[cfg_attr(feature = "openapi", utoipa::path(
    delete,
    path = "/push/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Agent removed", body = serde_json::Value),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn remove_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<Json<serde_json::Value>, BridgeError> {
    if state.supervisor.get_agent(&agent_id).is_none() {
        return Err(BridgeError::AgentNotFound(agent_id));
    }

    state
        .supervisor
        .apply_diff(vec![], vec![], vec![agent_id])
        .await?;
    Ok(Json(json!({"status": "removed"})))
}

/// POST /push/agents/{agent_id}/conversations — hydrate conversations for an agent.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/push/agents/{agent_id}/conversations",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body = HydrateConversationsRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Conversations hydrated", body = serde_json::Value),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "Agent not found"),
        (status = 409, description = "Agent has active conversations")
    )
))]
pub async fn hydrate_conversations(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    Json(body): Json<HydrateConversationsRequest>,
) -> Result<(StatusCode, Json<serde_json::Value>), BridgeError> {
    let agent = state
        .supervisor
        .get_agent(&agent_id)
        .ok_or_else(|| BridgeError::AgentNotFound(agent_id.clone()))?;

    if agent.active_conversation_count() > 0 {
        return Err(BridgeError::Conflict(format!(
            "agent '{}' has {} active conversation(s); cannot hydrate",
            agent_id,
            agent.active_conversation_count()
        )));
    }

    let count = body.conversations.len();
    let sse_receivers = state
        .supervisor
        .hydrate_conversations(&agent_id, body.conversations);
    for (conv_id, sse_rx) in sse_receivers {
        state.sse_streams.insert(conv_id, sse_rx);
    }

    Ok((StatusCode::OK, Json(json!({"hydrated": count}))))
}

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpdateApiKeyRequest {
    pub api_key: String,
}

/// PATCH /push/agents/{agent_id}/api-key — rotate an agent's LLM API key at runtime.
#[cfg_attr(feature = "openapi", utoipa::path(
    patch,
    path = "/push/agents/{agent_id}/api-key",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body = UpdateApiKeyRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "API key updated", body = serde_json::Value),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn update_agent_api_key(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    Json(body): Json<UpdateApiKeyRequest>,
) -> Result<Json<serde_json::Value>, BridgeError> {
    state
        .supervisor
        .update_agent_api_key(&agent_id, body.api_key)?;
    Ok(Json(json!({"status": "updated"})))
}

/// POST /push/diff — apply a diff of agent changes.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/push/diff",
    request_body = PushDiffRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Diff applied", body = serde_json::Value),
        (status = 401, description = "Unauthorized")
    )
))]
pub async fn push_diff(
    State(state): State<AppState>,
    Json(body): Json<PushDiffRequest>,
) -> Result<Json<serde_json::Value>, BridgeError> {
    let added = body.added.len();
    let updated = body.updated.len();
    let removed = body.removed.len();

    state
        .supervisor
        .apply_diff(body.added, body.updated, body.removed)
        .await?;

    Ok(Json(
        json!({"added": added, "updated": updated, "removed": removed}),
    ))
}
