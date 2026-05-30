use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::str::FromStr;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum AgentMessageRole {
    System,
    User,
    Assistant,
    Tool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum MessagePart {
    Text { text: String },
    InlineData { mime_type: String, data: Vec<u8> },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentMessage {
    pub role: AgentMessageRole,
    pub parts: Vec<MessagePart>,
    pub tool_calls: Vec<ToolCall>,
    pub tool_call_id: Option<String>,
}

impl AgentMessage {
    pub fn system(text: impl Into<String>) -> Self {
        Self::text(AgentMessageRole::System, text)
    }

    pub fn user(text: impl Into<String>) -> Self {
        Self::text(AgentMessageRole::User, text)
    }

    pub fn assistant(text: impl Into<String>) -> Self {
        Self::text(AgentMessageRole::Assistant, text)
    }

    pub fn tool_result(tool_call_id: impl Into<String>, text: impl Into<String>) -> Self {
        Self {
            role: AgentMessageRole::Tool,
            parts: vec![MessagePart::Text { text: text.into() }],
            tool_calls: Vec::new(),
            tool_call_id: Some(tool_call_id.into()),
        }
    }

    pub fn assistant_tool_calls(tool_calls: Vec<ToolCall>) -> Self {
        Self {
            role: AgentMessageRole::Assistant,
            parts: Vec::new(),
            tool_calls,
            tool_call_id: None,
        }
    }

    pub fn push_part(&mut self, part: MessagePart) {
        self.parts.push(part);
    }

    fn text(role: AgentMessageRole, text: impl Into<String>) -> Self {
        Self {
            role,
            parts: vec![MessagePart::Text { text: text.into() }],
            tool_calls: Vec::new(),
            tool_call_id: None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub id: String,
    pub name: String,
    pub arguments: Value,
}

#[derive(Debug, Clone)]
pub struct ModelRequest {
    pub model: String,
    pub messages: Vec<AgentMessage>,
    pub tools: Vec<tools::ToolDefinition>,
    pub temperature: Option<f32>,
    pub max_output_tokens: Option<u32>,
    pub reasoning_effort: Option<String>,
    pub cache_policy: CacheControlPolicy,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CacheControlPolicy {
    Disabled,
    OpenRouterGeminiEphemeral,
}

#[derive(Debug, Clone, Default)]
pub struct ProviderUsage {
    pub prompt_tokens: i64,
    pub completion_tokens: i64,
    pub total_tokens: i64,
    pub cached_tokens: i64,
    pub cache_write_tokens: i64,
    pub reasoning_tokens: i64,
    pub cost: Option<f64>,
    pub raw: Option<Value>,
}

#[derive(Debug, Clone)]
pub enum ModelStreamEvent {
    TextDelta(String),
    ThinkingDelta(String),
    ToolCalls(Vec<ToolCall>),
    Usage(ProviderUsage),
    Done(FinishReason),
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum FinishReason {
    Stop,
    Length,
    ToolCalls,
    ContentFilter,
    Unknown(String),
}

impl FinishReason {
    pub fn is_complete(&self) -> bool {
        matches!(self, Self::Stop)
    }

    pub fn is_cut_off(&self) -> bool {
        matches!(self, Self::Length)
    }
}

impl FromStr for FinishReason {
    type Err = std::convert::Infallible;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        Ok(match s {
            "stop" | "end_turn" => Self::Stop,
            "length" | "max_tokens" => Self::Length,
            "tool_calls" | "tool_use" => Self::ToolCalls,
            "content_filter" | "safety" | "recitation" => Self::ContentFilter,
            other => Self::Unknown(other.to_string()),
        })
    }
}
