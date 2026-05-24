use std::collections::HashMap;
use std::io::Write;
use std::sync::Arc;
use std::time::Duration;

use chrono::{DateTime, Utc};
use domain::{OutboundChannelKind, OutboundChannelSpec, OutboundEvent};
use flate2::write::GzEncoder;
use flate2::Compression;
use reqwest::Client as HttpClient;
use serde_json::{json, Value};
use tokio::sync::Mutex;
use tracing::warn;

use crate::webhook::{compute_signature, filter_matches};
use crate::{OutboundError, Result};

const HTTP_TIMEOUT_SECONDS: u64 = 30;
const MAX_BATCH_EVENTS: usize = 1000;
const MAX_BATCH_BYTES: usize = 5 * 1024 * 1024;
const MAX_COALESCED_TEXT_BYTES: usize = 128 * 1024;
const MAX_COALESCED_DELTAS: u64 = 1500;

pub struct StreamBatcher {
    sinks: Vec<BatchWebhookSink>,
    state: Mutex<BatchState>,
    http: HttpClient,
}

struct BatchWebhookSink {
    name: String,
    url: String,
    secret: String,
    extra_headers: HashMap<String, String>,
    event_filter: Option<Vec<String>>,
}

#[derive(Default)]
struct BatchState {
    coalescing: HashMap<String, CoalescedStream>,
    pending: Vec<OutboundEvent>,
    pending_bytes: usize,
}

struct CoalescedStream {
    event_type: String,
    session_id: String,
    source: String,
    sequence_start: u64,
    sequence_end: u64,
    delta_count: u64,
    text: String,
    started_at: DateTime<Utc>,
    ended_at: DateTime<Utc>,
}

impl StreamBatcher {
    pub fn from_specs(specs: &[OutboundChannelSpec]) -> Result<Option<Arc<Self>>> {
        let mut sinks = Vec::new();
        for spec in specs {
            let OutboundChannelKind::Webhook {
                url,
                secret_env,
                extra_headers,
            } = &spec.kind;
            let secret = match std::env::var(secret_env) {
                Ok(secret) => secret,
                Err(_) => {
                    warn!(
                        name = %spec.name,
                        "skipping stream batch webhook because secret env var is not set"
                    );
                    continue;
                }
            };
            sinks.push(BatchWebhookSink {
                name: spec.name.clone(),
                url: batch_url(url),
                secret,
                extra_headers: extra_headers.clone(),
                event_filter: spec.event_filter.clone(),
            });
        }
        if sinks.is_empty() {
            return Ok(None);
        }
        let http = HttpClient::builder()
            .timeout(Duration::from_secs(HTTP_TIMEOUT_SECONDS))
            .build()
            .map_err(|e| OutboundError::Delivery(format!("http client: {e}")))?;
        Ok(Some(Arc::new(Self {
            sinks,
            state: Mutex::new(BatchState::default()),
            http,
        })))
    }

    pub async fn emit(&self, event: OutboundEvent) -> Result<bool> {
        if !is_stream_delta(&event.event_type) {
            return Ok(false);
        }
        if !self
            .sinks
            .iter()
            .any(|sink| sink.accepts(&event.event_type))
        {
            return Ok(false);
        }
        let mut state = self.state.lock().await;
        state.add_stream_delta(event);
        if state.pending.len() >= MAX_BATCH_EVENTS || state.pending_bytes >= MAX_BATCH_BYTES {
            self.flush_locked(&mut state).await?;
        }
        Ok(true)
    }

    pub async fn flush_before_event(&self, event: &OutboundEvent) -> Result<()> {
        if is_stream_delta(&event.event_type) {
            return Ok(());
        }
        if string_field(&event.payload, "session_id").is_none() {
            return Ok(());
        }
        let mut state = self.state.lock().await;
        state.finish_streams_before_event(event);
        if !state.pending.is_empty() {
            self.flush_locked(&mut state).await?;
        }
        Ok(())
    }

    pub async fn flush_session(&self, session_id: &str) -> Result<()> {
        let mut state = self.state.lock().await;
        state.finish_session(session_id);
        if !state.pending.is_empty() {
            self.flush_locked(&mut state).await?;
        }
        Ok(())
    }

