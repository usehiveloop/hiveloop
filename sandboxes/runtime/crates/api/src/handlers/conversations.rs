use std::collections::HashMap;

use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::event::{BridgeEvent, BridgeEventType};
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

/// Optional request body for creating a conversation with tool/MCP scoping.
#[derive(Deserialize, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CreateConversationRequest {
    /// When provided, only these tools are available in the conversation.
    /// Tool names must match the agent's registered tool names exactly.
    #[serde(default)]
    pub tool_names: Option<Vec<String>>,

    /// When provided, only tools from these MCP servers are available.
    /// Server names must match the agent's configured MCP server names.
    #[serde(default)]
    pub mcp_server_names: Option<Vec<String>>,

    /// When provided, overrides the agent's LLM API key for this conversation only.
    /// For full provider/model override, use the `provider` field instead.
    #[serde(default)]
    pub api_key: Option<String>,

    /// When provided, fully overrides the agent's LLM provider for this conversation.
    /// Allows switching model, provider type, API key, and base URL per conversation
    /// while keeping the same agent definition (tools, system prompt, skills, etc.).
    #[serde(default)]
    pub provider: Option<bridge_core::ProviderConfig>,

    /// Per-subagent API key overrides. Key = subagent name, Value = API key.
    /// Only named subagents are overridden; others keep their configured keys.
    #[serde(default)]
    pub subagent_api_keys: Option<HashMap<String, String>>,

    /// Additional MCP servers to load for this conversation only.
    /// Connected at conversation creation, torn down when the conversation ends
    /// (or is aborted, drained, or cancelled). Tool names produced by these
    /// servers must not collide with the agent's existing tool names.
    ///
    /// Stdio transport requires the runtime config flag
    /// `allow_stdio_mcp_from_api` to be enabled; otherwise only
    /// `streamable_http` is accepted.
    #[serde(default)]
    pub mcp_servers: Option<Vec<bridge_core::mcp::McpServerDefinition>>,
}

/// Request body for creating a message.
#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SendMessageRequest {
    /// The text content to send. When [`full_message`](Self::full_message) is
    /// also supplied, `content` is the LLM-visible summary; omit it to let
    /// bridge auto-generate one from the first bytes of `full_message`.
    #[serde(default)]
    pub content: String,
    /// Optional system reminder to inject with this message.
    /// Will be wrapped in `<system-reminder>` tags and prepended to the user message.
    #[serde(default)]
    pub system_reminder: Option<String>,
    /// Optional full payload written to a per-conversation attachment file.
    /// When present, bridge writes it to disk, appends a `<system-reminder>`
    /// with the file path and tool-usage hint to `content`, and sends the
    /// composed text to the LLM. Callers use this to offload large inputs
    /// (stack traces, log dumps, file contents) without bloating the
    /// agent's context on every turn.
    ///
    /// Failures (disk full, permission denied) do NOT reject the message —
    /// bridge logs a warning and delivers `content` alone.
    #[serde(default)]
    pub full_message: Option<String>,
}

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
    let agent_id = find_agent_for_conversation(&state, &conv_id).await?;

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
    let agent_id = find_agent_for_conversation(&state, &conv_id).await?;

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
    let agent_id = find_agent_for_conversation(&state, &conv_id).await?;
    state
        .supervisor
        .abort_conversation(&agent_id, &conv_id)
        .await?;
    Ok(Json(AbortConversationResponse {
        status: "aborted".to_string(),
    }))
}

/// Find the agent that owns a conversation by searching all agents.
async fn find_agent_for_conversation(
    state: &AppState,
    conv_id: &str,
) -> Result<String, BridgeError> {
    for summary in state.supervisor.list_agents().await {
        if let Some(agent_state) = state.supervisor.get_agent(&summary.id) {
            if agent_state.has_conversation(conv_id) {
                return Ok(summary.id);
            }
        }
    }
    Err(BridgeError::ConversationNotFound(conv_id.to_string()))
}
