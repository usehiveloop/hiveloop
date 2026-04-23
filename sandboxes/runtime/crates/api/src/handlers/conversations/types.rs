use std::collections::HashMap;

use serde::{Deserialize, Serialize};

/// Response for creating a conversation.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CreateConversationResponse {
    /// The ID of the newly created conversation.
    pub conversation_id: String,
    /// The URL to stream events from this conversation.
    pub stream_url: String,
}

/// Response for sending a message.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SendMessageResponse {
    /// Status of the message acceptance.
    pub status: String,
}

/// Response for ending a conversation.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct EndConversationResponse {
    /// Status of the end operation.
    pub status: String,
}

/// Response for aborting a conversation turn.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AbortConversationResponse {
    /// Status of the abort operation.
    pub status: String,
}

/// Optional request body for creating a conversation with tool/MCP scoping.
#[derive(Deserialize, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CreateConversationRequest {
    /// When provided, only these tools are available in the conversation.
    /// Tool names must match the agent's registered tool names exactly.
    #[serde(default)]
    pub tool_names: Option<Vec<String>>,

    /// When provided, only tools from these MCP servers are available.
    /// Server names must match the agent's configured MCP server names.
    #[serde(default)]
    pub mcp_server_names: Option<Vec<String>>,

    /// When provided, overrides the agent's LLM API key for this conversation only.
    /// For full provider/model override, use the `provider` field instead.
    #[serde(default)]
    pub api_key: Option<String>,

    /// When provided, fully overrides the agent's LLM provider for this conversation.
    /// Allows switching model, provider type, API key, and base URL per conversation
    /// while keeping the same agent definition (tools, system prompt, skills, etc.).
    #[serde(default)]
    pub provider: Option<bridge_core::ProviderConfig>,

    /// Per-subagent API key overrides. Key = subagent name, Value = API key.
    /// Only named subagents are overridden; others keep their configured keys.
    #[serde(default)]
    pub subagent_api_keys: Option<HashMap<String, String>>,

    /// Additional MCP servers to load for this conversation only.
    /// Connected at conversation creation, torn down when the conversation ends
    /// (or is aborted, drained, or cancelled). Tool names produced by these
    /// servers must not collide with the agent's existing tool names.
    ///
    /// Stdio transport requires the runtime config flag
    /// `allow_stdio_mcp_from_api` to be enabled; otherwise only
    /// `streamable_http` is accepted.
    #[serde(default)]
    pub mcp_servers: Option<Vec<bridge_core::mcp::McpServerDefinition>>,
}

/// Request body for creating a message.
#[derive(Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SendMessageRequest {
    /// The text content to send. When [`full_message`](Self::full_message) is
    /// also supplied, `content` is the LLM-visible summary; omit it to let
    /// bridge auto-generate one from the first bytes of `full_message`.
    #[serde(default)]
    pub content: String,
    /// Optional system reminder to inject with this message.
    /// Will be wrapped in `<system-reminder>` tags and prepended to the user message.
    #[serde(default)]
    pub system_reminder: Option<String>,
    /// Optional full payload written to a per-conversation attachment file.
    /// When present, bridge writes it to disk, appends a `<system-reminder>`
    /// with the file path and tool-usage hint to `content`, and sends the
    /// composed text to the LLM. Callers use this to offload large inputs
    /// (stack traces, log dumps, file contents) without bloating the
    /// agent's context on every turn.
    ///
    /// Failures (disk full, permission denied) do NOT reject the message —
    /// bridge logs a warning and delivers `content` alone.
    #[serde(default)]
    pub full_message: Option<String>,
}
