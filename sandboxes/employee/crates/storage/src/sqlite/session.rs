use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{Session, SessionId, SessionStatus};
use sqlx::{Row, SqlitePool};

use crate::repos::{notify_write, Result, SessionRepo, SharedWriteNotifier, StorageError};

pub struct SqliteSessionRepo {
    pool: Arc<SqlitePool>,
    write_notifier: Option<SharedWriteNotifier>,
}

impl SqliteSessionRepo {
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

fn status_to_str(status: SessionStatus) -> &'static str {
    match status {
        SessionStatus::Active => "active",
        SessionStatus::Completed => "completed",
        SessionStatus::Errored => "errored",
    }
}

fn status_from_str(value: &str) -> Result<SessionStatus> {
    match value {
        "active" => Ok(SessionStatus::Active),
        "completed" => Ok(SessionStatus::Completed),
        "errored" => Ok(SessionStatus::Errored),
        other => Err(StorageError::Other(anyhow::anyhow!(
            "unknown session status: {other}"
        ))),
    }
}

fn parse_timestamp(raw: &str) -> Result<DateTime<Utc>> {
    DateTime::parse_from_rfc3339(raw)
        .map(|dt| dt.with_timezone(&Utc))
        .map_err(|e| StorageError::Other(anyhow::anyhow!("parse timestamp `{raw}`: {e}")))
}

fn row_to_session(row: &sqlx::sqlite::SqliteRow) -> Result<Session> {
    let id: String = row.try_get("id")?;
    let channel: String = row.try_get("channel")?;
    let thread_ts: String = row.try_get("thread_ts")?;
    let agent_session_id: String = row.try_get("agent_session_id")?;
    let status_raw: String = row.try_get("status")?;
    let created_at_raw: String = row.try_get("created_at")?;
    let last_activity_at_raw: String = row.try_get("last_activity_at")?;
    Ok(Session {
        id: SessionId::from(id),
        channel,
        thread_ts,
        agent_session_id,
        status: status_from_str(&status_raw)?,
        created_at: parse_timestamp(&created_at_raw)?,
        last_activity_at: parse_timestamp(&last_activity_at_raw)?,
    })
}

#[async_trait]
impl SessionRepo for SqliteSessionRepo {
    async fn get(&self, id: &SessionId) -> Result<Option<Session>> {
        let maybe_row = sqlx::query("SELECT * FROM sessions WHERE id = ?")
            .bind(id.as_str())
            .fetch_optional(self.pool.as_ref())
            .await?;
        match maybe_row {
            Some(row) => Ok(Some(row_to_session(&row)?)),
            None => Ok(None),
        }
    }

    async fn create(&self, session: &Session) -> Result<()> {
        let result = sqlx::query(
            "INSERT INTO sessions (id, channel, thread_ts, agent_session_id, status, \
             created_at, last_activity_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
        )
        .bind(session.id.as_str())
        .bind(&session.channel)
        .bind(&session.thread_ts)
        .bind(&session.agent_session_id)
        .bind(status_to_str(session.status))
        .bind(session.created_at.to_rfc3339())
        .bind(session.last_activity_at.to_rfc3339())
        .execute(self.pool.as_ref())
        .await;
        match result {
            Ok(_) => {
                notify_write(&self.write_notifier);
                Ok(())
            }
            Err(sqlx::Error::Database(db_err)) if db_err.is_unique_violation() => {
                Err(StorageError::Conflict)
            }
            Err(e) => Err(StorageError::from(e)),
        }
    }

    async fn touch(&self, id: &SessionId, at: DateTime<Utc>) -> Result<()> {
        sqlx::query("UPDATE sessions SET last_activity_at = ? WHERE id = ?")
            .bind(at.to_rfc3339())
            .bind(id.as_str())
            .execute(self.pool.as_ref())
            .await?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn set_status(&self, id: &SessionId, status: SessionStatus) -> Result<()> {
        sqlx::query("UPDATE sessions SET status = ? WHERE id = ?")
            .bind(status_to_str(status))
            .bind(id.as_str())
            .execute(self.pool.as_ref())
            .await?;
        notify_write(&self.write_notifier);
        Ok(())
    }

    async fn list(
        &self,
        cursor: Option<DateTime<Utc>>,
        status: Option<SessionStatus>,
        limit: u32,
    ) -> Result<Vec<Session>> {
        let limit = limit.min(500);
        let cursor_text = cursor.map(|t| t.to_rfc3339());
        let status_text = status.map(status_to_str);

        let mut query = String::from("SELECT * FROM sessions WHERE 1 = 1");
        if cursor_text.is_some() {
            query.push_str(" AND last_activity_at < ?");
        }
        if status_text.is_some() {
            query.push_str(" AND status = ?");
        }
        query.push_str(" ORDER BY last_activity_at DESC LIMIT ?");

        let mut prepared = sqlx::query(&query);
        if let Some(text) = cursor_text.as_ref() {
            prepared = prepared.bind(text);
        }
        if let Some(text) = status_text.as_ref() {
            prepared = prepared.bind(*text);
        }
        prepared = prepared.bind(limit as i64);

        let rows = prepared.fetch_all(self.pool.as_ref()).await?;
        let mut sessions = Vec::with_capacity(rows.len());
        for row in &rows {
            sessions.push(row_to_session(row)?);
        }
        Ok(sessions)
    }
}
