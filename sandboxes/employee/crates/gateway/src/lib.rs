//! `ChannelGateway` trait — the contract every channel adapter (Slack, Discord,
//! Teams, WhatsApp) implements. The runtime owns one gateway per process.

use async_trait::async_trait;
use domain::{Attachment, HistoryMessage, InboundEvent, MessageHandle, Reply, SessionId};
use tokio::sync::mpsc;

pub mod slack;
pub use slack::SlackGateway;

#[derive(Debug, thiserror::Error)]
pub enum GatewayError {
    #[error("transport error: {0}")]
    Transport(String),
    #[error("rate limited; retry after {retry_after_ms}ms")]
    RateLimited { retry_after_ms: u64 },
    #[error("not found: {0}")]
    NotFound(String),
    #[error("unauthorized: {0}")]
    Unauthorized(String),
    #[error("unsupported: {0}")]
    Unsupported(&'static str),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

pub type Result<T> = std::result::Result<T, GatewayError>;

#[async_trait]
pub trait ChannelGateway: Send + Sync + 'static {
    fn platform(&self) -> &'static str;
    async fn run(&self, sink: mpsc::Sender<InboundEvent>) -> Result<()>;
    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle>;
    async fn post_to_channel(&self, channel: &str, body: Reply) -> Result<MessageHandle>;
    async fn edit(&self, handle: &MessageHandle, body: Reply) -> Result<()>;
    async fn typing(&self, session_id: &SessionId) -> Result<()>;
    async fn stop_typing(&self, session_id: &SessionId) -> Result<()>;
    async fn upload(
        &self,
        session_id: &SessionId,
        bytes: Vec<u8>,
        filename: &str,
        caption: Option<&str>,
    ) -> Result<MessageHandle>;
    async fn react(&self, handle: &MessageHandle, emoji: &str) -> Result<()>;
    async fn unreact(&self, handle: &MessageHandle, emoji: &str) -> Result<()>;
    async fn fetch_thread_history(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<HistoryMessage>>;
    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>>;
}