    async fn flush_locked(&self, state: &mut BatchState) -> Result<()> {
        state.finish_all();
        if state.pending.is_empty() {
            return Ok(());
        }
        let events = state.pending.clone();
        for sink in &self.sinks {
            let accepted = events
                .iter()
                .filter(|event| sink.accepts(&event.event_type))
                .collect::<Vec<_>>();
            if accepted.is_empty() {
                continue;
            }
            let mut ndjson = Vec::new();
            for event in accepted {
                serde_json::to_writer(&mut ndjson, event)
                    .map_err(|e| OutboundError::Delivery(format!("serialize batch event: {e}")))?;
                ndjson.push(b'\n');
            }
            let body = gzip_bytes(&ndjson)?;
            let signature = compute_signature(&sink.secret, &body);
            let mut request = self
                .http
                .post(&sink.url)
                .header("X-Hivy-Signature", format!("sha256={signature}"))
                .header(reqwest::header::CONTENT_TYPE, "application/x-ndjson")
                .header(reqwest::header::CONTENT_ENCODING, "gzip");
            for (header_name, header_value) in &sink.extra_headers {
                request = request.header(header_name.as_str(), header_value.as_str());
            }
            let response =
                request.body(body).send().await.map_err(|e| {
                    OutboundError::Delivery(format!("send batch {}: {e}", sink.url))
                })?;
            let status = response.status();
            if !status.is_success() {
                let body = response.text().await.unwrap_or_default();
                warn!(channel = %sink.name, %status, body = %body, "webhook batch non-2xx");
                return Err(OutboundError::Delivery(format!(
                    "{} returned {status}",
                    sink.url
                )));
            }
        }
        state.pending.clear();
        state.pending_bytes = 0;
        Ok(())
    }
}

impl BatchWebhookSink {
    fn accepts(&self, event_type: &str) -> bool {
        match self.event_filter.as_ref() {
            None => true,
            Some(filters) if filters.is_empty() => true,
            Some(filters) => filters.iter().any(|f| filter_matches(f, event_type)),
        }
    }
}

impl BatchState {
    fn add_stream_delta(&mut self, event: OutboundEvent) {
        let Some(session_id) = string_field(&event.payload, "session_id") else {
            self.push_event(event);
            return;
        };
        let text = event
            .payload
            .get("agent_event")
            .and_then(|v| v.get("text"))
            .and_then(Value::as_str)
            .unwrap_or("");
        if text.is_empty() {
            self.push_event(event);
            return;
        }
        let sequence = event
            .payload
            .get("sequence")
            .and_then(Value::as_u64)
            .unwrap_or(0);
        let source = string_field(&event.payload, "source").unwrap_or_else(|| "manual".to_string());
        let key = format!("{}:{session_id}", event.event_type);
        self.finish_other_session_streams(&session_id, &key);
        if self
            .coalescing
            .get(&key)
            .is_some_and(|entry| sequence == 0 || sequence != entry.sequence_end + 1)
        {
            self.finish_key(&key);
        }
        let should_finish = {
            let entry = self
                .coalescing
                .entry(key.clone())
                .or_insert_with(|| CoalescedStream {
                    event_type: event.event_type.clone(),
                    session_id,
                    source,
                    sequence_start: sequence,
                    sequence_end: sequence,
                    delta_count: 0,
                    text: String::new(),
                    started_at: event.at,
                    ended_at: event.at,
                });
            entry.sequence_end = sequence;
            entry.delta_count += 1;
            entry.text.push_str(text);
            entry.ended_at = event.at;
            entry.delta_count >= MAX_COALESCED_DELTAS
                || entry.text.len() >= MAX_COALESCED_TEXT_BYTES
        };
        if should_finish {
            self.finish_key(&key);
        }
    }

    fn finish_streams_before_event(&mut self, event: &OutboundEvent) {
        if is_stream_delta(&event.event_type) {
            return;
        }
        let Some(session_id) = string_field(&event.payload, "session_id") else {
            return;
        };
        self.finish_session(&session_id);
    }

    fn finish_other_session_streams(&mut self, session_id: &str, keep_key: &str) {
        let suffix = format!(":{session_id}");
        let keys = self
            .coalescing
            .keys()
            .filter(|key| key.ends_with(&suffix) && key.as_str() != keep_key)
            .cloned()
            .collect::<Vec<_>>();
        for key in keys {
            self.finish_key(&key);
        }
    }

    fn finish_session(&mut self, session_id: &str) {
        let keys = self
            .coalescing
            .keys()
            .filter(|key| key.ends_with(&format!(":{session_id}")))
            .cloned()
            .collect::<Vec<_>>();
        for key in keys {
            self.finish_key(&key);
        }
    }

    fn finish_all(&mut self) {
        let keys = self.coalescing.keys().cloned().collect::<Vec<_>>();
        for key in keys {
            self.finish_key(&key);
        }
    }

    fn finish_key(&mut self, key: &str) {
        let Some(entry) = self.coalescing.remove(key) else {
            return;
        };
        self.push_event(entry.into_event());
    }

    fn push_event(&mut self, event: OutboundEvent) {
        self.pending_bytes += serde_json::to_vec(&event).map(|v| v.len() + 1).unwrap_or(0);
        self.pending.push(event);
    }
}

impl CoalescedStream {
    fn into_event(self) -> OutboundEvent {
        let kind = match self.event_type.as_str() {
            "agent.stream.thinking" => "thinking_chunk",
            "agent.stream.token" => "token_chunk",
            _ => "stream_chunk",
        };
        OutboundEvent {
            event_type: self.event_type,
            payload: json!({
                "session_id": self.session_id,
                "source": self.source,
                "coalesced": true,
                "delta_count": self.delta_count,
                "sequence_start": self.sequence_start,
                "sequence_end": self.sequence_end,
                "started_at": self.started_at,
                "ended_at": self.ended_at,
                "agent_event": {
                    "kind": kind,
                    "text": self.text,
                },
            }),
            at: self.ended_at,
        }
    }
}

