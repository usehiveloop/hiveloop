use std::collections::VecDeque;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use domain::{OutboundEvent, DATABASE_CHANNEL_NAME};
use storage::{EventsLogWrite, SqliteWriteGateway};
use tokio::sync::{Mutex, Notify};
use tracing::warn;

use crate::{OutboundChannel, OutboundError, Result};

pub const DATABASE_BATCH_MAX_EVENTS: usize = 250;
pub const DATABASE_BATCH_MAX_BYTES: usize = 1024 * 1024;
const DATABASE_QUEUE_MAX_EVENTS: usize = 20_000;
pub const DATABASE_BATCH_FLUSH_INTERVAL: Duration = Duration::from_millis(200);

pub struct DatabaseChannel {
    writer: Arc<SqliteWriteGateway>,
}

impl DatabaseChannel {
    pub fn new(writer: Arc<SqliteWriteGateway>) -> Self {
        Self { writer }
    }
}

pub struct DatabaseEventQueue {
    writer: Arc<SqliteWriteGateway>,
    state: Mutex<DatabaseQueueState>,
    notify: Notify,
}

#[derive(Default)]
struct DatabaseQueueState {
    pending: VecDeque<DatabaseQueuedEvent>,
    pending_bytes: usize,
    dropped_stream_events: u64,
}

#[derive(Clone)]
struct DatabaseQueuedEvent {
    event_type: String,
    payload_json: String,
    occurred_at: String,
    estimated_bytes: usize,
}

impl DatabaseEventQueue {
    pub fn new(writer: Arc<SqliteWriteGateway>) -> Arc<Self> {
        Arc::new(Self {
            writer,
            state: Mutex::new(DatabaseQueueState::default()),
            notify: Notify::new(),
        })
    }

    pub fn spawn(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(DATABASE_BATCH_FLUSH_INTERVAL);
            loop {
                tokio::select! {
                    _ = interval.tick() => {}
                    _ = self.notify.notified() => {}
                }
                if let Err(error) = self.flush_one_batch().await {
                    warn!(%error, "database event queue flush failed");
                }
            }
        })
    }

    pub async fn enqueue(&self, event: &OutboundEvent) -> Result<()> {
        let queued = DatabaseQueuedEvent::from_event(event)?;
        let should_flush = {
            let mut state = self.state.lock().await;
            state.push(queued)?;
            state.pending.len() >= DATABASE_BATCH_MAX_EVENTS
                || state.pending_bytes >= DATABASE_BATCH_MAX_BYTES
        };
        if should_flush {
            self.notify.notify_one();
        }
        Ok(())
    }

    pub async fn flush(&self) -> Result<()> {
        loop {
            let batch = self.drain_one_batch().await;
            if batch.is_empty() {
                return Ok(());
            }
            if let Err(error) = self.insert_batch(&batch).await {
                self.requeue_front(batch).await;
                return Err(error);
            }
        }
    }

    async fn flush_one_batch(&self) -> Result<()> {
        let batch = self.drain_one_batch().await;
        if batch.is_empty() {
            return Ok(());
        }
        if let Err(error) = self.insert_batch(&batch).await {
            self.requeue_front(batch).await;
            return Err(error);
        }
        Ok(())
    }

    async fn drain_one_batch(&self) -> Vec<DatabaseQueuedEvent> {
        let mut state = self.state.lock().await;
        let mut batch = Vec::new();
        let mut batch_bytes = 0usize;
        while batch.len() < DATABASE_BATCH_MAX_EVENTS {
            let Some(next) = state.pending.front() else {
                break;
            };
            if !batch.is_empty() && batch_bytes + next.estimated_bytes > DATABASE_BATCH_MAX_BYTES {
                break;
            }
            let next = state.pending.pop_front().expect("pending front exists");
            state.pending_bytes = state.pending_bytes.saturating_sub(next.estimated_bytes);
            batch_bytes += next.estimated_bytes;
            batch.push(next);
        }
        batch
    }

    async fn requeue_front(&self, mut batch: Vec<DatabaseQueuedEvent>) {
        let mut state = self.state.lock().await;
        while let Some(event) = batch.pop() {
            state.pending_bytes += event.estimated_bytes;
            state.pending.push_front(event);
        }
    }

    async fn insert_batch(&self, batch: &[DatabaseQueuedEvent]) -> Result<()> {
        if batch.is_empty() {
            return Ok(());
        }
        let events = batch
            .iter()
            .map(|event| EventsLogWrite {
                event_type: event.event_type.clone(),
                payload_json: event.payload_json.clone(),
                occurred_at: event.occurred_at.clone(),
            })
            .collect();
        self.writer
            .append_events_log_batch(events)
            .await
            .map_err(OutboundError::Storage)?;
        Ok(())
    }
}

