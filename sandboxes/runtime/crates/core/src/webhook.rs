use serde::{Deserialize, Serialize};

use crate::agent::AgentId;
use crate::conversation::ConversationId;

/// Types of webhook events delivered to the control plane.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum WebhookEventType {
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
    /// The conversation history was compacted (summarized)
    ConversationCompacted,
}

/// Payload for a webhook delivery.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookPayload {
    /// Type of event that triggered this webhook
    pub event_type: WebhookEventType,
    /// Agent that triggered the event
    pub agent_id: AgentId,
    /// Conversation associated with the event
    pub conversation_id: ConversationId,
    /// Timestamp of the event
    pub timestamp: chrono::DateTime<chrono::Utc>,
    /// Event-specific data
    pub data: serde_json::Value,
    /// URL to deliver the webhook to
    pub webhook_url: String,
    /// Secret for HMAC signing
    pub webhook_secret: String,
}

impl WebhookPayload {
    /// Create a new webhook payload with the current timestamp.
    pub fn new(
        event_type: WebhookEventType,
        agent_id: impl Into<AgentId>,
        conversation_id: impl Into<ConversationId>,
        data: serde_json::Value,
        webhook_url: impl Into<String>,
        webhook_secret: impl Into<String>,
    ) -> Self {
        Self {
            event_type,
            agent_id: agent_id.into(),
            conversation_id: conversation_id.into(),
            timestamp: chrono::Utc::now(),
            data,
            webhook_url: webhook_url.into(),
            webhook_secret: webhook_secret.into(),
        }
    }
}
