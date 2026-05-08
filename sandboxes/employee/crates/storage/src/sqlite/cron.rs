use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::cron::CronJob;
use sqlx::SqlitePool;
use std::sync::Arc;

use crate::repos::{CronJobRepo, Result, StorageError};

pub struct SqliteCronJobRepo {
    pool: Arc<SqlitePool>,
}

impl SqliteCronJobRepo {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self { pool }
    }
}

const SELECT_COLS: &str =
    "id, description, channel, task_prompt, cron_expression, interval_seconds, next_run_at, created_at, created_by_session";

#[async_trait]
impl CronJobRepo for SqliteCronJobRepo {
    async fn create(&self, job: &CronJob) -> Result<()> {
        sqlx::query(
            "INSERT INTO cron_jobs (id, description, channel, task_prompt, cron_expression, interval_seconds, next_run_at, created_at, created_by_session) \
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
        )
        .bind(&job.id)
        .bind(&job.description)
        .bind(&job.channel)
        .bind(&job.task_prompt)
        .bind(&job.cron_expression)
        .bind(job.interval_seconds.map(|v| v as i64))
        .bind(job.next_run_at.to_rfc3339())
        .bind(job.created_at.to_rfc3339())
        .bind(&job.created_by_session)
        .execute(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(())
    }

    async fn get(&self, id: &str) -> Result<Option<CronJob>> {
        let row: Option<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs WHERE id = ?"
        ))
        .bind(id)
        .fetch_optional(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(row.map(|r| r.into()))
    }

    async fn list_all(&self) -> Result<Vec<CronJob>> {
        let rows: Vec<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs ORDER BY created_at DESC"
        ))
        .fetch_all(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(rows.into_iter().map(|r| r.into()).collect())
    }

    async fn list_due(&self) -> Result<Vec<CronJob>> {
        let now = Utc::now().to_rfc3339();
        let rows: Vec<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs WHERE next_run_at <= ?"
        ))
        .bind(&now)
        .fetch_all(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(rows.into_iter().map(|r| r.into()).collect())
    }

    async fn update(
        &self,
        id: &str,
        task_prompt: Option<String>,
        interval_seconds: Option<u64>,
    ) -> Result<()> {
        if let Some(prompt) = task_prompt {
            sqlx::query("UPDATE cron_jobs SET task_prompt = ? WHERE id = ?")
                .bind(&prompt)
                .bind(id)
                .execute(self.pool.as_ref())
                .await
                .map_err(StorageError::from)?;
        }
        if let Some(interval) = interval_seconds {
            sqlx::query("UPDATE cron_jobs SET interval_seconds = ? WHERE id = ?")
                .bind(interval as i64)
                .bind(id)
                .execute(self.pool.as_ref())
                .await
                .map_err(StorageError::from)?;
        }
        Ok(())
    }

    async fn update_next_run(&self, id: &str, next_run_at: DateTime<Utc>) -> Result<()> {
        sqlx::query("UPDATE cron_jobs SET next_run_at = ? WHERE id = ?")
            .bind(next_run_at.to_rfc3339())
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        Ok(())
    }

    async fn delete(&self, id: &str) -> Result<()> {
        sqlx::query("DELETE FROM cron_jobs WHERE id = ?")
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        Ok(())
    }
}

#[derive(Debug, sqlx::FromRow)]
struct CronJobRow {
    id: String,
    description: String,
    channel: String,
    task_prompt: String,
    cron_expression: Option<String>,
    interval_seconds: Option<i64>,
    next_run_at: String,
    created_at: String,
    created_by_session: String,
}

impl From<CronJobRow> for CronJob {
    fn from(r: CronJobRow) -> Self {
        Self {
            id: r.id,
            description: r.description,
            channel: r.channel,
            task_prompt: r.task_prompt,
            cron_expression: r.cron_expression,
            interval_seconds: r.interval_seconds.map(|v| v as u64),
            next_run_at: DateTime::parse_from_rfc3339(&r.next_run_at)
                .unwrap_or_default()
                .with_timezone(&Utc),
            created_at: DateTime::parse_from_rfc3339(&r.created_at)
                .unwrap_or_default()
                .with_timezone(&Utc),
            created_by_session: r.created_by_session,
        }
    }
}
