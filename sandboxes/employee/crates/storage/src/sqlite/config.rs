use std::sync::Arc;

use async_trait::async_trait;
use chrono::Utc;
use domain::AgentDefinition;
use sqlx::SqlitePool;

use crate::repos::{ConfigRepo, Result, StorageError};

pub struct SqliteConfigRepo {
    pool: Arc<SqlitePool>,
}

impl SqliteConfigRepo {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self { pool }
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
        let definition_json = serde_json::to_string(definition)?;
        let updated_at = Utc::now().to_rfc3339();
        sqlx::query(
            "INSERT INTO agent_config (id, definition_json, updated_at) VALUES (1, ?, ?) \
             ON CONFLICT(id) DO UPDATE SET \
             definition_json = excluded.definition_json, \
             updated_at = excluded.updated_at",
        )
        .bind(&definition_json)
        .bind(&updated_at)
        .execute(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(())
    }
}
