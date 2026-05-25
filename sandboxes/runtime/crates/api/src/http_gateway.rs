use std::collections::HashMap;
use std::convert::Infallible;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use anyhow::Result;
use async_stream::stream;
use axum::response::sse::{Event, Sse};
use chrono::{DateTime, Utc};
use domain::{Attachment, InboundEvent, MessageHandle, SessionId};
use observability::{
    parse_model_usage, EventTimings, ObservabilityEvent, ObservabilityEventType,
    ObservabilityRecorder, ToolUsage,
};
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
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HttpStreamEvent {
    pub event: String,
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub payload: Value,
}

#[derive(Default)]
pub struct HttpStreamBroker {
    streams: Mutex<HashMap<String, StreamState>>,
    session_streams: Mutex<HashMap<String, String>>,
    active_session_streams: Mutex<HashMap<String, String>>,
    observability: ObservabilityRecorder,
    counter: AtomicU64,
}

struct StreamState {
    sender: broadcast::Sender<HttpStreamEvent>,
    history: Vec<HttpStreamEvent>,
    context: Option<StreamObservabilityContext>,
}

#[derive(Debug, Clone)]
struct StreamObservabilityContext {
    trace_id: String,
    session_id: String,
    turn_id: String,
    run_id: String,
    started_at: DateTime<Utc>,
}

impl HttpStreamBroker {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn observability(&self) -> ObservabilityRecorder {
        self.observability.clone()
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
                context: None,
            },
        );
        id
    }

    pub async fn start_turn(
        &self,
        stream_id: &str,
        session_id: &str,
        text_len: usize,
    ) -> Option<(String, String)> {
        let now = Utc::now();
        let trace_id = format!("trace-{stream_id}");
        let turn_id = format!("turn-{stream_id}");
        let run_id = format!("run-{stream_id}");
        let context = StreamObservabilityContext {
            trace_id: trace_id.clone(),
            session_id: session_id.to_string(),
            turn_id: turn_id.clone(),
            run_id,
            started_at: now,
        };

        let mut streams = self.streams.lock().await;
        let state = streams.get_mut(stream_id)?;
        state.context = Some(context.clone());
        drop(streams);

        self.record_context_event(
            &context,
            ObservabilityEventType::RunStarted,
            json!({ "stream_id": stream_id }),
        );
        self.record_context_event(
            &context,
            ObservabilityEventType::TurnStarted,
            json!({
                "stream_id": stream_id,
                "input_text_len": text_len,
            }),
        );
        Some((trace_id, turn_id))
    }

    pub async fn publish(&self, stream_id: &str, event: impl Into<String>, payload: Value) {
        let event = HttpStreamEvent {
            event: event.into(),
            payload,
        };
        let mut context = None;
        let mut streams = self.streams.lock().await;
        if let Some(state) = streams.get_mut(stream_id) {
            state.history.push(event.clone());
            state.history.truncate(512);
            context = state.context.clone();
            let _ = state.sender.send(event.clone());
        }
        drop(streams);
        if let Some(context) = context.as_ref() {
            self.record_stream_event(context, &event.event, &event.payload);
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

    fn record_stream_event(
        &self,
        context: &StreamObservabilityContext,
        stream_event: &str,
        payload: &Value,
    ) {
        let event_type = match stream_event {
            "turn_started" => ObservabilityEventType::TurnStarted,
            "thinking" | "token" => ObservabilityEventType::AssistantDelta,
            "tool_call" => ObservabilityEventType::ToolCall,
            "tool_result" => ObservabilityEventType::ToolResult,
            "model_usage" => ObservabilityEventType::ModelUsage,
            "turn_completed" | "final" => ObservabilityEventType::TurnCompleted,
            "done" => ObservabilityEventType::RunCompleted,
            "model_request_failed" | "model_stream_failed" | "error" => {
                ObservabilityEventType::Error
            }
            _ => return,
        };

        let mut event = ObservabilityEvent::new(
            context.trace_id.clone(),
            context.session_id.clone(),
            context.turn_id.clone(),
            event_type,
        );
        event.run_id = Some(context.run_id.clone());
        event.payload = payload.clone();

        match stream_event {
            "tool_call" => {
                event.tool = Some(ToolUsage {
                    call_id: payload
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    tool_name: payload
                        .get("tool")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    status: "started".to_string(),
                });
            }
            "tool_result" => {
                event.tool = Some(ToolUsage {
                    call_id: payload
                        .get("id")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    tool_name: payload
                        .get("tool")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    status: "completed".to_string(),
                });
            }
            "model_usage" => {
                event.model = Some(parse_model_usage(payload));
            }
            "turn_completed"
            | "final"
            | "done"
            | "error"
            | "model_request_failed"
            | "model_stream_failed" => {
                let completed_at = event.occurred_at;
                event.timings = EventTimings {
                    started_at: Some(context.started_at),
                    completed_at: Some(completed_at),
                    duration_ms: (completed_at - context.started_at)
                        .num_milliseconds()
                        .try_into()
                        .ok(),
                    ..EventTimings::default()
                };
            }
            _ => {}
        }

        self.observability.append(event);
    }

    fn record_context_event(
        &self,
        context: &StreamObservabilityContext,
        event_type: ObservabilityEventType,
        payload: Value,
    ) {
        let mut event = ObservabilityEvent::new(
            context.trace_id.clone(),
            context.session_id.clone(),
            context.turn_id.clone(),
            event_type,
        );
        event.run_id = Some(context.run_id.clone());
        event.payload = payload;
        if matches!(
            event.event_type,
            ObservabilityEventType::RunStarted | ObservabilityEventType::TurnStarted
        ) {
            event.timings.started_at = Some(context.started_at);
        }
        self.observability.append(event);
    }
}

#[derive(Debug, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
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
    #[cfg_attr(feature = "openapi", schema(value_type = Object))]
    pub raw: Value,
}

