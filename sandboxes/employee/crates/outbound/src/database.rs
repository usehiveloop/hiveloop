use std::sync::Arc;

use async_trait::async_trait;
use chrono::Utc;
use domain::{OutboundEvent, DATABASE_CHANNEL_NAME};
use sqlx::SqlitePool;

use crate::{OutboundChannel, OutboundError, Result};

pub struct DatabaseChannel {
    pool: Arc<SqlitePool>,
}

impl DatabaseChannel {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self { pool }
    }
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
        let recorded_at = Utc::now().to_rfc3339();
        sqlx::query(
            "INSERT INTO events_log (event_type, payload_json, occurred_at, recorded_at) \
             VALUES (?, ?, ?, ?)",
        )
        .bind(&event.event_type)
        .bind(&payload)
        .bind(&timestamp)
        .bind(&recorded_at)
        .execute(self.pool.as_ref())
        .await
        .map_err(|e| OutboundError::Delivery(format!("events_log insert: {e}")))?;
        Ok(())
    }
}
