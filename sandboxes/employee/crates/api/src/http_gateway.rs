use std::collections::HashMap;
use std::convert::Infallible;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use anyhow::Result;
use async_stream::stream;
use axum::response::sse::{Event, Sse};
use chrono::Utc;
use domain::{Attachment, InboundEvent, MessageHandle, SessionId};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::sync::{broadcast, Mutex};
use tokio::time::Duration;

#[derive(Clone)]
pub struct HttpGatewayState {
    pub inbound_sink: tokio::sync::mpsc::Sender<InboundEvent>,
    pub broker: Arc<HttpStreamBroker>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HttpStreamEvent {
    pub event: String,
    pub payload: Value,
}

#[derive(Default)]
pub struct HttpStreamBroker {
    streams: Mutex<HashMap<String, StreamState>>,
    session_streams: Mutex<HashMap<String, String>>,
    active_session_streams: Mutex<HashMap<String, String>>,
    counter: AtomicU64,
}

struct StreamState {
    sender: broadcast::Sender<HttpStreamEvent>,
    history: Vec<HttpStreamEvent>,
}

impl HttpStreamBroker {
    pub fn new() -> Self {
        Self::default()
    }

    pub async fn create_stream(&self) -> String {
        let id = format!(
            "http-stream-{}-{}",
            Utc::now().timestamp_millis(),
            self.counter.fetch_add(1, Ordering::Relaxed)
        );
        let (sender, _) = broadcast::channel(256);
        self.streams.lock().await.insert(
            id.clone(),
            StreamState {
                sender,
                history: Vec::new(),
            },
        );
        id
    }

    pub async fn publish(&self, stream_id: &str, event: impl Into<String>, payload: Value) {
        let event = HttpStreamEvent {
            event: event.into(),
            payload,
        };
        let mut streams = self.streams.lock().await;
        if let Some(state) = streams.get_mut(stream_id) {
            state.history.push(event.clone());
            state.history.truncate(512);
            let _ = state.sender.send(event);
        }
    }

    pub async fn register_session(&self, session_id: &str, stream_id: &str) {
        self.session_streams
            .lock()
            .await
            .insert(session_id.to_string(), stream_id.to_string());
    }

    pub async fn activate_session_stream(&self, session_id: &str, stream_id: &str) {
        self.active_session_streams
            .lock()
            .await
            .insert(session_id.to_string(), stream_id.to_string());
    }

    pub async fn clear_active_session_stream(&self, session_id: &str, stream_id: &str) {
        let mut active = self.active_session_streams.lock().await;
        if active
            .get(session_id)
            .is_some_and(|current| current == stream_id)
        {
            active.remove(session_id);
        }
    }

    pub async fn stream_id_for_session(&self, session_id: &str) -> Option<String> {
        if let Some(stream_id) = self.active_session_streams.lock().await.get(session_id) {
            return Some(stream_id.clone());
        }
        self.session_streams.lock().await.get(session_id).cloned()
    }

