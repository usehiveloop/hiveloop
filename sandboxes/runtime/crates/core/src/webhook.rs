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
