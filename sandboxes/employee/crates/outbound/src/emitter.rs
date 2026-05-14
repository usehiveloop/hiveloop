use std::sync::Arc;

use domain::OutboundEvent;
use storage::OutboxRepo;
use tokio::sync::RwLock;
use tracing::warn;

use crate::OutboundRegistry;

pub struct OutboundEmitter {
    outbox: Arc<dyn OutboxRepo>,
    registry: Arc<RwLock<OutboundRegistry>>,
}

impl OutboundEmitter {
    pub fn new(outbox: Arc<dyn OutboxRepo>, registry: Arc<RwLock<OutboundRegistry>>) -> Self {
        Self { outbox, registry }
    }

    pub async fn emit(&self, event: OutboundEvent) {
        let channels = {
            let registry = self.registry.read().await;
            registry
                .matching(&event.event_type)
                .map(|channel| channel.name().to_string())
                .collect::<Vec<_>>()
        };

        if channels.is_empty() {
            return;
        }

        for channel_name in channels {
            if let Err(error) = self
                .outbox
                .enqueue(&channel_name, &event.event_type, event.payload.clone())
                .await
            {
                warn!(channel = %channel_name, event_type = %event.event_type, %error, "outbox enqueue failed");
            }
        }
    }
}
