use std::collections::HashSet;
use std::sync::{Arc, Mutex};

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

pub const OBSERVABILITY_SCHEMA_VERSION: u32 = 1;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ObservabilityEventType {
    RunStarted,
    RunCompleted,
    TurnStarted,
    AssistantDelta,
    ModelUsage,
    ToolCall,
    ToolResult,
    TurnCompleted,
    Error,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct EventTimings {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub queued_ms: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub started_at: Option<DateTime<Utc>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub completed_at: Option<DateTime<Utc>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub duration_ms: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model_ms: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_ms: Option<u64>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct ModelUsage {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default)]
    pub prompt_tokens: u64,
    #[serde(default)]
    pub completion_tokens: u64,
    #[serde(default)]
    pub total_tokens: u64,
    #[serde(default)]
    pub cached_tokens: u64,
    #[serde(default)]
    pub cache_write_tokens: u64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cost: Option<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ToolUsage {
    pub call_id: String,
    pub tool_name: String,
    pub status: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ObservabilityEvent {
    pub schema_version: u32,
    pub event_id: String,
    pub trace_id: String,
    pub session_id: String,
    pub turn_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub run_id: Option<String>,
    pub event_type: ObservabilityEventType,
    pub occurred_at: DateTime<Utc>,
    #[serde(default)]
    pub timings: EventTimings,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool: Option<ToolUsage>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<ModelUsage>,
    #[serde(default)]
    pub payload: Value,
}

impl ObservabilityEvent {
    pub fn new(
        trace_id: impl Into<String>,
        session_id: impl Into<String>,
        turn_id: impl Into<String>,
        event_type: ObservabilityEventType,
    ) -> Self {
        Self {
            schema_version: OBSERVABILITY_SCHEMA_VERSION,
            event_id: String::new(),
            trace_id: trace_id.into(),
            session_id: session_id.into(),
            turn_id: turn_id.into(),
            run_id: None,
            event_type,
            occurred_at: Utc::now(),
            timings: EventTimings::default(),
            tool: None,
            model: None,
            payload: Value::Object(Default::default()),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct TraceSummary {
    pub trace_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub session_id: Option<String>,
    pub event_count: usize,
    pub turn_count: usize,
    pub tool_call_count: usize,
    pub error_count: usize,
    pub is_complete: bool,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub final_text: Option<String>,
    pub model_usage: ModelUsage,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub duration_ms: Option<u64>,
}

#[derive(Debug, Default)]
struct ObservabilityInner {
    next_event_id: u64,
    events: Vec<ObservabilityEvent>,
}

#[derive(Clone, Debug, Default)]
pub struct ObservabilityRecorder {
    inner: Arc<Mutex<ObservabilityInner>>,
}

impl ObservabilityRecorder {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn append(&self, mut event: ObservabilityEvent) -> ObservabilityEvent {
        let mut inner = self.inner.lock().expect("observability recorder poisoned");
        inner.next_event_id += 1;
        if event.event_id.is_empty() {
            event.event_id = format!("evt-{:020}", inner.next_event_id);
        }
        inner.events.push(event.clone());
        event
    }

    pub fn list_by_trace(&self, trace_id: &str) -> Vec<ObservabilityEvent> {
        self.inner
            .lock()
            .expect("observability recorder poisoned")
            .events
            .iter()
            .filter(|event| event.trace_id == trace_id)
            .cloned()
            .collect()
    }

    pub fn list_by_session(&self, session_id: &str) -> Vec<ObservabilityEvent> {
        self.inner
            .lock()
            .expect("observability recorder poisoned")
            .events
            .iter()
            .filter(|event| event.session_id == session_id)
            .cloned()
            .collect()
    }

    pub fn summarize_trace(&self, trace_id: &str) -> TraceSummary {
        summarize_trace(trace_id.to_string(), self.list_by_trace(trace_id))
    }
}

pub fn summarize_trace(trace_id: String, events: Vec<ObservabilityEvent>) -> TraceSummary {
    let mut turn_ids = HashSet::new();
    let mut tool_call_count = 0;
    let mut error_count = 0;
    let mut is_complete = false;
    let mut final_text = None;
    let mut usage = ModelUsage::default();
    let mut session_id = None;
    let mut first_at = None;
    let mut last_at = None;

    for event in &events {
        session_id.get_or_insert_with(|| event.session_id.clone());
        first_at = Some(
            first_at.map_or(event.occurred_at, |current: DateTime<Utc>| {
                current.min(event.occurred_at)
            }),
        );
        last_at = Some(last_at.map_or(event.occurred_at, |current: DateTime<Utc>| {
            current.max(event.occurred_at)
        }));
        turn_ids.insert(event.turn_id.clone());
        match event.event_type {
            ObservabilityEventType::ToolCall => tool_call_count += 1,
            ObservabilityEventType::Error => error_count += 1,
            ObservabilityEventType::RunCompleted => is_complete = true,
            ObservabilityEventType::TurnCompleted => {
                final_text = event
                    .payload
                    .get("text")
                    .and_then(Value::as_str)
                    .map(ToOwned::to_owned)
                    .or(final_text);
            }
            ObservabilityEventType::ModelUsage => {
                if let Some(model) = &event.model {
                    usage.prompt_tokens += model.prompt_tokens;
                    usage.completion_tokens += model.completion_tokens;
                    usage.total_tokens += model.total_tokens;
                    usage.cached_tokens += model.cached_tokens;
                    usage.cache_write_tokens += model.cache_write_tokens;
                    usage.provider = usage.provider.clone().or_else(|| model.provider.clone());
                    usage.model = usage.model.clone().or_else(|| model.model.clone());
                    usage.cost = match (usage.cost, model.cost) {
                        (Some(left), Some(right)) => Some(left + right),
                        (Some(left), None) => Some(left),
                        (None, Some(right)) => Some(right),
                        (None, None) => None,
                    };
                }
            }
            _ => {}
        }
    }

    let duration_ms = first_at
        .zip(last_at)
        .and_then(|(start, end)| (end - start).num_milliseconds().try_into().ok());

    TraceSummary {
        trace_id,
        session_id,
        event_count: events.len(),
        turn_count: turn_ids.len(),
        tool_call_count,
        error_count,
        is_complete,
        final_text,
        model_usage: usage,
        duration_ms,
    }
}

pub fn parse_model_usage(payload: &Value) -> ModelUsage {
    let usage = payload.get("usage").unwrap_or(payload);
    ModelUsage {
        provider: string_field(payload, "provider"),
        model: string_field(payload, "model"),
        prompt_tokens: u64_field(usage, "prompt_tokens"),
        completion_tokens: u64_field(usage, "completion_tokens"),
        total_tokens: u64_field(usage, "total_tokens"),
        cached_tokens: u64_field(usage, "cached_tokens"),
        cache_write_tokens: u64_field(usage, "cache_write_tokens"),
        cost: usage.get("cost").and_then(Value::as_f64),
    }
}

pub fn payload_with_text(text: impl Into<String>) -> Value {
    json!({ "text": text.into() })
}

fn string_field(value: &Value, key: &str) -> Option<String> {
    value
        .get(key)
        .and_then(Value::as_str)
        .map(ToOwned::to_owned)
}

fn u64_field(value: &Value, key: &str) -> u64 {
    value.get(key).and_then(Value::as_u64).unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn recorder_captures_stable_event_schema_and_summary() {
        let recorder = ObservabilityRecorder::new();
        recorder.append(ObservabilityEvent::new(
            "trace-1",
            "session-1",
            "turn-1",
            ObservabilityEventType::TurnStarted,
        ));
        let mut tool = ObservabilityEvent::new(
            "trace-1",
            "session-1",
            "turn-1",
            ObservabilityEventType::ToolCall,
        );
        tool.tool = Some(ToolUsage {
            call_id: "call-1".to_string(),
            tool_name: "lookup".to_string(),
            status: "started".to_string(),
        });
        recorder.append(tool);
        let mut usage = ObservabilityEvent::new(
            "trace-1",
            "session-1",
            "turn-1",
            ObservabilityEventType::ModelUsage,
        );
        usage.model = Some(ModelUsage {
            provider: Some("fake".to_string()),
            model: Some("fake-model".to_string()),
            prompt_tokens: 3,
            completion_tokens: 4,
            total_tokens: 7,
            cached_tokens: 0,
            cache_write_tokens: 0,
            cost: Some(0.0),
        });
        recorder.append(usage);
        let mut completed = ObservabilityEvent::new(
            "trace-1",
            "session-1",
            "turn-1",
            ObservabilityEventType::TurnCompleted,
        );
        completed.payload = payload_with_text("final answer");
        recorder.append(completed);
        recorder.append(ObservabilityEvent::new(
            "trace-1",
            "session-1",
            "turn-1",
            ObservabilityEventType::RunCompleted,
        ));

        let events = recorder.list_by_trace("trace-1");
        assert_eq!(events[0].schema_version, OBSERVABILITY_SCHEMA_VERSION);
        assert_eq!(events[0].event_id, "evt-00000000000000000001");

        let summary = recorder.summarize_trace("trace-1");
        assert!(summary.is_complete);
        assert_eq!(summary.turn_count, 1);
        assert_eq!(summary.tool_call_count, 1);
        assert_eq!(summary.model_usage.total_tokens, 7);
        assert_eq!(summary.final_text.as_deref(), Some("final answer"));
    }
}
