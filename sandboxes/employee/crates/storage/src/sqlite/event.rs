use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{EventKind, SessionEvent, SessionId};
use sqlx::{Row, SqlitePool};

use crate::repos::{EventRepo, Result, StorageError};

pub struct SqliteEventRepo {
    pool: Arc<SqlitePool>,
}

impl SqliteEventRepo {
    pub fn new(pool: Arc<SqlitePool>) -> Self {
        Self { pool }
    }
}

fn kind_to_str(kind: EventKind) -> &'static str {
    match kind {
        EventKind::UserMessage => "user_message",
        EventKind::AssistantMessage => "assistant_message",
        EventKind::ToolCall => "tool_call",
        EventKind::ToolResult => "tool_result",
        EventKind::Error => "error",
    }
}

fn kind_from_str(value: &str) -> Result<EventKind> {
    match value {
        "user_message" => Ok(EventKind::UserMessage),
        "assistant_message" => Ok(EventKind::AssistantMessage),
        "tool_call" => Ok(EventKind::ToolCall),
        "tool_result" => Ok(EventKind::ToolResult),
        "error" => Ok(EventKind::Error),
        other => Err(StorageError::Other(anyhow::anyhow!(
            "unknown event kind: {other}"
        ))),
    }
}

fn parse_timestamp(raw: &str) -> Result<DateTime<Utc>> {
    DateTime::parse_from_rfc3339(raw)
        .map(|dt| dt.with_timezone(&Utc))
        .map_err(|e| StorageError::Other(anyhow::anyhow!("parse timestamp `{raw}`: {e}")))
}

#[async_trait]
impl EventRepo for SqliteEventRepo {
    async fn append(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
    ) -> Result<i64> {
        let mut tx = self.pool.begin().await?;
        let next_seq: Option<i64> =
            sqlx::query_scalar("SELECT MAX(seq) FROM session_events WHERE session_id = ?")
                .bind(session_id.as_str())
                .fetch_one(&mut *tx)
                .await?;
        let seq = next_seq.unwrap_or(0) + 1;
        let payload_json = serde_json::to_string(&payload)?;
        let created_at = Utc::now().to_rfc3339();
        let inserted_id: i64 = sqlx::query_scalar(
            "INSERT INTO session_events (session_id, seq, kind, payload_json, created_at) \
             VALUES (?, ?, ?, ?, ?) RETURNING id",
        )
        .bind(session_id.as_str())
        .bind(seq)
        .bind(kind_to_str(kind))
        .bind(&payload_json)
        .bind(&created_at)
        .fetch_one(&mut *tx)
        .await?;
        tx.commit().await?;
        Ok(inserted_id)
    }

    async fn list_recent(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<SessionEvent>> {
        let limit = limit.min(1000);
        let rows = sqlx::query(
            "SELECT id, session_id, seq, kind, payload_json, created_at \
             FROM session_events \
             WHERE session_id = ? \
             ORDER BY seq DESC \
             LIMIT ?",
        )
        .bind(session_id.as_str())
        .bind(limit as i64)
        .fetch_all(self.pool.as_ref())
        .await?;

        let mut events = Vec::with_capacity(rows.len());
        for row in &rows {
            let id: i64 = row.try_get("id")?;
            let session_id_text: String = row.try_get("session_id")?;
            let seq: i64 = row.try_get("seq")?;
            let kind_text: String = row.try_get("kind")?;
            let payload_text: String = row.try_get("payload_json")?;
            let created_at_text: String = row.try_get("created_at")?;
            events.push(SessionEvent {
                id,
                session_id: SessionId::from(session_id_text),
                seq,
                kind: kind_from_str(&kind_text)?,
                payload: serde_json::from_str(&payload_text)?,
                created_at: parse_timestamp(&created_at_text)?,
            });
        }
        Ok(events)
    }
}
