use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use sqlx::{Row, SqlitePool};

use crate::repos::{OutboxRepo, OutboxRow, Result};

use super::{SqliteStore, SqliteWriteGateway};

const STATUS_PENDING: &str = "pending";

pub struct SqliteOutboxRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteOutboxRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl OutboxRepo for SqliteOutboxRepo {
    async fn enqueue(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: serde_json::Value,
    ) -> Result<i64> {
        let payload_json = serde_json::to_string(&payload)?;
        self.writer
            .enqueue_outbox(
                channel_name.to_string(),
                event_type.to_string(),
                payload_json,
            )
            .await
    }

    async fn claim_due(&self, limit: u32) -> Result<Vec<OutboxRow>> {
        let limit = limit.min(256);
        let now = Utc::now().to_rfc3339();
        let rows = sqlx::query(
            "SELECT id, channel_name, event_type, payload_json, attempts \
             FROM outbound_outbox \
             WHERE status = ? AND next_retry_at <= ? \
             ORDER BY id ASC \
             LIMIT ?",
        )
        .bind(STATUS_PENDING)
        .bind(&now)
        .bind(limit as i64)
        .fetch_all(self.pool.as_ref())
        .await?;

        let mut outbox_rows = Vec::with_capacity(rows.len());
        for row in &rows {
            let payload_text: String = row.try_get("payload_json")?;
            outbox_rows.push(OutboxRow {
                id: row.try_get("id")?,
                channel_name: row.try_get("channel_name")?,
                event_type: row.try_get("event_type")?,
                payload: serde_json::from_str(&payload_text)?,
                attempts: row.try_get("attempts")?,
            });
        }
        Ok(outbox_rows)
    }

    async fn mark_delivered(&self, id: i64) -> Result<()> {
        self.writer.mark_outbox_delivered(id).await
    }

    async fn schedule_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()> {
        self.writer
            .schedule_outbox_retry(id, attempts, next_retry_at)
            .await
    }

    async fn mark_failed(&self, id: i64) -> Result<()> {
        self.writer.mark_outbox_failed(id).await
    }
}
