use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::backend::ArtifactUploadRow;
use crate::error::StorageError;

pub(super) async fn get_artifact_upload(
    conn: &Connection,
    idempotency_key: &str,
) -> Result<Option<ArtifactUploadRow>, StorageError> {
    let key = idempotency_key.to_string();
    conn.call(move |conn| {
        let mut stmt = conn.prepare(
            "SELECT idempotency_key, agent_id, conversation_id, location,
                    total_size, file_sha256, bytes_sent, status,
                    response_json, last_error, created_at, updated_at
               FROM artifact_uploads
              WHERE idempotency_key = ?1",
        )?;
        let mut rows = stmt.query(params![key])?;
        if let Some(row) = rows.next()? {
            Ok(Some(ArtifactUploadRow {
                idempotency_key: row.get(0)?,
                agent_id: row.get(1)?,
                conversation_id: row.get(2)?,
                location: row.get(3)?,
                total_size: row.get::<_, i64>(4)? as u64,
                file_sha256: row.get(5)?,
                bytes_sent: row.get::<_, i64>(6)? as u64,
                status: row.get(7)?,
                response_json: row.get(8)?,
                last_error: row.get(9)?,
                created_at: row.get(10)?,
                updated_at: row.get(11)?,
            }))
        } else {
            Ok(None)
        }
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn upsert_in_progress(
    conn: &Connection,
    row: ArtifactUploadRow,
) -> Result<(), StorageError> {
    conn.call(move |conn| {
        let now = chrono::Utc::now().to_rfc3339();
        conn.execute(
            "INSERT INTO artifact_uploads
                (idempotency_key, agent_id, conversation_id, location,
                 total_size, file_sha256, bytes_sent, status,
                 response_json, last_error, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, 'in_progress', NULL, NULL, ?8, ?8)
             ON CONFLICT(idempotency_key) DO UPDATE SET
                 location    = excluded.location,
                 total_size  = excluded.total_size,
                 file_sha256 = excluded.file_sha256,
                 bytes_sent  = MAX(artifact_uploads.bytes_sent, excluded.bytes_sent),
                 status      = 'in_progress',
                 last_error  = NULL,
                 updated_at  = excluded.updated_at",
            params![
                row.idempotency_key,
                row.agent_id,
                row.conversation_id,
                row.location,
                row.total_size as i64,
                row.file_sha256,
                row.bytes_sent as i64,
                now,
            ],
        )?;
        Ok(())
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn update_offset(
    conn: &Connection,
    idempotency_key: &str,
    bytes_sent: u64,
) -> Result<(), StorageError> {
    let key = idempotency_key.to_string();
    conn.call(move |conn| {
        let now = chrono::Utc::now().to_rfc3339();
        conn.execute(
            "UPDATE artifact_uploads
                SET bytes_sent = ?1, updated_at = ?2
              WHERE idempotency_key = ?3",
            params![bytes_sent as i64, now, key],
        )?;
        Ok(())
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn mark_completed(
    conn: &Connection,
    idempotency_key: &str,
    bytes_sent: u64,
    response_json: &str,
) -> Result<(), StorageError> {
    let key = idempotency_key.to_string();
    let resp = response_json.to_string();
    conn.call(move |conn| {
        let now = chrono::Utc::now().to_rfc3339();
        conn.execute(
            "UPDATE artifact_uploads
                SET status = 'completed',
                    bytes_sent = ?1,
                    response_json = ?2,
                    last_error = NULL,
                    updated_at = ?3
              WHERE idempotency_key = ?4",
            params![bytes_sent as i64, resp, now, key],
        )?;
        Ok(())
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn mark_failed(
    conn: &Connection,
    idempotency_key: &str,
    error: &str,
) -> Result<(), StorageError> {
    let key = idempotency_key.to_string();
    let err = error.to_string();
    conn.call(move |conn| {
        let now = chrono::Utc::now().to_rfc3339();
        conn.execute(
            "UPDATE artifact_uploads
                SET status = 'failed',
                    last_error = ?1,
                    updated_at = ?2
              WHERE idempotency_key = ?3",
            params![err, now, key],
        )?;
        Ok(())
    })
    .await
    .map_err(StorageError::from)
}
