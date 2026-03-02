use axum::extract::{Path, State};
use axum::response::sse::{Event, KeepAlive, Sse};
use bridge_core::BridgeError;
use futures::stream::Stream;
use std::convert::Infallible;
use std::time::Duration;

use crate::sse::to_sse_event;
use crate::state::AppState;

/// GET /conversations/:conv_id/stream — SSE stream for a conversation.
pub async fn stream_conversation(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, BridgeError> {
    let (_, rx) = state
        .sse_streams
        .remove(&conv_id)
        .ok_or_else(|| BridgeError::ConversationNotFound(conv_id.clone()))?;

    let stream = futures::stream::unfold(rx, |mut rx| async move {
        match rx.recv().await {
            Some(event) => {
                let sse_event = to_sse_event(&event).unwrap_or_else(|_| {
                    Event::default()
                        .event("error")
                        .data("{\"error\": \"serialization error\"}")
                });
                Some((Ok(sse_event), rx))
            }
            None => None,
        }
    });

    Ok(Sse::new(stream).keep_alive(
        KeepAlive::new()
            .interval(Duration::from_secs(15))
            .text("ping"),
    ))
}
