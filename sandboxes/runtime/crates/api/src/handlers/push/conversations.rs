use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::BridgeError;

use crate::state::AppState;

use super::helpers::restore_stored_conversations_for_agent;
use super::types::{
    HydrateConversationsRequest, HydrateConversationsResponse, PushDiffRequest, PushDiffResponse,
};

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
        .hydrate_conversations(&agent_id, body.conversations)
        .await;
    for (conv_id, sse_rx) in sse_receivers {
        state.sse_streams.insert(conv_id, sse_rx);
    }

    Ok((
        StatusCode::OK,
        Json(HydrateConversationsResponse { hydrated: count }),
    ))
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
    let agent_ids: Vec<String> = body
        .added
        .iter()
        .chain(body.updated.iter())
        .map(|agent| agent.id.clone())
        .collect();

    state
        .supervisor
        .apply_diff(body.added, body.updated, body.removed)
        .await?;

    for agent_id in agent_ids {
        restore_stored_conversations_for_agent(&state, &agent_id).await?;
    }

    Ok(Json(PushDiffResponse {
        added,
        updated,
        removed,
    }))
}
