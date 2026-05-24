//! `ChannelGateway` trait — the HTTP-only egress contract used by the employee runtime.

use async_trait::async_trait;
use domain::{Attachment, MessageHandle, Reply, SessionId};

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
pub trait GatewayEgress: Send + Sync + 'static {
    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle>;
    async fn edit(&self, handle: &MessageHandle, body: Reply) -> Result<()>;
}

#[async_trait]
pub trait FileCapability: Send + Sync + 'static {
    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>>;
}

#[async_trait]
pub trait ChannelGateway: Send + Sync + 'static {
    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle>;
    async fn edit(&self, handle: &MessageHandle, body: Reply) -> Result<()>;
    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>>;
}

#[async_trait]
impl<T> GatewayEgress for T
where
    T: ChannelGateway + ?Sized,
{
    async fn reply(&self, session_id: &SessionId, body: Reply) -> Result<MessageHandle> {
        ChannelGateway::reply(self, session_id, body).await
    }

    async fn edit(&self, handle: &MessageHandle, body: Reply) -> Result<()> {
        ChannelGateway::edit(self, handle, body).await
    }
}

#[async_trait]
impl<T> FileCapability for T
where
    T: ChannelGateway + ?Sized,
{
    async fn download_attachment(&self, attachment: &Attachment) -> Result<Vec<u8>> {
        ChannelGateway::download_attachment(self, attachment).await
    }
}
