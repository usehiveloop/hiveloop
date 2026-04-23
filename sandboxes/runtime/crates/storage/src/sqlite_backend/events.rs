use bridge_core::BridgeEvent;
use rusqlite::params;
use tokio_rusqlite::Connection;
use tracing::error;

use crate::compression;
use crate::error::StorageError;

pub(super) async fn enqueue_event(
    conn: &Connection,
    event: &BridgeEvent,
) -> Result<String, StorageError> {
    let json = serde_json::to_vec(event)?;
    let blob = compression::compress(&json)?;
    let event_type = serde_json::to_value(&event.event_type)?
        .as_str()
        .unwrap_or("unknown")
        .to_string();
    let event_id = event.event_id.clone();
    let conversation_id = event.conversation_id.clone();
    let sequence_number = event.sequence_number as i64;

    conn.call(move |conn| {
        conn.execute(
            "INSERT OR REPLACE INTO webhook_outbox
                 (event_id, conversation_id, event_type, payload, sequence_number)
             VALUES (?1, ?2, ?3, ?4, ?5)",
            params![event_id, conversation_id, event_type, blob, sequence_number],
        )?;
        Ok(())
    })
    .await?;

    Ok(event.event_id.clone())
}

pub(super) async fn mark_webhook_delivered(
    conn: &Connection,
    event_id: &str,
) -> Result<(), StorageError> {
    let now = chrono::Utc::now().to_rfc3339();
    let event_id = event_id.to_string();
    conn.call(move |conn| {
        conn.execute(
            "UPDATE webhook_outbox SET delivered_at = ?1, attempts = attempts + 1
             WHERE event_id = ?2",
            params![now, event_id],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn load_pending_events(
    conn: &Connection,
) -> Result<Vec<BridgeEvent>, StorageError> {
    conn.call(|conn| {
        let mut stmt = conn.prepare(
            "SELECT payload FROM webhook_outbox
             WHERE delivered_at IS NULL
             ORDER BY id ASC",
        )?;

        let results: Vec<BridgeEvent> = stmt
            .query_map([], |row| {
                let blob: Vec<u8> = row.get(0)?;
                Ok(blob)
            })?
            .filter_map(|r| r.ok())
            .filter_map(|blob| {
                let json = compression::decompress(&blob).ok()?;
                match serde_json::from_slice::<BridgeEvent>(&json) {
                    Ok(event) => Some(event),
                    Err(e) => {
                        error!(error = %e, "failed to deserialize event, skipping");
                        None
                    }
                }
            })
            .collect();
        Ok(results)
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn cleanup_delivered_events(
    conn: &Connection,
    older_than_secs: u64,
) -> Result<u64, StorageError> {
    let cutoff =
        (chrono::Utc::now() - chrono::Duration::seconds(older_than_secs as i64)).to_rfc3339();
    conn.call(move |conn| {
        let count = conn.execute(
            "DELETE FROM webhook_outbox
             WHERE delivered_at IS NOT NULL AND delivered_at < ?1",
            params![cutoff],
        )?;
        Ok(count as u64)
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn load_events_since(
    conn: &Connection,
    after_sequence: u64,
    limit: u32,
) -> Result<Vec<BridgeEvent>, StorageError> {
    let after = after_sequence as i64;
    let lim = limit as i64;
    conn.call(move |conn| {
        let mut stmt = conn.prepare(
            "SELECT payload FROM webhook_outbox
             WHERE sequence_number > ?1
             ORDER BY sequence_number ASC
             LIMIT ?2",
        )?;

        let results: Vec<BridgeEvent> = stmt
            .query_map(params![after, lim], |row| {
                let blob: Vec<u8> = row.get(0)?;
                Ok(blob)
            })?
            .filter_map(|r| r.ok())
            .filter_map(|blob| {
                let json = compression::decompress(&blob).ok()?;
                match serde_json::from_slice::<BridgeEvent>(&json) {
                    Ok(event) => Some(event),
                    Err(e) => {
                        error!(error = %e, "failed to deserialize BridgeEvent, skipping");
                        None
                    }
                }
            })
            .collect();
        Ok(results)
    })
    .await
    .map_err(StorageError::from)
}
