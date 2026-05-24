use std::sync::Arc;

use domain::OutboundEvent;
use storage::OutboxRepo;
use tokio::sync::RwLock;
use tracing::warn;

use crate::{OutboundRegistry, StreamBatcher};

pub struct OutboundEmitter {
    outbox: Arc<dyn OutboxRepo>,
    registry: Arc<RwLock<OutboundRegistry>>,
    stream_batcher: Arc<RwLock<Option<Arc<StreamBatcher>>>>,
}

impl OutboundEmitter {
    pub fn new(outbox: Arc<dyn OutboxRepo>, registry: Arc<RwLock<OutboundRegistry>>) -> Self {
        Self {
            outbox,
            registry,
            stream_batcher: Arc::new(RwLock::new(None)),
        }
    }

    pub fn with_stream_batcher(
        mut self,
        stream_batcher: Arc<RwLock<Option<Arc<StreamBatcher>>>>,
    ) -> Self {
        self.stream_batcher = stream_batcher;
        self
    }

    pub async fn emit(&self, event: OutboundEvent) {
        if let Some(batcher) = self.stream_batcher.read().await.clone() {
            match batcher.emit(event.clone()).await {
                Ok(true) => return,
                Ok(false) => {}
                Err(error) => {
                    warn!(event_type = %event.event_type, %error, "stream batch enqueue failed")
                }
            }
            if let Err(error) = batcher.flush_before_event(&event).await {
                warn!(event_type = %event.event_type, %error, "stream batch flush before event failed");
            }
        }

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

    pub async fn flush_streams_for_session(&self, session_id: &str) {
        if let Some(batcher) = self.stream_batcher.read().await.clone() {
            if let Err(error) = batcher.flush_session(session_id).await {
                warn!(session_id, %error, "stream batch flush failed");
            }
        }
    }
}
