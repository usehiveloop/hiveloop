use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::{EventKind, SessionEvent, SessionId};
use sqlx::{Row, SqlitePool};

use crate::repos::{
    notify_write, EventRepo, Result, SessionSearchResult, SharedWriteNotifier, StorageError,
};

pub struct SqliteEventRepo {
    pool: Arc<SqlitePool>,
    write_notifier: Option<SharedWriteNotifier>,
}

impl SqliteEventRepo {
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

fn kind_to_str(kind: EventKind) -> &'static str {
    match kind {
        EventKind::UserMessage => "user_message",
        EventKind::AssistantMessage => "assistant_message",
        EventKind::ToolCall => "tool_call",
        EventKind::ToolResult => "tool_result",
        EventKind::RunEvent => "run_event",
        EventKind::SpecialistEvent => "specialist_event",
        EventKind::Error => "error",
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

fn searchable_content(kind: EventKind, payload: &serde_json::Value) -> Option<String> {
    match kind {
        EventKind::UserMessage | EventKind::AssistantMessage | EventKind::ToolResult => {
            message_text(payload)
        }
        EventKind::ToolCall => tool_call_summary(payload),
        EventKind::RunEvent | EventKind::SpecialistEvent | EventKind::Error => None,
    }
}

fn message_text(payload: &serde_json::Value) -> Option<String> {
    let parts = payload.get("message")?.get("parts")?.as_array()?;
    let mut out = String::new();
    for part in parts {
        if let Some(text) = part.get("text").and_then(|v| v.as_str()) {
            if !out.is_empty() {
                out.push('\n');
            }
            out.push_str(text.trim());
        }
    }
    non_empty(out)
}

fn tool_call_summary(payload: &serde_json::Value) -> Option<String> {
    let calls = payload.get("message")?.get("tool_calls")?.as_array()?;
    let mut out = String::new();
    for call in calls {
        let name = call.get("name").and_then(|v| v.as_str()).unwrap_or("tool");
        let args = call
            .get("arguments")
            .map(|v| v.to_string())
            .unwrap_or_default();
        if !out.is_empty() {
            out.push('\n');
        }
        out.push_str(name);
        if !args.is_empty() {
            out.push_str(": ");
            out.push_str(&args);
        }
    }
    non_empty(out)
}

fn non_empty(value: String) -> Option<String> {
    let value = value.trim().to_string();
    if value.is_empty() {
        None
    } else {
        Some(value)
    }
}

async fn index_search_row(
    tx: &mut sqlx::Transaction<'_, sqlx::Sqlite>,
    event_id: i64,
    session_id: &SessionId,
    kind: EventKind,
    payload: &serde_json::Value,
    created_at: &str,
) -> Result<()> {
    let Some(content) = searchable_content(kind, payload) else {
        return Ok(());
    };
    sqlx::query(
        "INSERT INTO session_event_search (event_id, session_id, kind, content, created_at) \
         VALUES (?, ?, ?, ?, ?)",
    )
    .bind(event_id.to_string())
    .bind(session_id.as_str())
    .bind(kind_to_str(kind))
    .bind(content)
    .bind(created_at)
    .execute(&mut **tx)
    .await?;
    Ok(())
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
        index_search_row(
            &mut tx,
            inserted_id,
            session_id,
            kind,
            &payload,
            &created_at,
        )
        .await?;
        tx.commit().await?;
        notify_write(&self.write_notifier);
        Ok(inserted_id)
    }

    async fn append_idempotent(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
        idempotency_key: &str,
    ) -> Result<Option<i64>> {
        let mut tx = self.pool.begin().await?;
        let created_at = Utc::now().to_rfc3339();
        let inserted_key: Option<String> = sqlx::query_scalar(
            "INSERT INTO event_idempotency_keys (key, session_id, created_at) \
             VALUES (?, ?, ?) \
             ON CONFLICT(key) DO NOTHING \
             RETURNING key",
        )
        .bind(idempotency_key)
        .bind(session_id.as_str())
        .bind(&created_at)
        .fetch_optional(&mut *tx)
        .await?;
        if inserted_key.is_none() {
            tx.commit().await?;
            return Ok(None);
        }

        let next_seq: Option<i64> =
            sqlx::query_scalar("SELECT MAX(seq) FROM session_events WHERE session_id = ?")
                .bind(session_id.as_str())
                .fetch_one(&mut *tx)
                .await?;
        let seq = next_seq.unwrap_or(0) + 1;
        let payload_json = serde_json::to_string(&payload)?;
        let inserted_id: Option<i64> = sqlx::query_scalar(
            "INSERT INTO session_events (session_id, seq, kind, payload_json, created_at) \
             VALUES (?, ?, ?, ?, ?) \
             RETURNING id",
        )
        .bind(session_id.as_str())
        .bind(seq)
        .bind(kind_to_str(kind))
        .bind(&payload_json)
        .bind(&created_at)
        .fetch_optional(&mut *tx)
        .await?;
        if let Some(inserted_id) = inserted_id {
            sqlx::query("UPDATE event_idempotency_keys SET event_id = ? WHERE key = ?")
                .bind(inserted_id)
                .bind(idempotency_key)
                .execute(&mut *tx)
                .await?;
            index_search_row(
                &mut tx,
                inserted_id,
                session_id,
                kind,
                &payload,
                &created_at,
            )
            .await?;
        }
        tx.commit().await?;
        if inserted_id.is_some() {
            notify_write(&self.write_notifier);
        }
        Ok(inserted_id)
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
