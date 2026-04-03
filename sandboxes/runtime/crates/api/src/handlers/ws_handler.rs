use axum::extract::ws::{Message, WebSocket};
use axum::extract::{Query, State, WebSocketUpgrade};
use axum::response::IntoResponse;
use bridge_core::BridgeError;
use serde::Deserialize;
use tokio::sync::broadcast;

use crate::state::AppState;

#[derive(Deserialize)]
pub struct WsParams {
    /// Authentication token (must match the control plane API key).
    token: Option<String>,
}

/// WebSocket upgrade handler for the `/ws/events` endpoint.
///
/// Authenticates via `?token=<api_key>` query parameter (WebSocket clients
/// cannot always set custom headers), then upgrades to a persistent WebSocket
/// connection that receives ALL events from ALL agents and conversations.
pub async fn ws_events(
    State(state): State<AppState>,
    Query(params): Query<WsParams>,
    ws: WebSocketUpgrade,
) -> Result<impl IntoResponse, BridgeError> {
    // Authenticate
    let token = params
        .token
        .ok_or_else(|| BridgeError::Unauthorized("missing token parameter".into()))?;

    if token != state.control_plane_api_key {
        return Err(BridgeError::Unauthorized("invalid token".into()));
    }

    let broadcaster = state
        .ws_broadcaster
        .as_ref()
        .ok_or_else(|| {
            BridgeError::InvalidRequest("WebSocket event stream is not enabled".into())
        })?
        .clone();

    let rx = broadcaster.subscribe();
    let cancel = state.cancel.clone();

    Ok(ws.on_upgrade(move |socket| handle_socket(socket, rx, cancel)))
}

/// Main WebSocket connection loop. Forwards broadcast events to the client
/// and handles ping/pong and close frames.
async fn handle_socket(
    mut socket: WebSocket,
    mut rx: broadcast::Receiver<bridge_core::webhook::WebhookPayload>,
    cancel: tokio_util::sync::CancellationToken,
) {
    loop {
        tokio::select! {
            biased;

            _ = cancel.cancelled() => {
                let _ = socket.send(Message::Close(None)).await;
                break;
            }

            result = rx.recv() => {
                match result {
                    Ok(payload) => {
                        let ws_event = strip_secrets(&payload);
                        match serde_json::to_string(&ws_event) {
                            Ok(json) => {
                                if socket.send(Message::Text(json.into())).await.is_err() {
                                    break; // client disconnected
                                }
                            }
                            Err(e) => {
                                tracing::warn!(error = %e, "failed to serialize WS event");
                            }
                        }
                    }
                    Err(broadcast::error::RecvError::Lagged(n)) => {
                        let warning = serde_json::json!({
                            "type": "lagged",
                            "missed_events": n
                        });
                        if let Ok(json) = serde_json::to_string(&warning) {
                            if socket.send(Message::Text(json.into())).await.is_err() {
                                break;
                            }
                        }
                    }
                    Err(broadcast::error::RecvError::Closed) => break,
                }
            }

            msg = socket.recv() => {
                match msg {
                    Some(Ok(Message::Close(_))) | None => break,
                    Some(Ok(Message::Ping(data))) => {
                        if socket.send(Message::Pong(data)).await.is_err() {
                            break;
                        }
                    }
                    Some(Err(_)) => break,
                    _ => {} // ignore text/binary from client
                }
            }
        }
    }
}

/// Strip webhook-specific sensitive fields before sending over WebSocket.
fn strip_secrets(
    payload: &bridge_core::webhook::WebhookPayload,
) -> serde_json::Value {
    let mut value = serde_json::to_value(payload).unwrap_or_default();
    if let Some(obj) = value.as_object_mut() {
        obj.remove("webhook_url");
        obj.remove("webhook_secret");
    }
    value
}