    pub async fn subscribe(
        &self,
        stream_id: &str,
    ) -> Option<(Vec<HttpStreamEvent>, broadcast::Receiver<HttpStreamEvent>)> {
        let streams = self.streams.lock().await;
        let state = streams.get(stream_id)?;
        Some((state.history.clone(), state.sender.subscribe()))
    }
}

#[derive(Debug, Deserialize)]
pub struct HttpMessageRequest {
    pub text: String,
    #[serde(default)]
    pub conversation_id: Option<String>,
    #[serde(default = "default_http_user")]
    pub user: String,
    #[serde(default)]
    pub user_display_name: Option<String>,
    #[serde(default)]
    pub attachments: Vec<Attachment>,
    #[serde(default)]
    pub raw: Value,
}

#[derive(Debug, Serialize)]
pub struct HttpMessageResponse {
    pub session_id: String,
    pub stream_id: String,
    pub stream_url: String,
}

impl HttpGatewayState {
    pub async fn inject_message(&self, request: HttpMessageRequest) -> Result<HttpMessageResponse> {
        let stream_id = self.broker.create_stream().await;
        let conversation_id = request
            .conversation_id
            .clone()
            .unwrap_or_else(|| stream_id.clone());
        let session_id = SessionId::from(format!("http-{conversation_id}"));
        self.broker
            .register_session(session_id.as_str(), &stream_id)
            .await;
        let raw = json!({
            "http_stream_id": stream_id,
            "conversation_id": request.conversation_id,
            "raw": request.raw,
        });
        let inbound = InboundEvent {
            envelope_id: stream_id.clone(),
            session_id: session_id.clone(),
            user: request.user,
            user_display_name: request.user_display_name,
            text: request.text,
            attachments: request.attachments,
            raw,
            inbound_handle: MessageHandle {
                channel: "http".to_string(),
                ts: stream_id.clone(),
            },
            is_direct_message: true,
            is_directly_addressed: true,
            link_previews: Vec::new(),
        };
        self.inbound_sink.send(inbound).await?;
        Ok(HttpMessageResponse {
            session_id: session_id.as_str().to_string(),
            stream_id: stream_id.clone(),
            stream_url: format!("/gateway/http/streams/{stream_id}"),
        })
    }
}

pub async fn stream_response(
    broker: Arc<HttpStreamBroker>,
    stream_id: String,
) -> Option<Sse<impl futures::Stream<Item = Result<Event, Infallible>>>> {
    let (history, mut receiver) = broker.subscribe(&stream_id).await?;
    let output = stream! {
        for item in history {
            yield Ok(to_sse_event(item));
        }
        loop {
            match receiver.recv().await {
                Ok(item) => {
                    let is_done = item.event == "done";
                    yield Ok(to_sse_event(item));
                    if is_done {
                        break;
                    }
                }
                Err(broadcast::error::RecvError::Lagged(_)) => continue,
                Err(broadcast::error::RecvError::Closed) => break,
            }
        }
    };
    Some(
        Sse::new(output).keep_alive(
            axum::response::sse::KeepAlive::new()
                .interval(Duration::from_secs(15))
                .text("keep-alive"),
        ),
    )
}

fn to_sse_event(item: HttpStreamEvent) -> Event {
    Event::default()
        .event(item.event)
        .json_data(item.payload)
        .unwrap_or_else(|_| Event::default().event("error").data("serialize event"))
}

fn default_http_user() -> String {
    "http".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::sync::mpsc;

    #[tokio::test]
    async fn inject_message_creates_inbound_event_and_stream_mapping() {
        let (tx, mut rx) = mpsc::channel(1);
        let broker = Arc::new(HttpStreamBroker::new());
        let gateway = HttpGatewayState {
            inbound_sink: tx,
            broker: broker.clone(),
        };

        let response = gateway
            .inject_message(HttpMessageRequest {
                text: "hello".to_string(),
                conversation_id: Some("conversation-1".to_string()),
                user: "user-1".to_string(),
                user_display_name: Some("User One".to_string()),
                attachments: Vec::new(),
                raw: json!({"source": "test"}),
            })
            .await
            .expect("inject message");

        assert_eq!(response.session_id, "http-conversation-1");
        assert!(response.stream_url.ends_with(&response.stream_id));
        assert_eq!(
            broker.stream_id_for_session("http-conversation-1").await,
            Some(response.stream_id.clone())
        );

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(inbound.session_id.as_str(), "http-conversation-1");
        assert_eq!(inbound.user, "user-1");
        assert_eq!(inbound.text, "hello");
        assert_eq!(inbound.raw["http_stream_id"], response.stream_id);
        assert!(inbound.is_direct_message);
        assert!(inbound.is_directly_addressed);
    }

    #[tokio::test]
    async fn broker_replays_history_to_late_subscribers() {
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;
        broker
            .publish(&stream_id, "token", json!({"text": "hello"}))
            .await;

        let (history, mut receiver) = broker.subscribe(&stream_id).await.expect("stream exists");
        assert_eq!(history.len(), 1);
        assert_eq!(history[0].event, "token");
        assert_eq!(history[0].payload["text"], "hello");

        broker
            .publish(&stream_id, "final", json!({"text": "done"}))
            .await;
        let live = receiver.recv().await.expect("live event");
        assert_eq!(live.event, "final");
        assert_eq!(live.payload["text"], "done");
    }

    #[tokio::test]
    async fn active_session_stream_overrides_latest_registered_stream_temporarily() {
        let broker = HttpStreamBroker::new();
        broker
            .register_session("http-conversation", "stream-1")
            .await;
        broker
            .activate_session_stream("http-conversation", "stream-1")
            .await;
        broker
            .register_session("http-conversation", "stream-2")
            .await;

        assert_eq!(
            broker.stream_id_for_session("http-conversation").await,
            Some("stream-1".to_string())
        );

        broker
            .clear_active_session_stream("http-conversation", "stream-1")
            .await;
        assert_eq!(
            broker.stream_id_for_session("http-conversation").await,
            Some("stream-2".to_string())
        );
    }
}
