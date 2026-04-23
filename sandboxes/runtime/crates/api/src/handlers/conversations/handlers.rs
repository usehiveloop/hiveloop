use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::BridgeError;
use serde_json::json;

use crate::state::AppState;

use super::types::{
    AbortConversationResponse, CreateConversationRequest, CreateConversationResponse,
    EndConversationResponse, SendMessageRequest, SendMessageResponse,
};

/// POST /agents/:agent_id/conversations — create a new conversation.
#[cfg_attr(feature = "openapi", utoipa::path(
    post,
    path = "/agents/{agent_id}/conversations",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    request_body(content = Option<CreateConversationRequest>, description = "Optional tool/MCP scoping filters"),
    responses(
        (status = 201, description = "Conversation created", body = CreateConversationResponse),
        (status = 400, description = "Invalid tool or MCP server name"),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn create_conversation(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
    body: Option<Json<CreateConversationRequest>>,
) -> Result<(StatusCode, Json<CreateConversationResponse>), BridgeError> {
    let request = body.map(|b| b.0).unwrap_or_default();
    let (conv_id, sse_rx) = state
        .supervisor
        .create_conversation(
            &agent_id,
            request.tool_names,
            request.mcp_server_names,
            request.api_key,
            request.subagent_api_keys,
            request.provider,
            request.mcp_servers,
        )
        .await?;

    // Store the SSE receiver for the stream handler to pick up
    state.sse_streams.insert(conv_id.clone(), sse_rx);

    state.event_bus.emit(BridgeEvent::new(
        BridgeEventType::ConversationCreated,
        &*agent_id,
        &*conv_id,
        json!({}),
    ));

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
    // `content` is `#[serde(default)]` so that callers who only supply
    // `full_message` can omit it (bridge auto-summarizes). Callers must
    // provide at least ONE of the two — an empty payload with neither is
    // an invalid request (preserves the pre-attachments 400 behavior for
    // malformed bodies like `{"invalid": true}`).
    if body.content.is_empty() && body.full_message.is_none() {
        return Err(BridgeError::InvalidRequest(
            "send_message requires either 'content' or 'full_message' to be set".into(),
        ));
    }

    // Find which agent owns this conversation
    let agent_id = super::helpers::find_agent_for_conversation(&state, &conv_id).await?;

    // If `full_message` was supplied, write it to disk and compose a
    // reminder pointing the agent at the attachment. Failure here is
    // intentionally non-fatal — we fall back to the caller's `content`
    // alone rather than rejecting the message.
    let (final_content, attachment_path_str) = if let Some(full) = &body.full_message {
        match crate::attachments::write_full_message(&conv_id, full).await {
            Some(path) => {
                let tools = state
                    .supervisor
                    .agent_tool_names(&agent_id)
                    .unwrap_or_default();
                let composed =
                    crate::attachments::compose_with_attachment(&body.content, full, &path, &tools);
                (composed, Some(path.display().to_string()))
            }
            None => (body.content.clone(), None),
        }
    } else {
        (body.content.clone(), None)
    };

    state.event_bus.emit(BridgeEvent::new(
        BridgeEventType::MessageReceived,
        &*agent_id,
        &*conv_id,
        json!({
            "content": &final_content,
            "attachment_path": attachment_path_str,
        }),
    ));

    state
        .supervisor
        .send_message(&agent_id, &conv_id, final_content, body.system_reminder)
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
    let agent_id = super::helpers::find_agent_for_conversation(&state, &conv_id).await?;

    state.supervisor.end_conversation(&agent_id, &conv_id)?;

    // Clean up SSE stream
    state.sse_streams.remove(&conv_id);
    state.event_bus.remove_sse_stream(&conv_id);

    // Remove any attachment files this conversation accumulated via
    // `full_message` payloads. Best-effort — failures are logged and
    // swallowed in the helper.
    crate::attachments::cleanup_conversation_attachments(&conv_id).await;

    state.event_bus.emit(BridgeEvent::new(
        BridgeEventType::ConversationEnded,
        &*agent_id,
        &*conv_id,
        json!({}),
    ));

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
    let agent_id = super::helpers::find_agent_for_conversation(&state, &conv_id).await?;
    state
        .supervisor
        .abort_conversation(&agent_id, &conv_id)
        .await?;
    Ok(Json(AbortConversationResponse {
        status: "aborted".to_string(),
    }))
}
