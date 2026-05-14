use axum::extract::{Path, State};
use axum::http::HeaderMap;
use axum::response::sse::{Event, KeepAlive, Sse};
use bridge_core::event::BridgeEvent;
use bridge_core::BridgeError;
use futures::stream::{Stream, StreamExt};
use std::convert::Infallible;
use std::time::Duration;
use tokio::sync::broadcast;

use crate::sse::to_sse_event;
use crate::state::AppState;

/// GET /conversations/:conv_id/stream — SSE stream for a conversation.
///
/// Multi-subscriber: any number of clients can attach to the same
/// conversation; each gets an independent broadcast receiver. Clients
/// reconnecting after a disconnect can pass `Last-Event-ID` (a numeric
/// `sequence_number`) to receive any persisted events with sequence
/// strictly greater than the last seen value before the live stream resumes.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/conversations/{conv_id}/stream",
    params(("conv_id" = String, Path, description = "Conversation identifier")),
    responses(
        (status = 200, description = "SSE event stream", content_type = "text/event-stream"),
        (status = 404, description = "Conversation not found")
    )
))]
pub async fn stream_conversation(
    State(state): State<AppState>,
    Path(conv_id): Path<String>,
    headers: HeaderMap,
) -> Result<Sse<impl Stream<Item = Result<Event, Infallible>>>, BridgeError> {
    // Reject if the conversation isn't known to the supervisor at all.
    // Without this check we'd happily open a long-lived stream for a typo.
    super::conversations::helpers::find_agent_for_conversation(&state, &conv_id).await?;

    let last_event_id = headers
        .get("last-event-id")
        .and_then(|v| v.to_str().ok())
        .and_then(|v| v.trim().parse::<u64>().ok());

    // Subscribe FIRST, then read history. Order matters: any event emitted
    // after we subscribe but before we finish replaying history is buffered
    // by the broadcast channel and delivered live; events emitted before we
    // subscribed are picked up from the DB during the replay phase. Result:
    // no gap, no dup, monotonic by sequence_number.
    let live_rx = state.event_bus.subscribe_sse(&conv_id);

    let replay = if let (Some(last_id), Some(backend)) = (last_event_id, &state.storage_backend) {
        backend
            .load_events_since_for_conversation(&conv_id, last_id, REPLAY_LIMIT)
            .await
            .unwrap_or_default()
    } else {
        Vec::new()
    };

    let stream = build_stream(replay, live_rx, last_event_id);

    Ok(Sse::new(stream).keep_alive(
        KeepAlive::new()
            .interval(Duration::from_secs(15))
            .text("ping"),
    ))
}

/// Maximum number of historical events replayed in a single Last-Event-ID
/// catch-up. A client this far behind is already pathological — anything
/// beyond this should be reconciled out-of-band.
const REPLAY_LIMIT: u32 = 10_000;

fn build_stream(
    replay: Vec<BridgeEvent>,
    live_rx: broadcast::Receiver<BridgeEvent>,
    last_event_id: Option<u64>,
) -> impl Stream<Item = Result<Event, Infallible>> {
    use tokio_stream::wrappers::BroadcastStream;

    let replayed_max = replay.last().map(|e| e.sequence_number);
    let cursor = last_event_id.unwrap_or(0).max(replayed_max.unwrap_or(0));

    let replay_stream = futures::stream::iter(replay.into_iter().filter_map(move |ev| {
        if let Some(last) = last_event_id {
            if ev.sequence_number <= last {
                return None;
            }
        }
        Some(serialize(&ev))
    }));

    // BroadcastStream surfaces lagged errors as `Lagged(skipped)`. The
    // client should reconnect with the last sequence_number it actually
    // received and let DB replay fill the gap. We log lag at error level
    // so it surfaces in Sentry — repeated lag means a slow consumer or
    // an undersized broadcast buffer.
    let live_stream = BroadcastStream::new(live_rx).filter_map(move |item| {
        let cursor = cursor;
        async move {
            match item {
                Ok(ev) if ev.sequence_number > cursor => Some(serialize(&ev)),
                Ok(_) => None,
                Err(tokio_stream::wrappers::errors::BroadcastStreamRecvError::Lagged(n)) => {
                    tracing::error!(
                        skipped = n,
                        "SSE subscriber fell behind broadcast buffer; events dropped"
                    );
                    None
                }
            }
        }
    });

    replay_stream.chain(live_stream)
}

fn serialize(event: &BridgeEvent) -> Result<Event, Infallible> {
    let sse_event = to_sse_event(event).unwrap_or_else(|_| {
        Event::default()
            .event("error")
            .data("{\"error\": \"serialization error\"}")
    });
    Ok(sse_event.id(event.sequence_number.to_string()))
}
