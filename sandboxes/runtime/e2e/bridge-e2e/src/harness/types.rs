use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::Duration;

/// A single turn in a multi-turn conversation.
#[derive(Debug)]
pub struct ConversationTurn {
    pub conversation_id: String,
    pub response_text: String,
    pub sse_events: Vec<SseEvent>,
    pub duration: Duration,
}

/// Parsed SSE event from the bridge stream.
///
/// The bridge SSE `data:` line is a full `BridgeEvent` JSON. This struct
/// unwraps it: `event_type` comes from `event_type` (or the SSE `event:` line),
/// and `data` is the inner event-specific payload (`BridgeEvent.data`).
#[derive(Debug, Clone)]
pub struct SseEvent {
    pub event_type: String,
    pub data: serde_json::Value,
}

/// Extract the inner event-specific data from a BridgeEvent JSON.
/// If the value has a `"data"` field (i.e. it's a BridgeEvent), returns that inner field.
/// Otherwise returns the value as-is (for non-BridgeEvent payloads like "ping" or errors).
pub(super) fn unwrap_bridge_event(raw: &serde_json::Value) -> (Option<String>, serde_json::Value) {
    if let Some(obj) = raw.as_object() {
        if let Some(inner_data) = obj.get("data") {
            let event_type = obj
                .get("event_type")
                .and_then(|v| v.as_str())
                .map(|s| s.to_string());
            return (event_type, inner_data.clone());
        }
    }
    (None, raw.clone())
}

/// A tool call log entry from the mock Portal MCP server.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCallLogEntry {
    pub timestamp: String,
    pub tool_name: String,
    pub arguments: serde_json::Value,
    pub result: serde_json::Value,
}

/// Raw received webhook as stored by the mock control plane.
/// The `body` may be either a single event object or a batched array.
#[derive(Debug, Clone, Deserialize)]
pub(super) struct ReceivedWebhookRaw {
    pub timestamp: String,
    pub headers: HashMap<String, String>,
    pub body: serde_json::Value,
}

/// A received webhook entry from the mock control plane.
///
/// Provides typed access to the webhook payload fields so tests can assert
/// on event types, agent/conversation IDs, data, and HMAC headers without
/// manually navigating raw JSON.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookEntry {
    /// Server-assigned timestamp when the webhook was received.
    pub timestamp: String,
    /// HTTP headers from the webhook request.
    pub headers: HashMap<String, String>,
    /// Parsed JSON body of the webhook payload.
    pub body: serde_json::Value,
}

impl WebhookEntry {
    /// The webhook event type (e.g. "conversation_created", "response_started").
    pub fn event_type(&self) -> Option<&str> {
        self.body.get("event_type").and_then(|v| v.as_str())
    }

    /// The agent ID from the webhook payload.
    pub fn agent_id(&self) -> Option<&str> {
        self.body.get("agent_id").and_then(|v| v.as_str())
    }

    /// The conversation ID from the webhook payload.
    pub fn conversation_id(&self) -> Option<&str> {
        self.body.get("conversation_id").and_then(|v| v.as_str())
    }

    /// The event-specific data payload.
    pub fn data(&self) -> Option<&serde_json::Value> {
        self.body.get("data")
    }

    /// Whether the `X-Webhook-Signature` header is present.
    pub fn has_signature(&self) -> bool {
        self.headers.contains_key("x-webhook-signature")
    }

    /// Whether the `X-Webhook-Timestamp` header is present.
    pub fn has_timestamp_header(&self) -> bool {
        self.headers.contains_key("x-webhook-timestamp")
    }
}

/// A collected set of webhook entries with query/assertion helpers.
#[derive(Debug, Clone)]
pub struct WebhookLog {
    pub entries: Vec<WebhookEntry>,
}

impl WebhookLog {
    /// All distinct event types present in the log.
    pub fn event_types(&self) -> Vec<String> {
        self.entries
            .iter()
            .filter_map(|e| e.event_type().map(|s| s.to_string()))
            .collect()
    }

    /// All distinct event types, deduplicated.
    pub fn unique_event_types(&self) -> Vec<String> {
        let mut types = self.event_types();
        types.sort();
        types.dedup();
        types
    }

    /// Filter entries by event type.
    pub fn by_type(&self, event_type: &str) -> Vec<&WebhookEntry> {
        self.entries
            .iter()
            .filter(|e| e.event_type() == Some(event_type))
            .collect()
    }