#[derive(Debug, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HttpMessageResponse {
    pub session_id: String,
    pub stream_id: String,
    pub stream_url: String,
    pub trace_id: String,
    pub turn_id: String,
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
        let (trace_id, turn_id) = self
            .broker
            .start_turn(&stream_id, session_id.as_str(), request.text.len())
            .await
            .unwrap_or_else(|| (format!("trace-{stream_id}"), format!("turn-{stream_id}")));
        let source = request
            .raw
            .get("source")
            .and_then(Value::as_str)
            .filter(|value| *value == "gateway")
            .unwrap_or("http");
        let raw = json!({
            "source": source,
            "http_stream_id": stream_id,
            "trace_id": trace_id,
            "turn_id": turn_id,
            "conversation_id": conversation_id,
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
            trace_id,
            turn_id,
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
        assert!(response.trace_id.starts_with("trace-http-stream-"));
        assert!(response.turn_id.starts_with("turn-http-stream-"));
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
        assert_eq!(inbound.raw["trace_id"], response.trace_id);
        assert_eq!(inbound.raw["turn_id"], response.turn_id);
        assert!(inbound.is_direct_message);
        assert!(inbound.is_directly_addressed);

        let events = broker.observability().list_by_trace(&response.trace_id);
        assert_eq!(events.len(), 2);
        assert!(matches!(
            events[0].event_type,
            ObservabilityEventType::RunStarted
        ));
        assert!(matches!(
            events[1].event_type,
            ObservabilityEventType::TurnStarted
        ));
    }

    #[tokio::test]
    async fn inject_message_preserves_gateway_source_and_metadata() {
        let (tx, mut rx) = mpsc::channel(1);
        let gateway = HttpGatewayState {
            inbound_sink: tx,
            broker: Arc::new(HttpStreamBroker::new()),
        };

        let response = gateway
            .inject_message(HttpMessageRequest {
                text: "hello from slack".to_string(),
                conversation_id: Some("gateway-conversation".to_string()),
                user: "U123".to_string(),
                user_display_name: Some("Ada".to_string()),
                attachments: Vec::new(),
                raw: json!({
                    "source": "gateway",
                    "provider": "fake-slack",
                    "route_id": "route-1",
                    "thread_key": "fake-slack:T123:C123:100.000",
                    "channel_id": "C123",
                    "thread_id": "100.000"
                }),
            })
            .await
            .expect("inject gateway message");

        let inbound = rx.recv().await.expect("inbound event");
        assert_eq!(response.session_id, "http-gateway-conversation");
        assert_eq!(inbound.raw["source"], "gateway");
        assert_eq!(inbound.raw["conversation_id"], "gateway-conversation");
        assert_eq!(inbound.raw["raw"]["provider"], "fake-slack");
        assert_eq!(inbound.raw["raw"]["channel_id"], "C123");
        assert_eq!(inbound.raw["raw"]["thread_id"], "100.000");
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

    #[tokio::test]
    async fn broker_records_observability_for_stream_events() {
        let broker = HttpStreamBroker::new();
        let stream_id = broker.create_stream().await;
        let (trace_id, _) = broker
            .start_turn(&stream_id, "http-conversation", 5)
            .await
            .expect("turn context");

        broker
            .publish(
                &stream_id,
                "tool_call",
                json!({"id": "call-1", "tool": "lookup", "args": {"q": "hello"}}),
            )
            .await;
        broker
            .publish(
                &stream_id,
                "model_usage",
                json!({
                    "provider": "fake",
                    "model": "fake-model",
                    "usage": {
                        "prompt_tokens": 2,
                        "completion_tokens": 3,
                        "total_tokens": 5
                    }
                }),
            )
            .await;
        broker
            .publish(&stream_id, "final", json!({"text": "answer"}))
            .await;
        broker
            .publish(
                &stream_id,
                "done",
                json!({"session_id": "http-conversation"}),
            )
            .await;

        let summary = broker.observability().summarize_trace(&trace_id);
        assert!(summary.is_complete);
        assert_eq!(summary.tool_call_count, 1);
        assert_eq!(summary.model_usage.total_tokens, 5);
        assert_eq!(summary.final_text.as_deref(), Some("answer"));
    }
}