impl DatabaseQueueState {
    fn push(&mut self, event: DatabaseQueuedEvent) -> Result<()> {
        if self.pending.len() >= DATABASE_QUEUE_MAX_EVENTS {
            if is_low_value_stream_event(&event.event_type) {
                self.dropped_stream_events += 1;
                warn!(
                    dropped_stream_events = self.dropped_stream_events,
                    "dropping stream database event because queue is full"
                );
                return Ok(());
            }
            if let Some(index) = self
                .pending
                .iter()
                .position(|pending| is_low_value_stream_event(&pending.event_type))
            {
                if let Some(removed) = self.pending.remove(index) {
                    self.pending_bytes = self.pending_bytes.saturating_sub(removed.estimated_bytes);
                    self.dropped_stream_events += 1;
                    warn!(
                        dropped_stream_events = self.dropped_stream_events,
                        "dropped queued stream database event to preserve high-value event"
                    );
                }
            } else {
                return Err(OutboundError::Delivery(
                    "database event queue is full".to_string(),
                ));
            }
        }
        self.pending_bytes += event.estimated_bytes;
        self.pending.push_back(event);
        Ok(())
    }
}

impl DatabaseQueuedEvent {
    fn from_event(event: &OutboundEvent) -> Result<Self> {
        let payload_json = serde_json::to_string(&event.payload)
            .map_err(|e| OutboundError::Delivery(format!("serialize payload: {e}")))?;
        let estimated_bytes = event.event_type.len() + payload_json.len();
        Ok(Self {
            event_type: event.event_type.clone(),
            payload_json,
            occurred_at: event.at.to_rfc3339(),
            estimated_bytes,
        })
    }
}

fn is_low_value_stream_event(event_type: &str) -> bool {
    matches!(event_type, "agent.stream.thinking" | "agent.stream.token")
}

#[async_trait]
impl OutboundChannel for DatabaseChannel {
    fn name(&self) -> &str {
        DATABASE_CHANNEL_NAME
    }

    fn kind(&self) -> &'static str {
        "database"
    }

    fn accepts(&self, _event_type: &str) -> bool {
        true
    }

    async fn deliver(&self, event: &OutboundEvent) -> Result<()> {
        let payload = serde_json::to_string(&event.payload)
            .map_err(|e| OutboundError::Delivery(format!("serialize payload: {e}")))?;
        let timestamp = event.at.to_rfc3339();
        self.writer
            .append_events_log_batch(vec![EventsLogWrite {
                event_type: event.event_type.clone(),
                payload_json: payload,
                occurred_at: timestamp,
            }])
            .await
            .map_err(OutboundError::Storage)?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicU64, Ordering};

    use domain::OutboundEvent;
    use serde_json::json;
    use storage::init_sqlite_store;

    use super::{DatabaseEventQueue, DATABASE_BATCH_MAX_EVENTS};

    static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

    async fn setup_store() -> storage::SqliteStore {
        let db_path = std::env::temp_dir().join(format!(
            "outbound-database-{}-{}.db",
            std::process::id(),
            DB_COUNTER.fetch_add(1, Ordering::Relaxed)
        ));
        init_sqlite_store(&db_path, None)
            .await
            .expect("init sqlite store")
    }

    #[tokio::test]
    async fn force_flush_persists_accumulated_database_events() {
        let store = setup_store().await;
        let queue = DatabaseEventQueue::new(store.writer());
        for index in 0..DATABASE_BATCH_MAX_EVENTS {
            queue
                .enqueue(&OutboundEvent::new(
                    "agent.stream.token",
                    json!({
                        "session_id": "http-1",
                        "sequence": index,
                        "agent_event": {"text": "x"}
                    }),
                ))
                .await
                .expect("enqueue event");
        }

        queue.flush().await.expect("flush queue");

        let count: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM events_log")
            .fetch_one(store.read_pool().as_ref())
            .await
            .expect("count events_log");
        assert_eq!(count, DATABASE_BATCH_MAX_EVENTS as i64);
    }
}