    /// Whether any entry has the given event type.
    pub fn has_type(&self, event_type: &str) -> bool {
        self.entries
            .iter()
            .any(|e| e.event_type() == Some(event_type))
    }

    /// Filter entries by conversation ID.
    pub fn by_conversation(&self, conv_id: &str) -> Vec<&WebhookEntry> {
        self.entries
            .iter()
            .filter(|e| e.conversation_id() == Some(conv_id))
            .collect()
    }

    /// Assert that a given event type is present, with a descriptive panic message.
    pub fn assert_has_type(&self, event_type: &str) {
        assert!(
            self.has_type(event_type),
            "expected webhook event type '{}' not found in log; got: {:?}",
            event_type,
            self.unique_event_types()
        );
    }

    /// Assert that every entry has a valid `agent_id` field.
    pub fn assert_all_have_agent_id(&self) {
        for entry in &self.entries {
            assert!(
                entry.agent_id().is_some(),
                "webhook body should have agent_id: {:?}",
                entry.body
            );
        }
    }

    /// Assert that every entry has a valid `conversation_id` field.
    pub fn assert_all_have_conversation_id(&self) {
        for entry in &self.entries {
            assert!(
                entry.conversation_id().is_some(),
                "webhook body should have conversation_id: {:?}",
                entry.body
            );
        }
    }

    /// Assert that at least one entry has the `X-Webhook-Signature` header.
    pub fn assert_has_signature_header(&self) {
        assert!(
            self.entries.iter().any(|e| e.has_signature()),
            "at least one webhook should have x-webhook-signature header"
        );
    }

    /// Assert that at least one entry has the `X-Webhook-Timestamp` header.
    pub fn assert_has_timestamp_header(&self) {
        assert!(
            self.entries.iter().any(|e| e.has_timestamp_header()),
            "at least one webhook should have x-webhook-timestamp header"
        );
    }

    /// Number of entries in the log.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Whether the log is empty.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

/// Returns a UTC timestamp string for log entries.
pub(super) fn now_str() -> String {
    let d = std::time::SystemTime::now()
        .duration_since(std::time::SystemTime::UNIX_EPOCH)
        .unwrap_or_default();
    let total_secs = d.as_secs();
    let hours = (total_secs / 3600) % 24;
    let mins = (total_secs / 60) % 60;
    let secs = total_secs % 60;
    let millis = d.subsec_millis();
    format!("{:02}:{:02}:{:02}.{:03}", hours, mins, secs, millis)
}

/// Format an SSE event for human-readable logging.
pub(super) fn format_sse_for_log(event_type: &str, data: &serde_json::Value) -> String {
    match event_type {
        "tool_call_start" => {
            let name = data.get("name").and_then(|v| v.as_str()).unwrap_or("?");
            let id = data.get("id").and_then(|v| v.as_str()).unwrap_or("?");
            let args = data
                .get("arguments")
                .map(|a| serde_json::to_string_pretty(a).unwrap_or_else(|_| a.to_string()))
                .unwrap_or_default();
            format!("Tool: {} (id: {})\nArguments:\n{}", name, id, args)
        }
        "tool_call_result" => {
            let id = data.get("id").and_then(|v| v.as_str()).unwrap_or("?");
            let is_error = data
                .get("is_error")
                .and_then(|v| v.as_bool())
                .unwrap_or(false);
            let result_str = data.get("result").and_then(|v| v.as_str()).unwrap_or("");
            let formatted = serde_json::from_str::<serde_json::Value>(result_str)
                .map(|v| {
                    serde_json::to_string_pretty(&v).unwrap_or_else(|_| result_str.to_string())
                })
                .unwrap_or_else(|_| result_str.to_string());
            // Truncate very long results for readability
            let truncated = if formatted.len() > 4000 {
                let boundary = formatted.floor_char_boundary(4000);
                format!(
                    "{}...\n[truncated, {} total chars]",
                    &formatted[..boundary],
                    formatted.len()
                )
            } else {
                formatted
            };
            format!("id: {}, is_error: {}\nResult:\n{}", id, is_error, truncated)
        }
        "content_delta" => {
            let delta = data.get("delta").and_then(|v| v.as_str()).unwrap_or("");
            format!("\"{}\"", delta)
        }
        _ => serde_json::to_string_pretty(data).unwrap_or_else(|_| data.to_string()),
    }
}
