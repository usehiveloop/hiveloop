use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::Json;
use bridge_core::BridgeError;
use serde::Deserialize;
use serde_json::json;

use crate::state::AppState;

/// Request body for creating a message.
#[derive(Deserialize)]
pub struct SendMessageRequest {
    /// The text content to send.
    pub content: String,
}

/// POST /agents/:agent_id/conversations — create a new conversation.
pub async fn create_conversation(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<(StatusCode, Json<serde_json::Value>), BridgeError> {
    let (conv_id, sse_rx) = state.supervisor.create_conversation(&agent_id)?;

    // Store the SSE receiver for the stream handler to pick up
    state.sse_streams.insert(conv_id.clone(), sse_rx);

    Ok((
        StatusCode::CREATED,
        Json(json!({
            "conversation_id": conv_id,
            "stream_url": format!("/conversations/{}/stream", conv_id),
        })),
    ))
}

/// POST /conversations/:conv_id/messages — send a message to a conversation.
pub async fn send_message(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
    Json(body): Json<SendMessageRequest>,
) -> Result<(StatusCode, Json<serde_json::Value>), BridgeError> {
    // Find which agent owns this conversation
    let agent_id = find_agent_for_conversation(&state, &conv_id)?;

    state
        .supervisor
        .send_message(&agent_id, &conv_id, body.content)
        .await?;

    Ok((StatusCode::ACCEPTED, Json(json!({"status": "accepted"}))))
}

/// DELETE /conversations/:conv_id — end a conversation.
pub async fn end_conversation(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
) -> Result<Json<serde_json::Value>, BridgeError> {
    let agent_id = find_agent_for_conversation(&state, &conv_id)?;

    state.supervisor.end_conversation(&agent_id, &conv_id)?;

    // Clean up SSE stream
    state.sse_streams.remove(&conv_id);

    Ok(Json(json!({"status": "ended"})))
}

/// Find the agent that owns a conversation by searching all agents.
fn find_agent_for_conversation(state: &AppState, conv_id: &str) -> Result<String, BridgeError> {
    for summary in state.supervisor.list_agents() {
        if let Some(agent_state) = state.supervisor.get_agent(&summary.id) {
            if agent_state.has_conversation(conv_id) {
                return Ok(summary.id);
            }
        }
    }
    Err(BridgeError::ConversationNotFound(conv_id.to_string()))
}
