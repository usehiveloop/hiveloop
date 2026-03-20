use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::BridgeError;
use serde::{Deserialize, Serialize};
use serde_json::json;

use crate::state::AppState;

/// Response for creating a conversation.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CreateConversationResponse {
    /// The ID of the newly created conversation.
    pub conversation_id: String,
    /// The URL to stream events from this conversation.
    pub stream_url: String,
}

/// Response for sending a message.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SendMessageResponse {
    /// Status of the message acceptance.
    pub status: String,
}

/// Response for ending a conversation.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct EndConversationResponse {
    /// Status of the end operation.
    pub status: String,
}

/// Response for aborting a conversation turn.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AbortConversationResponse {
    /// Status of the abort operation.
    pub status: String,
}

/// Fire a webhook if the context is configured. Non-blocking, fire-and-forget.
fn emit_webhook(state: &AppState, payload: bridge_core::webhook::WebhookPayload) {
    if let Some(ref wh) = state.webhook_ctx {
        wh.dispatcher.dispatch(payload);
    }
}

/// Request body for creating a message.
#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SendMessageRequest {
    /// The text content to send.
    pub content: String,
}

/// POST /agents/:agent_id/conversations — create a new conversation.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/agents/{agent_id}/conversations",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    responses(
        (status = 201, description = "Conversation created", body = CreateConversationResponse),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn create_conversation(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<(StatusCode, Json<CreateConversationResponse>), BridgeError> {
    let (conv_id, sse_rx) = state.supervisor.create_conversation(&agent_id).await?;

    // Store the SSE receiver for the stream handler to pick up
    state.sse_streams.insert(conv_id.clone(), sse_rx);

    if let Some(ref wh) = state.webhook_ctx {
        emit_webhook(
            &state,
            webhooks::events::conversation_created(&agent_id, &conv_id, &wh.url, &wh.secret),
        );
    }

    Ok((
        StatusCode::CREATED,
        Json(CreateConversationResponse {
            conversation_id: conv_id.clone(),
            stream_url: format!("/conversations/{}/stream", conv_id),
        }),
    ))
}

/// POST /conversations/:conv_id/messages — send a message to a conversation.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/conversations/{conv_id}/messages",
    params(("conv_id" = String, Path, description = "Conversation identifier")),
    request_body = SendMessageRequest,
    responses(
        (status = 202, description = "Message accepted", body = SendMessageResponse),
        (status = 404, description = "Conversation not found")
    )
))]
pub async fn send_message(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
    Json(body): Json<SendMessageRequest>,
) -> Result<(StatusCode, Json<SendMessageResponse>), BridgeError> {
    // Find which agent owns this conversation
    let agent_id = find_agent_for_conversation(&state, &conv_id).await?;

    if let Some(ref wh) = state.webhook_ctx {
        emit_webhook(
            &state,
            webhooks::events::message_received(
                &agent_id,
                &conv_id,
                json!({"content": &body.content}),
                &wh.url,
                &wh.secret,
            ),
        );
    }

    state
        .supervisor
        .send_message(&agent_id, &conv_id, body.content)
        .await?;

    Ok((
        StatusCode::ACCEPTED,
        Json(SendMessageResponse {
            status: "accepted".to_string(),
        }),
    ))
}

/// DELETE /conversations/:conv_id — end a conversation.
#[cfg_attr(feature = "openapi", utoipa::path(
    delete,
    path = "/conversations/{conv_id}",
    params(("conv_id" = String, Path, description = "Conversation identifier")),
    responses(
        (status = 200, description = "Conversation ended", body = EndConversationResponse),
        (status = 404, description = "Conversation not found")
    )
))]
pub async fn end_conversation(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
) -> Result<Json<EndConversationResponse>, BridgeError> {
    let agent_id = find_agent_for_conversation(&state, &conv_id).await?;

    state.supervisor.end_conversation(&agent_id, &conv_id)?;

    // Clean up SSE stream
    state.sse_streams.remove(&conv_id);

    if let Some(ref wh) = state.webhook_ctx {
        emit_webhook(
            &state,
            webhooks::events::conversation_ended(&agent_id, &conv_id, &wh.url, &wh.secret),
        );
    }

    Ok(Json(EndConversationResponse {
        status: "ended".to_string(),
    }))
}

/// POST /conversations/:conv_id/abort — abort the current in-flight turn.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/conversations/{conv_id}/abort",
    params(("conv_id" = String, Path, description = "Conversation identifier")),
    responses(
        (status = 200, description = "Turn aborted", body = AbortConversationResponse),
        (status = 404, description = "Conversation not found")
    )
))]
pub async fn abort_conversation(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
) -> Result<Json<AbortConversationResponse>, BridgeError> {
    let agent_id = find_agent_for_conversation(&state, &conv_id).await?;
    state.supervisor.abort_conversation(&agent_id, &conv_id).await?;
    Ok(Json(AbortConversationResponse {
        status: "aborted".to_string(),
    }))
}

/// Find the agent that owns a conversation by searching all agents.
async fn find_agent_for_conversation(state: &AppState, conv_id: &str) -> Result<String, BridgeError> {
    for summary in state.supervisor.list_agents().await {
        if let Some(agent_state) = state.supervisor.get_agent(&summary.id) {
            if agent_state.has_conversation(conv_id) {
                return Ok(summary.id);
            }
        }
    }
    Err(BridgeError::ConversationNotFound(conv_id.to_string()))
}
