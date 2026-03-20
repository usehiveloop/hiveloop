use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::{AgentDefinition, BridgeError, ConversationRecord};
use serde::{Deserialize, Serialize};

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

#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpdateApiKeyRequest {
    pub api_key: String,
}

/// Response for pushing agents.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushAgentsResponse {
    /// Number of agents loaded.
    pub loaded: usize,
}

/// Response for upserting an agent.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpsertAgentResponse {
    /// Status of the operation: "unchanged", "updated", or "created".
    pub status: String,
}

/// Response for removing an agent.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct RemoveAgentResponse {
    /// Status of the operation.
    pub status: String,
}

/// Response for hydrating conversations.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HydrateConversationsResponse {
    /// Number of conversations hydrated.
    pub hydrated: usize,
}

/// Response for updating an API key.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct UpdateApiKeyResponse {
    /// Status of the operation.
    pub status: String,
}

/// Response for pushing a diff.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PushDiffResponse {
    /// Number of agents added.
    pub added: usize,
    /// Number of agents updated.
    pub updated: usize,
    /// Number of agents removed.
    pub removed: usize,
}

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
    let count = body.agents.len();
    state.supervisor.load_agents(body.agents).await?;
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

    // Check if agent already exists
    if let Some(existing) = state.supervisor.get_agent(&agent_id) {
        // Same version → no-op
        if existing.version().await.as_deref() == agent.version.as_deref() {
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

/// POST /push/agents/{agent_id}/conversations — hydrate conversations for an agent.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/push/agents/{agent_id}/conversations",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body = HydrateConversationsRequest,
    security(("bearer" = [])),
    responses(
        (status = 200, description = "Conversations hydrated", body = HydrateConversationsResponse),
        (status = 401, description = "Unauthorized"),
        (status = 404, description = "Agent not found"),
        (status = 409, description = "Agent has active conversations")
    )
))]
pub async fn hydrate_conversations(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    Json(body): Json<HydrateConversationsRequest>,
) -> Result<(StatusCode, Json<HydrateConversationsResponse>), BridgeError> {
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
        .hydrate_conversations(&agent_id, body.conversations).await;
    for (conv_id, sse_rx) in sse_receivers {
        state.sse_streams.insert(conv_id, sse_rx);
    }

    Ok((
        StatusCode::OK,
        Json(HydrateConversationsResponse { hydrated: count }),
    ))
}

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
        .update_agent_api_key(&agent_id, body.api_key).await?;
    Ok(Json(UpdateApiKeyResponse {
        status: "updated".to_string(),
    }))
}

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
