use chrono::{DateTime, Utc};
use sqlx::{Connection, QueryBuilder, Sqlite, SqliteConnection};

use crate::repos::Result;

use super::EventsLogWrite;

const EVENTS_LOG_INSERT_CHUNK_EVENTS: usize = 200;

pub(super) async fn outbox_enqueue(
    conn: &mut SqliteConnection,
    channel_name: &str,
    event_type: &str,
    payload_json: &str,
    now: &str,
) -> Result<i64> {
    let id: i64 = sqlx::query_scalar(
        "INSERT INTO outbound_outbox \
         (channel_name, event_type, payload_json, attempts, next_retry_at, status, created_at) \
         VALUES (?, ?, ?, 0, ?, ?, ?) RETURNING id",
    )
    .bind(channel_name)
    .bind(event_type)
    .bind(payload_json)
    .bind(now)
    .bind("pending")
    .bind(now)
    .fetch_one(conn)
    .await?;
    Ok(id)
}

pub(super) async fn outbox_mark_status(
    conn: &mut SqliteConnection,
    id: i64,
    status: &str,
) -> Result<()> {
    sqlx::query("UPDATE outbound_outbox SET status = ? WHERE id = ?")
        .bind(status)
        .bind(id)
        .execute(conn)
        .await?;
    Ok(())
}

pub(super) async fn outbox_schedule_retry(
    conn: &mut SqliteConnection,
    id: i64,
    attempts: i32,
    next_retry_at: DateTime<Utc>,
) -> Result<()> {
    sqlx::query(
        "UPDATE outbound_outbox SET attempts = ?, next_retry_at = ?, status = ? WHERE id = ?",
    )
    .bind(attempts)
    .bind(next_retry_at.to_rfc3339())
    .bind("pending")
    .bind(id)
    .execute(conn)
    .await?;
    Ok(())
}

pub(super) async fn events_log_batch(
    conn: &mut SqliteConnection,
    events: &[EventsLogWrite],
    recorded_at: &str,
) -> Result<()> {
    if events.is_empty() {
        return Ok(());
    }
    let mut tx = conn.begin().await?;
    for chunk in events.chunks(EVENTS_LOG_INSERT_CHUNK_EVENTS) {
        let mut builder = QueryBuilder::<Sqlite>::new(
            "INSERT INTO events_log (event_type, payload_json, occurred_at, recorded_at) ",
        );
        builder.push_values(chunk, |mut row, event| {
            row.push_bind(&event.event_type)
                .push_bind(&event.payload_json)
                .push_bind(&event.occurred_at)
                .push_bind(recorded_at);
        });
        builder.build().execute(&mut *tx).await?;
    }
    tx.commit().await?;
    Ok(())
}
