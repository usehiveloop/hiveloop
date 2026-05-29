use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{EventKind, SessionEvent, SessionId};
use sqlx::{Row, SqlitePool};

use crate::repos::{EventRepo, Result, SessionSearchResult, StorageError};

use super::{SqliteStore, SqliteWriteGateway};

pub struct SqliteEventRepo {
    pool: Arc<SqlitePool>,
    writer: Arc<SqliteWriteGateway>,
}

impl SqliteEventRepo {
    pub fn new(store: &SqliteStore) -> Self {
        Self {
            pool: store.read_pool(),
            writer: store.writer(),
        }
    }
}

fn kind_from_str(value: &str) -> Result<EventKind> {
    match value {
        "user_message" => Ok(EventKind::UserMessage),
        "assistant_message" => Ok(EventKind::AssistantMessage),
        "tool_call" => Ok(EventKind::ToolCall),
        "tool_result" => Ok(EventKind::ToolResult),
        "run_event" => Ok(EventKind::RunEvent),
        "specialist_event" => Ok(EventKind::SpecialistEvent),
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

fn build_fts_query(raw: &str) -> Option<String> {
    let mut terms = Vec::new();
    for token in raw.split_whitespace() {
        let cleaned: String = token
            .chars()
            .filter(|ch| ch.is_alphanumeric() || *ch == '_' || *ch == '-')
            .collect();
        let cleaned = cleaned.trim_matches('-').trim_matches('_').to_lowercase();
        if cleaned.len() >= 2 {
            terms.push(format!("\"{}\"*", cleaned.replace('"', "\"\"")));
        }
    }
    if terms.is_empty() {
        None
    } else {
        Some(terms.join(" AND "))
    }
}

#[async_trait]
impl EventRepo for SqliteEventRepo {
    async fn append(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
    ) -> Result<i64> {
        self.writer
            .append_event(session_id.clone(), kind, payload)
            .await
    }

    async fn append_idempotent(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
        idempotency_key: &str,
    ) -> Result<Option<i64>> {
        self.writer
            .append_event_idempotent(
                session_id.clone(),
                kind,
                payload,
                idempotency_key.to_string(),
            )
            .await
    }

    async fn list_recent(&self, session_id: &SessionId, limit: u32) -> Result<Vec<SessionEvent>> {
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

    async fn list_chronological(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<SessionEvent>> {
        let mut events = self.list_recent(session_id, limit).await?;
        events.reverse();
        Ok(events)
    }

    async fn search_sessions(
        &self,
        query: &str,
        session_id: Option<&SessionId>,
        limit: u32,
    ) -> Result<Vec<SessionSearchResult>> {
        let Some(fts_query) = build_fts_query(query) else {
            return Ok(Vec::new());
        };
        let limit = limit.clamp(1, 20);
        let rows = if let Some(session_id) = session_id {
            sqlx::query(
                "SELECT event_id, session_id, kind, content, created_at, bm25(session_event_search) AS score, \
                 snippet(session_event_search, 3, '[', ']', '...', 18) AS snippet \
                 FROM session_event_search \
                 WHERE session_event_search MATCH ? AND session_id = ? \
                 ORDER BY score ASC, created_at DESC \
                 LIMIT ?",
            )
            .bind(&fts_query)
            .bind(session_id.as_str())
            .bind(limit as i64)
            .fetch_all(self.pool.as_ref())
            .await?
        } else {
            sqlx::query(
                "SELECT event_id, session_id, kind, content, created_at, bm25(session_event_search) AS score, \
                 snippet(session_event_search, 3, '[', ']', '...', 18) AS snippet \
                 FROM session_event_search \
                 WHERE session_event_search MATCH ? \
                 ORDER BY score ASC, created_at DESC \
                 LIMIT ?",
            )
            .bind(&fts_query)
            .bind(limit as i64)
            .fetch_all(self.pool.as_ref())
            .await?
        };

        let mut out = Vec::with_capacity(rows.len());
        for row in &rows {
            let event_id: String = row.try_get("event_id")?;
            let session_id: String = row.try_get("session_id")?;
            let kind: String = row.try_get("kind")?;
            let content: String = row.try_get("content")?;
            let snippet: String = row.try_get("snippet")?;
            let created_at_raw: String = row.try_get("created_at")?;
            let score: f64 = row.try_get("score")?;
            out.push(SessionSearchResult {
                session_id,
                event_id,
                kind,
                content,
                snippet,
                created_at: parse_timestamp(&created_at_raw)?,
                score,
            });
        }
        Ok(out)
    }
}
