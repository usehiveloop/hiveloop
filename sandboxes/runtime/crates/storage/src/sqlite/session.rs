use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{Session, SessionId, SessionStatus};
use sqlx::{Row, SqlitePool};

use crate::repos::{Result, SessionListFilter, SessionRepo, StorageError};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteSessionRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteSessionRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
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
        self.writer.create_session(session.clone()).await
    }

    async fn touch(&self, id: &SessionId, at: DateTime<Utc>) -> Result<()> {
        self.writer.touch_session(id.clone(), at).await
    }

    async fn set_status(&self, id: &SessionId, status: SessionStatus) -> Result<()> {
        self.writer.set_session_status(id.clone(), status).await
    }

    async fn list(&self, filter: SessionListFilter, limit: u32) -> Result<Vec<Session>> {
        let limit = limit.min(500);
        let cursor_text = filter
            .cursor
            .as_ref()
            .map(|c| c.last_activity_at.to_rfc3339());
        let cursor_id = filter
            .cursor
            .as_ref()
            .and_then(|c| c.id.as_deref())
            .filter(|id| !id.is_empty());
        let status_text = filter.status.map(status_to_str);
        let search_prefix = filter
            .search
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(escape_like_prefix);

        let mut query = String::from("SELECT * FROM sessions WHERE 1 = 1");
        match (cursor_text.as_ref(), cursor_id) {
            (Some(_), Some(_)) => {
                query.push_str(" AND (last_activity_at < ? OR (last_activity_at = ? AND id < ?))");
            }
            (Some(_), None) => {
                query.push_str(" AND last_activity_at < ?");
            }
            (None, _) => {}
        }
        if status_text.is_some() {
            query.push_str(" AND status = ?");
        }
        if filter.session_id.is_some() {
            query.push_str(" AND id = ?");
        }
        if filter.channel.is_some() {
            query.push_str(" AND channel = ?");
        }
        if filter.thread_ts.is_some() {
            query.push_str(" AND thread_ts = ?");
        }
        if filter.agent_session_id.is_some() {
            query.push_str(" AND agent_session_id = ?");
        }
        if search_prefix.is_some() {
            query.push_str(
                " AND (id LIKE ? ESCAPE '\\' OR agent_session_id LIKE ? ESCAPE '\\' \
                 OR channel LIKE ? ESCAPE '\\' OR thread_ts LIKE ? ESCAPE '\\')",
            );
        }
        query.push_str(" ORDER BY last_activity_at DESC, id DESC LIMIT ?");

        let mut prepared = sqlx::query(&query);
        match (cursor_text.as_ref(), cursor_id) {
            (Some(text), Some(id)) => {
                prepared = prepared.bind(text).bind(text).bind(id);
            }
            (Some(text), None) => {
                prepared = prepared.bind(text);
            }
            (None, _) => {}
        }
        if let Some(text) = status_text.as_ref() {
            prepared = prepared.bind(*text);
        }
        if let Some(value) = filter.session_id.as_ref() {
            prepared = prepared.bind(value);
        }
        if let Some(value) = filter.channel.as_ref() {
            prepared = prepared.bind(value);
        }
        if let Some(value) = filter.thread_ts.as_ref() {
            prepared = prepared.bind(value);
        }
        if let Some(value) = filter.agent_session_id.as_ref() {
            prepared = prepared.bind(value);
        }
        if let Some(prefix) = search_prefix.as_ref() {
            let pattern = format!("{prefix}%");
            prepared = prepared
                .bind(pattern.clone())
                .bind(pattern.clone())
                .bind(pattern.clone())
                .bind(pattern);
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

fn escape_like_prefix(value: &str) -> String {
    let mut out = String::with_capacity(value.len());
    for ch in value.chars() {
        match ch {
            '\\' | '%' | '_' => {
                out.push('\\');
                out.push(ch);
            }
            _ => out.push(ch),
        }
    }
    out
}
