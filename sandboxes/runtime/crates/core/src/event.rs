use serde::{Deserialize, Serialize};

use crate::agent::AgentId;
use crate::conversation::ConversationId;

/// Unified event type covering every lifecycle event in the bridge runtime.
///
/// This is the single canonical event type that flows through all delivery
/// channels (DB, WebSocket, SSE, webhook HTTP, polling). Every channel
/// receives the exact same data.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum BridgeEventType {
    /// A new conversation was created
    ConversationCreated,
    /// A user message was received
    MessageReceived,
    /// The assistant started generating a response
    ResponseStarted,
    /// A streaming response chunk was generated
    ResponseChunk,
    /// The assistant completed its response
    ResponseCompleted,
    /// A tool call was initiated
    ToolCallStarted,
    /// A tool call completed
    ToolCallCompleted,
    /// The conversation was ended
    ConversationEnded,
    /// An error occurred during agent execution
    AgentError,
    /// The task/todo list was updated
    TodoUpdated,
    /// A turn completed (stream done signal)
    TurnCompleted,
    /// A tool call requires user approval before execution
    ToolApprovalRequired,
    /// A tool approval request was resolved (approved or denied)
    ToolApprovalResolved,
    /// A background task (bash or subagent) completed.
    BackgroundTaskCompleted,
    /// The model started emitting reasoning/thinking text.
    ReasoningStarted,
    /// A reasoning/thinking text chunk from the model.
    ReasoningDelta,
    /// The model finished emitting reasoning. Carries `full_reasoning`.
    ReasoningCompleted,
    /// A subagent was spawned (foreground or background)
    SubAgentStarted,
    /// A subagent completed execution
    SubAgentCompleted,
    /// The response stream is complete (terminal signal for SSE/WS)
    Done,
}

/// The single canonical event payload used across all delivery channels.
///
/// Every event emitted by the bridge runtime is a `BridgeEvent`. It carries
/// a globally unique `event_id`, a monotonically increasing `sequence_number`
/// assigned by the [`EventBus`], and the event-specific data as a JSON value.
///
/// Delivery-specific concerns (webhook URL, HMAC secret) are NOT part of the
/// event — they are resolved at delivery time by the webhook worker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BridgeEvent {
    /// Globally unique event identifier.
    pub event_id: String,
    /// Type of event.
    pub event_type: BridgeEventType,
    /// Agent that produced this event.
    pub agent_id: AgentId,
    /// Conversation associated with this event.
    pub conversation_id: ConversationId,
    /// When the event occurred (UTC).
    pub timestamp: chrono::DateTime<chrono::Utc>,
    /// Monotonically increasing sequence number assigned by the EventBus.
    /// Globally unique across all agents and conversations.
    pub sequence_number: u64,
    /// Event-specific data.
    pub data: serde_json::Value,
}

impl BridgeEvent {
    /// Create a new event. The `sequence_number` is left at 0 — it will be
    /// stamped by the EventBus before fan-out.
    pub fn new(
        event_type: BridgeEventType,
        agent_id: impl Into<AgentId>,
        conversation_id: impl Into<ConversationId>,
        data: serde_json::Value,
    ) -> Self {
        Self {
            event_id: uuid::Uuid::new_v4().to_string(),
            event_type,
            agent_id: agent_id.into(),
            conversation_id: conversation_id.into(),
            timestamp: chrono::Utc::now(),
            sequence_number: 0,
            data,
        }
    }
}
