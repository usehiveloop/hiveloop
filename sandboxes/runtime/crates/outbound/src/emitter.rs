use std::sync::Arc;

use domain::OutboundEvent;
use storage::OutboxRepo;
use tokio::sync::RwLock;
use tracing::warn;

use crate::{DatabaseEventQueue, OutboundRegistry, StreamBatcher};

pub struct OutboundEmitter {
    outbox: Arc<dyn OutboxRepo>,
    registry: Arc<RwLock<OutboundRegistry>>,
    stream_batcher: Arc<RwLock<Option<Arc<StreamBatcher>>>>,
    database_queue: Option<Arc<DatabaseEventQueue>>,
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use domain::{OutboundChannelKind, OutboundChannelSpec, OutboundEvent};
    use serde_json::{json, Value};
    use storage::{init_sqlite_store, OutboxRepo, OutboxRow, SqliteStore};
    use tokio::sync::RwLock;

    use crate::{
        DatabaseEventQueue, OutboundChannel, OutboundEmitter, OutboundRegistry, StreamBatcher,
    };

    static DB_COUNTER: AtomicU64 = AtomicU64::new(0);

    #[derive(Default)]
    struct FakeOutbox {
        rows: Mutex<Vec<(String, String, Value)>>,
    }

    #[async_trait]
    impl OutboxRepo for FakeOutbox {
        async fn enqueue(
            &self,
            channel_name: &str,
            event_type: &str,
            payload: Value,
        ) -> storage::Result<i64> {
            let mut rows = self.rows.lock().expect("outbox lock");
            rows.push((channel_name.to_string(), event_type.to_string(), payload));
            Ok(rows.len() as i64)
        }

        async fn claim_due(&self, _limit: u32) -> storage::Result<Vec<OutboxRow>> {
            Ok(Vec::new())
        }

        async fn mark_delivered(&self, _id: i64) -> storage::Result<()> {
            Ok(())
        }

        async fn schedule_retry(
            &self,
            _id: i64,
            _attempts: i32,
            _next_retry_at: DateTime<Utc>,
        ) -> storage::Result<()> {
            Ok(())
        }

        async fn mark_failed(&self, _id: i64) -> storage::Result<()> {
            Ok(())
        }
    }

    struct TestWebhookChannel;

    #[async_trait]
    impl OutboundChannel for TestWebhookChannel {
        fn name(&self) -> &str {
            "test-webhook"
        }

        fn kind(&self) -> &'static str {
            "webhook"
        }

        fn accepts(&self, _event_type: &str) -> bool {
            true
        }

        async fn deliver(&self, _event: &OutboundEvent) -> crate::Result<()> {
            Ok(())
        }
    }

    async fn setup_store() -> SqliteStore {
        let db_path = std::env::temp_dir().join(format!(
            "outbound-emitter-{}-{}.db",
            std::process::id(),
            DB_COUNTER.fetch_add(1, Ordering::Relaxed)
        ));
        init_sqlite_store(&db_path, None)
            .await
            .expect("init sqlite store")
    }

    async fn event_log_count(store: &SqliteStore) -> i64 {
        sqlx::query_scalar("SELECT COUNT(*) FROM events_log")
            .fetch_one(store.read_pool().as_ref())
            .await
            .expect("count events_log")
    }

    #[tokio::test]
    async fn emit_persists_database_events_without_using_webhook_outbox_for_database() {
        let store = setup_store().await;
        let database_queue = DatabaseEventQueue::new(store.writer());
        let outbox = Arc::new(FakeOutbox::default());
        let registry = OutboundRegistry::new().with_channel(Arc::new(TestWebhookChannel));
        let emitter = OutboundEmitter::new(outbox.clone(), Arc::new(RwLock::new(registry)))
            .with_database_queue(database_queue);

        emitter
            .emit(OutboundEvent::new(
                "agent.message.sent",
                json!({"session_id": "http-1", "text": "done"}),
            ))
            .await;
        emitter.flush_database().await;

        assert_eq!(event_log_count(&store).await, 1);
        let rows = outbox.rows.lock().expect("outbox lock");
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].0, "test-webhook");
    }

    #[tokio::test]
    async fn stream_batcher_early_return_does_not_skip_database_persistence() {
        let store = setup_store().await;
        let database_queue = DatabaseEventQueue::new(store.writer());
        let outbox = Arc::new(FakeOutbox::default());
        let registry = OutboundRegistry::new();
        let stream_batcher = StreamBatcher::from_specs(
            &[OutboundChannelSpec {
                name: "batched-webhook".to_string(),
                kind: OutboundChannelKind::Webhook {
                    url: "http://127.0.0.1:9/webhook".to_string(),
                    secret_env: "WEBHOOK_SECRET".to_string(),
                    extra_headers: HashMap::new(),
                },
                event_filter: Some(vec!["agent.stream.*".to_string()]),
            }],
            &HashMap::from([("WEBHOOK_SECRET".to_string(), "secret".to_string())]),
        )
        .expect("build stream batcher");
        let emitter = OutboundEmitter::new(outbox.clone(), Arc::new(RwLock::new(registry)))
            .with_database_queue(database_queue)
            .with_stream_batcher(Arc::new(RwLock::new(stream_batcher)));

        emitter
            .emit(OutboundEvent::new(
                "agent.stream.token",
                json!({
                    "session_id": "http-1",
                    "source": "http",
                    "sequence": 1,
                    "agent_event": {"text": "hello"}
                }),
            ))
            .await;
        emitter.flush_database().await;

        assert_eq!(event_log_count(&store).await, 1);
        assert!(outbox.rows.lock().expect("outbox lock").is_empty());
    }
}

impl OutboundEmitter {
    pub fn new(outbox: Arc<dyn OutboxRepo>, registry: Arc<RwLock<OutboundRegistry>>) -> Self {
        Self {
            outbox,
            registry,
            stream_batcher: Arc::new(RwLock::new(None)),
            database_queue: None,
        }
    }

    pub fn with_stream_batcher(
        mut self,
        stream_batcher: Arc<RwLock<Option<Arc<StreamBatcher>>>>,
    ) -> Self {
        self.stream_batcher = stream_batcher;
        self
    }

    pub fn with_database_queue(mut self, database_queue: Arc<DatabaseEventQueue>) -> Self {
        self.database_queue = Some(database_queue);
        self
    }

    pub async fn emit(&self, event: OutboundEvent) {
        if let Some(database_queue) = &self.database_queue {
            if let Err(error) = database_queue.enqueue(&event).await {
                warn!(event_type = %event.event_type, %error, "database event enqueue failed");
            }
        }

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
        self.flush_database().await;
    }

    pub async fn flush_database(&self) {
        if let Some(database_queue) = &self.database_queue {
            if let Err(error) = database_queue.flush().await {
                warn!(%error, "database event queue flush failed");
            }
        }
    }
}
