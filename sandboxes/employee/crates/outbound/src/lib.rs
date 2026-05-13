mod builder;
mod database;
mod dispatcher;
mod emitter;
mod registry;
mod webhook;

use async_trait::async_trait;
use domain::OutboundEvent;

pub use builder::{build_registry, build_registry_with_write_notifier};
pub use database::DatabaseChannel;
pub use dispatcher::OutboundDispatcher;
pub use emitter::OutboundEmitter;
pub use registry::OutboundRegistry;
pub use webhook::WebhookChannel;

#[derive(Debug, thiserror::Error)]
pub enum OutboundError {
    #[error("delivery failed: {0}")]
    Delivery(String),
    #[error(transparent)]
    Storage(#[from] storage::StorageError),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

pub type Result<T> = std::result::Result<T, OutboundError>;

#[async_trait]
pub trait OutboundChannel: Send + Sync + 'static {
    fn name(&self) -> &str;
    fn kind(&self) -> &'static str;
    fn accepts(&self, event_type: &str) -> bool;
    async fn deliver(&self, event: &OutboundEvent) -> Result<()>;
}
