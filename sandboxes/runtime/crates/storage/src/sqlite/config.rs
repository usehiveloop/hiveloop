use std::sync::Arc;

use async_trait::async_trait;
use domain::AgentDefinition;
use sqlx::SqlitePool;

use crate::repos::{ConfigRepo, Result};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteConfigRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteConfigRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

#[async_trait]
impl ConfigRepo for SqliteConfigRepo {
    async fn load(&self) -> Result<Option<AgentDefinition>> {
        let row: Option<(String,)> =
            sqlx::query_as("SELECT definition_json FROM agent_config WHERE id = 1")
                .fetch_optional(self.pool.as_ref())
                .await?;
        match row {
            Some((definition_json,)) => Ok(Some(serde_json::from_str(&definition_json)?)),
            None => Ok(None),
        }
    }

    async fn upsert(&self, definition: &AgentDefinition) -> Result<()> {
        self.writer.upsert_config(definition).await
    }
}