fn gzip_bytes(input: &[u8]) -> Result<Vec<u8>> {
    let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
    encoder
        .write_all(input)
        .map_err(|e| OutboundError::Delivery(format!("gzip batch: {e}")))?;
    encoder
        .finish()
        .map_err(|e| OutboundError::Delivery(format!("finish gzip batch: {e}")))
}

fn batch_url(url: &str) -> String {
    format!("{}/batch", url.trim_end_matches('/'))
}

fn is_stream_delta(event_type: &str) -> bool {
    matches!(event_type, "agent.stream.token" | "agent.stream.thinking")
}

fn string_field(value: &Value, key: &str) -> Option<String> {
    value
        .get(key)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|v| !v.is_empty())
        .map(ToString::to_string)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;

    #[test]
    fn coalesces_many_token_deltas_into_one_business_event() {
        let mut state = BatchState::default();
        for sequence in 1..=1500 {
            state.add_stream_delta(OutboundEvent::new(
                "agent.stream.token",
                json!({
                    "session_id": "http-thread-1",
                    "source": "http",
                    "sequence": sequence,
                    "agent_event": {"text": "x"},
                }),
            ));
        }
        state.finish_all();
        assert_eq!(state.pending.len(), 1);
        let event = &state.pending[0];
        assert_eq!(event.event_type, "agent.stream.token");
        assert_eq!(event.payload["coalesced"], true);
        assert_eq!(event.payload["delta_count"], 1500);
        assert_eq!(event.payload["sequence_start"], 1);
        assert_eq!(event.payload["sequence_end"], 1500);
        assert_eq!(
            event.payload["agent_event"]["text"].as_str().unwrap().len(),
            1500
        );
    }

    #[test]
    fn flush_session_does_not_flush_other_sessions() {
        let mut state = BatchState::default();
        for session in ["a", "b"] {
            state.add_stream_delta(OutboundEvent::new(
                "agent.stream.thinking",
                json!({
                    "session_id": session,
                    "source": "http",
                    "sequence": 1,
                    "agent_event": {"text": session},
                }),
            ));
        }
        state.finish_session("a");
        assert_eq!(state.pending.len(), 1);
        assert_eq!(state.pending[0].payload["session_id"], "a");
        assert_eq!(state.coalescing.len(), 1);
        assert!(state
            .coalescing
            .keys()
            .collect::<HashSet<_>>()
            .iter()
            .any(|k| k.ends_with(":b")));
    }

    #[test]
    fn flushes_stream_span_before_non_stream_event() {
        let mut state = BatchState::default();
        state.add_stream_delta(OutboundEvent::new(
            "agent.stream.thinking",
            json!({
                "session_id": "http-thread-1",
                "source": "http",
                "sequence": 3,
                "agent_event": {"text": "thinking"},
            }),
        ));
        state.finish_streams_before_event(&OutboundEvent::new(
            "agent.tool.call",
            json!({
                "session_id": "http-thread-1",
                "source": "http",
                "sequence": 4,
                "agent_event": {"kind": "tool_call"},
            }),
        ));

        assert_eq!(state.pending.len(), 1);
        assert_eq!(state.pending[0].event_type, "agent.stream.thinking");
        assert_eq!(state.pending[0].payload["sequence_start"], 3);
        assert_eq!(state.pending[0].payload["sequence_end"], 3);
        assert!(state.coalescing.is_empty());
    }

    #[test]
    fn splits_stream_spans_on_kind_change_and_sequence_gap() {
        let mut state = BatchState::default();
        for (event_type, sequence, text) in [
            ("agent.stream.thinking", 3, "think-a"),
            ("agent.stream.thinking", 4, "think-b"),
            ("agent.stream.token", 5, "token-a"),
            ("agent.stream.token", 8, "token-b"),
        ] {
            state.add_stream_delta(OutboundEvent::new(
                event_type,
                json!({
                    "session_id": "http-thread-1",
                    "source": "http",
                    "sequence": sequence,
                    "agent_event": {"text": text},
                }),
            ));
        }
        state.finish_all();

        assert_eq!(state.pending.len(), 3);
        assert_eq!(state.pending[0].event_type, "agent.stream.thinking");
        assert_eq!(state.pending[0].payload["sequence_start"], 3);
        assert_eq!(state.pending[0].payload["sequence_end"], 4);
        assert_eq!(state.pending[1].event_type, "agent.stream.token");
        assert_eq!(state.pending[1].payload["sequence_start"], 5);
        assert_eq!(state.pending[1].payload["sequence_end"], 5);
        assert_eq!(state.pending[2].event_type, "agent.stream.token");
        assert_eq!(state.pending[2].payload["sequence_start"], 8);
        assert_eq!(state.pending[2].payload["sequence_end"], 8);
    }
}
