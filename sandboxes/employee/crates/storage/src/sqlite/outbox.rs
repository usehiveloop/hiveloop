use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use sqlx::{Row, SqlitePool};

use crate::repos::{OutboxRepo, OutboxRow, Result};

const STATUS_PENDING: &str = "pending";
const STATUS_DELIVERED: &str = "delivered";
const STATUS_FAILED: &str = "failed";

pub struct SqliteOutboxRepo {
    pool: Arc<SqlitePool>,
}

impl SqliteOutboxRepo {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self { pool }
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
        let now = Utc::now().to_rfc3339();
        let id: i64 = sqlx::query_scalar(
            "INSERT INTO outbound_outbox \
             (channel_name, event_type, payload_json, attempts, next_retry_at, status, created_at) \
             VALUES (?, ?, ?, 0, ?, ?, ?) RETURNING id",
        )
        .bind(channel_name)
        .bind(event_type)
        .bind(&payload_json)
        .bind(&now)
        .bind(STATUS_PENDING)
        .bind(&now)
        .fetch_one(self.pool.as_ref())
        .await?;
        Ok(id)
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
        sqlx::query("UPDATE outbound_outbox SET status = ? WHERE id = ?")
            .bind(STATUS_DELIVERED)
            .bind(id)
            .execute(self.pool.as_ref())
            .await?;
        Ok(())
    }

    async fn schedule_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()> {
        sqlx::query(
            "UPDATE outbound_outbox SET attempts = ?, next_retry_at = ?, status = ? WHERE id = ?",
        )
        .bind(attempts)
        .bind(next_retry_at.to_rfc3339())
        .bind(STATUS_PENDING)
        .bind(id)
        .execute(self.pool.as_ref())
        .await?;
        Ok(())
    }

    async fn mark_failed(&self, id: i64) -> Result<()> {
        sqlx::query("UPDATE outbound_outbox SET status = ? WHERE id = ?")
            .bind(STATUS_FAILED)
            .bind(id)
            .execute(self.pool.as_ref())
            .await?;
        Ok(())
    }
}
