use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};

use crate::repos::{InboundDedupeRepo, Result};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteInboundDedupeRepo {
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteInboundDedupeRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl InboundDedupeRepo for SqliteInboundDedupeRepo {
    async fn check_and_record(&self, envelope_id: &str) -> Result<bool> {
        let now = Utc::now().to_rfc3339();
        self.writer
            .check_and_record_inbound(envelope_id.to_string(), now)
            .await
    }

    async fn cleanup_older_than(&self, before: DateTime<Utc>) -> Result<u64> {
        let cutoff = before.to_rfc3339();
        self.writer.cleanup_inbound_before(cutoff).await
    }
}
