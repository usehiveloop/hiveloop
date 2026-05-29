use chrono::Utc;
use domain::{EventKind, SessionId};
use serde_json::Value;
use sqlx::{Connection, Sqlite, SqliteConnection};

use crate::repos::Result;

pub(super) async fn event_append(
    conn: &mut SqliteConnection,
    session_id: &SessionId,
    kind: EventKind,
    payload: Value,
) -> Result<i64> {
    let mut tx = conn.begin().await?;
    let inserted_id = insert_session_event(&mut tx, session_id, kind, payload, None).await?;
    tx.commit().await?;
    Ok(inserted_id.expect("non-idempotent insert returns id"))
}

pub(super) async fn event_append_idempotent(
    conn: &mut SqliteConnection,
    session_id: &SessionId,
    kind: EventKind,
    payload: Value,
    idempotency_key: &str,
) -> Result<Option<i64>> {
    let mut tx = conn.begin().await?;
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

    let inserted_id =
        insert_session_event(&mut tx, session_id, kind, payload, Some(&created_at)).await?;
    if let Some(inserted_id) = inserted_id {
        sqlx::query("UPDATE event_idempotency_keys SET event_id = ? WHERE key = ?")
            .bind(inserted_id)
            .bind(idempotency_key)
            .execute(&mut *tx)
            .await?;
    }
    tx.commit().await?;
    Ok(inserted_id)
}

async fn insert_session_event(
    tx: &mut sqlx::Transaction<'_, Sqlite>,
    session_id: &SessionId,
    kind: EventKind,
    payload: Value,
    created_at_override: Option<&str>,
) -> Result<Option<i64>> {
    let next_seq: Option<i64> =
        sqlx::query_scalar("SELECT MAX(seq) FROM session_events WHERE session_id = ?")
            .bind(session_id.as_str())
            .fetch_one(&mut **tx)
            .await?;
    let seq = next_seq.unwrap_or(0) + 1;
    let payload_json = serde_json::to_string(&payload)?;
    let created_at_owned;
    let created_at = match created_at_override {
        Some(value) => value,
        None => {
            created_at_owned = Utc::now().to_rfc3339();
            &created_at_owned
        }
    };
    let inserted_id: Option<i64> = sqlx::query_scalar(
        "INSERT INTO session_events (session_id, seq, kind, payload_json, created_at) \
         VALUES (?, ?, ?, ?, ?) \
         RETURNING id",
    )
    .bind(session_id.as_str())
    .bind(seq)
    .bind(kind_to_str(kind))
    .bind(&payload_json)
    .bind(created_at)
    .fetch_optional(&mut **tx)
    .await?;
    if let Some(inserted_id) = inserted_id {
        index_search_row(tx, inserted_id, session_id, kind, &payload, created_at).await?;
    }
    Ok(inserted_id)
}

async fn index_search_row(
    tx: &mut sqlx::Transaction<'_, Sqlite>,
    event_id: i64,
    session_id: &SessionId,
    kind: EventKind,
    payload: &Value,
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

fn searchable_content(kind: EventKind, payload: &Value) -> Option<String> {
    match kind {
        EventKind::UserMessage | EventKind::AssistantMessage | EventKind::ToolResult => {
            message_text(payload)
        }
        EventKind::ToolCall => tool_call_summary(payload),
        EventKind::RunEvent | EventKind::SpecialistEvent | EventKind::Error => None,
    }
}

fn message_text(payload: &Value) -> Option<String> {
    let parts = payload.get("message")?.get("parts")?.as_array()?;
    let mut out = String::new();
    for part in parts {
        if let Some(text) = part.get("text").and_then(Value::as_str) {
            if !out.is_empty() {
                out.push('\n');
            }
            out.push_str(text.trim());
        }
    }
    non_empty(out)
}

fn tool_call_summary(payload: &Value) -> Option<String> {
    let calls = payload.get("message")?.get("tool_calls")?.as_array()?;
    let mut out = String::new();
    for call in calls {
        let name = call.get("name").and_then(Value::as_str).unwrap_or("tool");
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
