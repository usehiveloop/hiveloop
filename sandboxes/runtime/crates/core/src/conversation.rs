use serde::{Deserialize, Serialize};

/// Type alias for conversation identifiers.
pub type ConversationId = String;

/// A message in a conversation.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Message {
    /// Role of the message sender
    pub role: Role,
    /// Content blocks of the message
    pub content: Vec<ContentBlock>,
    /// Timestamp when the message was created
    pub timestamp: chrono::DateTime<chrono::Utc>,
}

/// Role of a message sender in a conversation.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Role {
    /// User message
    User,
    /// Assistant response
    Assistant,
    /// System message
    System,
    /// Tool result message
    Tool,
}

/// A content block within a message.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ContentBlock {
    /// Plain text content
    Text {
        /// The text content
        text: String,
    },
    /// A tool call request
    ToolCall(ToolCall),
    /// A tool execution result
    ToolResult(ToolResult),
    /// An image attachment
    Image {
        /// MIME type of the image
        media_type: String,
        /// Base64-encoded image data
        data: String,
    },
}

/// A request to call a tool.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ToolCall {
    /// Unique identifier for this tool call
    pub id: String,
    /// Name of the tool to call
    pub name: String,
    /// Arguments to pass to the tool
    pub arguments: serde_json::Value,
}

/// The result of a tool execution.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ToolResult {
    /// ID of the tool call this result corresponds to
    pub tool_call_id: String,
    /// The result content
    pub content: String,
    /// Whether the tool execution resulted in an error
    #[serde(default)]
    pub is_error: bool,
}

/// A conversation with its full message history, as returned by the control plane.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConversationRecord {
    /// Unique conversation identifier.
    pub id: ConversationId,
    /// Agent that owns this conversation.
    pub agent_id: String,
    /// Optional human-readable title.
    pub title: Option<String>,
    /// When the conversation was created.
    pub created_at: chrono::DateTime<chrono::Utc>,
    /// When the conversation was last updated.
    pub updated_at: chrono::DateTime<chrono::Utc>,
    /// Full message history.
    pub messages: Vec<Message>,
}

/// Paginated response from the control plane's conversations endpoint.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaginatedConversations {
    /// Conversations in this page.
    pub conversations: Vec<ConversationRecord>,
    /// Cursor for the next page, or `None` if this is the last page.
    pub next_cursor: Option<String>,
}
