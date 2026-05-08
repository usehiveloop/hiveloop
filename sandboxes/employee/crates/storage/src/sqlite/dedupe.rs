use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use sqlx::SqlitePool;

use crate::repos::{InboundDedupeRepo, Result};

pub struct SqliteInboundDedupeRepo {
    pool: Arc<SqlitePool>,
}

impl SqliteInboundDedupeRepo {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self { pool }
    }
}

#[async_trait]
impl InboundDedupeRepo for SqliteInboundDedupeRepo {
    async fn check_and_record(&self, envelope_id: &str) -> Result<bool> {
        let now = Utc::now().to_rfc3339();
        let result = sqlx::query(
            "INSERT INTO inbound_dedupe (envelope_id, received_at) VALUES (?, ?) \
             ON CONFLICT(envelope_id) DO NOTHING",
        )
        .bind(envelope_id)
        .bind(&now)
        .execute(self.pool.as_ref())
        .await?;
        Ok(result.rows_affected() == 1)
    }

    async fn cleanup_older_than(&self, before: DateTime<Utc>) -> Result<u64> {
        let cutoff = before.to_rfc3339();
        let result = sqlx::query("DELETE FROM inbound_dedupe WHERE received_at < ?")
            .bind(&cutoff)
            .execute(self.pool.as_ref())
            .await?;
        Ok(result.rows_affected())
    }
}
