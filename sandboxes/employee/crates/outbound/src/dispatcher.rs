use std::sync::Arc;
use std::time::Duration;

use chrono::{Duration as ChronoDuration, Utc};
use domain::OutboundEvent;
use storage::{OutboxRepo, OutboxRow};
use tokio::sync::{oneshot, RwLock};
use tracing::{info, warn};

use crate::OutboundRegistry;

const MAX_RETRY_ATTEMPTS: i32 = 8;
const POLL_INTERVAL: Duration = Duration::from_millis(250);
const CLAIM_BATCH_SIZE: u32 = 32;

pub struct OutboundDispatcher {
    outbox: Arc<dyn OutboxRepo>,
    registry: Arc<RwLock<OutboundRegistry>>,
}

impl OutboundDispatcher {
    pub fn new(outbox: Arc<dyn OutboxRepo>, registry: Arc<RwLock<OutboundRegistry>>) -> Self {
        Self { outbox, registry }
    }

    pub fn spawn(self) -> (tokio::task::JoinHandle<()>, oneshot::Sender<()>) {
        let (cancel_signal, mut cancel_receiver) = oneshot::channel();
        let handle = tokio::spawn(async move {
            info!("outbound dispatcher started");
            loop {
                tokio::select! {
                    _ = &mut cancel_receiver => {
                        info!("outbound dispatcher shutting down");
                        break;
                    }
                    _ = tokio::time::sleep(POLL_INTERVAL) => {
                        if let Err(error) = self.drain_one_batch().await {
                            warn!(%error, "outbound dispatcher batch failed");
                        }
                    }
                }
            }
        });
        (handle, cancel_signal)
    }

    async fn drain_one_batch(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let due = self.outbox.claim_due(CLAIM_BATCH_SIZE).await?;
        if due.is_empty() {
            return Ok(());
        }
        for row in due {
            self.deliver_one(row).await;
        }
        Ok(())
    }

    async fn deliver_one(&self, row: OutboxRow) {
        let registry_snapshot = self.registry.read().await;
        let channel = registry_snapshot.find(&row.channel_name);
        drop(registry_snapshot);

        let Some(channel) = channel else {
            warn!(channel = %row.channel_name, id = row.id, "channel not registered; marking failed");
            let _ = self.outbox.mark_failed(row.id).await;
            return;
        };

        let event = OutboundEvent {
            event_type: row.event_type.clone(),
            payload: row.payload.clone(),
            at: Utc::now(),
        };

        match channel.deliver(&event).await {
            Ok(()) => {
                if let Err(error) = self.outbox.mark_delivered(row.id).await {
                    warn!(%error, id = row.id, "mark_delivered failed");
                }
            }
            Err(error) => {
                warn!(%error, id = row.id, channel = %row.channel_name, "delivery failed");
                let next_attempts = row.attempts + 1;
                if next_attempts >= MAX_RETRY_ATTEMPTS {
                    let _ = self.outbox.mark_failed(row.id).await;
                    return;
                }
                let backoff_seconds = 2_i64.pow(next_attempts.min(10) as u32) * 5;
                let next_retry_at = Utc::now() + ChronoDuration::seconds(backoff_seconds);
                let _ = self
                    .outbox
                    .schedule_retry(row.id, next_attempts, next_retry_at)
                    .await;
            }
        }
    }
}
