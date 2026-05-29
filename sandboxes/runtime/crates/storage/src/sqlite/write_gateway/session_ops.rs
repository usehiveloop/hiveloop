use chrono::{DateTime, Utc};
use domain::{Session, SessionId, SessionStatus};
use sqlx::{Executor, SqliteConnection};

use crate::repos::{Result, StorageError};

pub(super) async fn configure_write_connection(conn: &mut SqliteConnection) -> Result<()> {
    conn.execute("PRAGMA journal_mode = WAL").await?;
    conn.execute("PRAGMA synchronous = NORMAL").await?;
    conn.execute("PRAGMA foreign_keys = ON").await?;
    conn.execute("PRAGMA busy_timeout = 30000").await?;
    conn.execute("PRAGMA temp_store = MEMORY").await?;
    conn.execute("PRAGMA wal_autocheckpoint = 1000").await?;
    Ok(())
}

pub(super) async fn config_upsert(
    conn: &mut SqliteConnection,
    definition_json: String,
    updated_at: String,
) -> Result<()> {
    sqlx::query(
        "INSERT INTO agent_config (id, definition_json, updated_at) VALUES (1, ?, ?) \
         ON CONFLICT(id) DO UPDATE SET \
         definition_json = excluded.definition_json, \
         updated_at = excluded.updated_at",
    )
    .bind(definition_json)
    .bind(updated_at)
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn session_create(conn: &mut SqliteConnection, session: Session) -> Result<()> {
    let result = sqlx::query(
        "INSERT INTO sessions (id, channel, thread_ts, agent_session_id, status, created_at, \
         last_activity_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
    )
    .bind(session.id.as_str())
    .bind(session.channel)
    .bind(session.thread_ts)
    .bind(session.agent_session_id)
    .bind(status_to_str(session.status))
    .bind(session.created_at.to_rfc3339())
    .bind(session.last_activity_at.to_rfc3339())
    .execute(conn)
    .await;
    match result {
        Ok(_) => Ok(()),
        Err(sqlx::Error::Database(db_err)) if db_err.is_unique_violation() => {
            Err(StorageError::Conflict)
        }
        Err(error) => Err(StorageError::from(error)),
    }
}

pub(super) async fn session_touch(
    conn: &mut SqliteConnection,
    id: &SessionId,
    at: DateTime<Utc>,
) -> Result<()> {
    sqlx::query("UPDATE sessions SET last_activity_at = ? WHERE id = ?")
        .bind(at.to_rfc3339())
        .bind(id.as_str())
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn session_set_status(
    conn: &mut SqliteConnection,
    id: &SessionId,
    status: SessionStatus,
) -> Result<()> {
    sqlx::query("UPDATE sessions SET status = ? WHERE id = ?")
        .bind(status_to_str(status))
        .bind(id.as_str())
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn inbound_dedupe_check_and_record(
    conn: &mut SqliteConnection,
    envelope_id: &str,
    received_at: &str,
) -> Result<bool> {
    let result = sqlx::query(
        "INSERT INTO inbound_dedupe (envelope_id, received_at) VALUES (?, ?) \
         ON CONFLICT(envelope_id) DO NOTHING",
    )
    .bind(envelope_id)
    .bind(received_at)
    .execute(conn)
    .await?;
    Ok(result.rows_affected() == 1)
}

pub(super) async fn inbound_dedupe_cleanup(
    conn: &mut SqliteConnection,
    before: &str,
) -> Result<u64> {
    let result = sqlx::query("DELETE FROM inbound_dedupe WHERE received_at < ?")
        .bind(before)
        .execute(conn)
        .await?;
    Ok(result.rows_affected())
}

fn status_to_str(status: SessionStatus) -> &'static str {
    match status {
        SessionStatus::Active => "active",
        SessionStatus::Completed => "completed",
        SessionStatus::Errored => "errored",
    }
}
