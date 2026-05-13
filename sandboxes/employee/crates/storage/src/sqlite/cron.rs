use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobSource, CronJobState};
use sqlx::SqlitePool;
use std::sync::Arc;

use crate::repos::{notify_write, CronJobRepo, Result, SharedWriteNotifier, StorageError};

pub struct SqliteCronJobRepo {
    pool: Arc<SqlitePool>,
    write_notifier: Option<SharedWriteNotifier>,
}

impl SqliteCronJobRepo {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self {
            pool,
            write_notifier: None,
        }
    }

    pub fn with_write_notifier(pool: Arc<SqlitePool>, write_notifier: SharedWriteNotifier) -> Self {
        Self {
            pool,
            write_notifier: Some(write_notifier),
        }
    }
}

const SELECT_COLS: &str = "id, description, channel, task_prompt, cron_expression, \
    interval_seconds, repeat_count, repeat_completed, state, source, next_run_at, last_run_at, \
    last_status, last_error, delegated_session_id, session_continuation_id, created_at, created_by_session";

#[async_trait]
impl CronJobRepo for SqliteCronJobRepo {
    async fn create(&self, job: &CronJob) -> Result<()> {
        sqlx::query(&format!(
            "INSERT INTO cron_jobs ({SELECT_COLS}) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
        ))
        .bind(&job.id).bind(&job.description).bind(&job.channel)
        .bind(&job.task_prompt).bind(&job.cron_expression)
        .bind(job.interval_seconds.map(|v| v as i64))
        .bind(job.repeat_count.map(|v| v as i32)).bind(job.repeat_completed as i32)
        .bind(state_str(job.state)).bind(source_str(job.source))
        .bind(job.next_run_at.to_rfc3339())
        .bind(job.last_run_at.map(|t| t.to_rfc3339()))
        .bind(&job.last_status).bind(&job.last_error)
        .bind(&job.delegated_session_id).bind(&job.session_continuation_id)
        .bind(job.created_at.to_rfc3339()).bind(&job.created_by_session)
        .execute(self.pool.as_ref()).await.map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn get(&self, id: &str) -> Result<Option<CronJob>> {
        let row: Option<CronJobRow> =
            sqlx::query_as(&format!("SELECT {SELECT_COLS} FROM cron_jobs WHERE id = ?"))
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

    async fn list_by_source(&self, source: CronJobSource) -> Result<Vec<CronJob>> {
        let rows: Vec<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs WHERE source = ? ORDER BY created_at DESC"
        ))
        .bind(source_str(source))
        .fetch_all(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(rows.into_iter().map(|r| r.into()).collect())
    }

    async fn list_due(&self) -> Result<Vec<CronJob>> {
        let now = Utc::now().to_rfc3339();
        let rows: Vec<CronJobRow> = sqlx::query_as(&format!(
            "SELECT {SELECT_COLS} FROM cron_jobs WHERE state = 'active' AND next_run_at <= ?"
        ))
        .bind(&now)
        .fetch_all(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        Ok(rows.into_iter().map(|r| r.into()).collect())
    }

    async fn update_prompt(&self, id: &str, task_prompt: String) -> Result<()> {
        sqlx::query("UPDATE cron_jobs SET task_prompt = ? WHERE id = ?")
            .bind(&task_prompt)
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn update_interval(&self, id: &str, interval_seconds: u64) -> Result<()> {
        sqlx::query("UPDATE cron_jobs SET interval_seconds = ? WHERE id = ?")
            .bind(interval_seconds as i64)
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn update_next_run(&self, id: &str, next_run_at: DateTime<Utc>) -> Result<()> {
        sqlx::query("UPDATE cron_jobs SET next_run_at = ? WHERE id = ?")
            .bind(next_run_at.to_rfc3339())
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn set_state(&self, id: &str, state: CronJobState) -> Result<()> {
        sqlx::query("UPDATE cron_jobs SET state = ? WHERE id = ?")
            .bind(state_str(state))
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn record_run(
        &self,
        id: &str,
        run_at: DateTime<Utc>,
        status: &str,
        error: Option<&str>,
    ) -> Result<()> {
        sqlx::query(
            "UPDATE cron_jobs SET last_run_at = ?, last_status = ?, last_error = ? WHERE id = ?",
        )
        .bind(run_at.to_rfc3339())
        .bind(status)
        .bind(error)
        .bind(id)
        .execute(self.pool.as_ref())
        .await
        .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn increment_repeat(&self, id: &str) -> Result<()> {
        sqlx::query("UPDATE cron_jobs SET repeat_completed = repeat_completed + 1 WHERE id = ?")
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn delete(&self, id: &str) -> Result<()> {
        sqlx::query("DELETE FROM cron_jobs WHERE id = ?")
            .bind(id)
            .execute(self.pool.as_ref())
            .await
            .map_err(StorageError::from)?;
        notify_write(&self.write_notifier);
        Ok(())
    }
}

fn state_str(state: CronJobState) -> &'static str {
    match state {
        CronJobState::Active => "active",
        CronJobState::Paused => "paused",
        CronJobState::Completed => "completed",
    }
}

fn source_str(source: CronJobSource) -> &'static str {
    match source {
        CronJobSource::Cron => "cron",
        CronJobSource::Delegate => "delegate",
    }
}

fn parse_source(s: &str) -> CronJobSource {
    match s {
        "delegate" => CronJobSource::Delegate,
        _ => CronJobSource::Cron,
    }
}

fn parse_state(s: &str) -> CronJobState {
    match s {
        "active" => CronJobState::Active,
        "paused" => CronJobState::Paused,
        "completed" => CronJobState::Completed,
        _ => CronJobState::Active,
    }
}

fn parse_opt_dt(s: &Option<String>) -> Option<DateTime<Utc>> {
    s.as_deref().and_then(|v| {
        DateTime::parse_from_rfc3339(v)
            .ok()
            .map(|d| d.with_timezone(&Utc))
    })
}

fn parse_dt(s: &str) -> DateTime<Utc> {
    DateTime::parse_from_rfc3339(s)
        .unwrap_or_default()
        .with_timezone(&Utc)
}

#[derive(Debug, sqlx::FromRow)]
struct CronJobRow {
    id: String,
    description: String,
    channel: String,
    task_prompt: String,
    cron_expression: Option<String>,
    interval_seconds: Option<i64>,
    repeat_count: Option<i32>,
    repeat_completed: i32,
    state: String,
    source: String,
    next_run_at: String,
    last_run_at: Option<String>,
    last_status: Option<String>,
    last_error: Option<String>,
    delegated_session_id: Option<String>,
    session_continuation_id: Option<String>,
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
            repeat_count: r.repeat_count.map(|v| v as u32),
            repeat_completed: r.repeat_completed as u32,
            state: parse_state(&r.state),
            source: parse_source(&r.source),
            next_run_at: parse_dt(&r.next_run_at),
            last_run_at: parse_opt_dt(&r.last_run_at),
            last_status: r.last_status,
            last_error: r.last_error,
            delegated_session_id: r.delegated_session_id,
            session_continuation_id: r.session_continuation_id,
            created_at: parse_dt(&r.created_at),
            created_by_session: r.created_by_session,
        }
    }
}
