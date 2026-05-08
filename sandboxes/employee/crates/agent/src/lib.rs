use async_trait::async_trait;
use domain::SessionId;
use futures::stream::BoxStream;
use serde::{Deserialize, Serialize};

pub mod cancel_cron_tool;
pub mod list_cron_jobs_tool;
pub mod model_helpers;
pub mod post_to_channel_tool;
pub mod runner;
pub mod schedule_cron_tool;
pub mod session_helpers;
pub mod status_update_tool;
pub mod tool_registry;
pub mod update_cron_tool;
mod streaming_fix;
pub use runner::AdkAgentRunner;

#[derive(Debug, Clone)]
pub struct TurnInput {
    pub text: String,
    pub images: Vec<ImageInput>,
    pub prior_history: Vec<HistoryEntry>,
}

#[derive(Debug, Clone)]
pub struct ImageInput {
    pub mime_type: String,
    pub data: Vec<u8>,
}

#[derive(Debug, Clone)]
pub struct HistoryEntry {
    pub role: HistoryRole,
    pub speaker_id: String,
    pub speaker_display_name: Option<String>,
    pub text: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HistoryRole {
    User,
    Assistant,
}

impl TurnInput {
    pub fn text(input: impl Into<String>) -> Self {
        Self {
            text: input.into(),
            images: Vec::new(),
            prior_history: Vec::new(),
        }
    }

    pub fn with_image(mut self, mime_type: impl Into<String>, data: Vec<u8>) -> Self {
        self.images.push(ImageInput {
            mime_type: mime_type.into(),
            data,
        });
        self
    }

    pub fn with_history(mut self, history: Vec<HistoryEntry>) -> Self {
        self.prior_history = history;
        self
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum AgentEvent {
    TokenChunk { text: String },
    ToolCall {
        id: String,
        tool: String,
        args: serde_json::Value,
    },
    ToolResult {
        id: String,
        result: serde_json::Value,
    },
    FinalMessage { text: String },
    Error { message: String },
}

#[derive(Debug, thiserror::Error)]
pub enum AgentError {
    #[error("model error: {0}")]
    Model(String),
    #[error("limit exceeded: {0}")]
    LimitExceeded(String),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

pub type Result<T> = std::result::Result<T, AgentError>;

#[async_trait]
pub trait AgentRunner: Send + Sync + 'static {
    async fn run_turn(
        &self,
        session_id: &SessionId,
        user_input: TurnInput,
    ) -> Result<BoxStream<'static, AgentEvent>>;
}
