use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::compression;
use crate::error::StorageError;

pub(super) async fn save_session(
    conn: &Connection,
    task_id: &str,
    agent_id: &str,
    history_json: &[u8],
) -> Result<(), StorageError> {
    let blob = compression::compress(history_json)?;
    let now = chrono::Utc::now().to_rfc3339();
    let task_id = task_id.to_string();
    let agent_id = agent_id.to_string();
    conn.call(move |conn| {
        conn.execute(
            "INSERT INTO session_store (task_id, agent_id, content, updated_at)
             VALUES (?1, ?2, ?3, ?4)
             ON CONFLICT(task_id) DO UPDATE SET
                 content = excluded.content,
                 updated_at = excluded.updated_at",
            params![task_id, agent_id, blob, now],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn load_sessions(
    conn: &Connection,
    agent_id: &str,
) -> Result<Vec<(String, Vec<u8>)>, StorageError> {
    let agent_id = agent_id.to_string();
    conn.call(move |conn| {
        let mut stmt =
            conn.prepare("SELECT task_id, content FROM session_store WHERE agent_id = ?1")?;
        let results: Vec<(String, Vec<u8>)> = stmt
            .query_map(params![agent_id], |row| {
                Ok((row.get::<_, String>(0)?, row.get::<_, Vec<u8>>(1)?))
            })?
            .filter_map(|r| r.ok())
            .filter_map(|(task_id, blob)| {
                let json = compression::decompress(&blob).ok()?;
                Some((task_id, json))
            })
            .collect();
        Ok(results)
    })
    .await
    .map_err(StorageError::from)
}

pub(super) async fn delete_sessions_for_agent(
    conn: &Connection,
    agent_id: &str,
) -> Result<(), StorageError> {
    let agent_id = agent_id.to_string();
    conn.call(move |conn| {
        conn.execute(
            "DELETE FROM session_store WHERE agent_id = ?1",
            params![agent_id],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn delete_sessions_by_prefix(
    conn: &Connection,
    prefix: &str,
) -> Result<(), StorageError> {
    let pattern = format!("{}%", prefix);
    conn.call(move |conn| {
        conn.execute(
            "DELETE FROM session_store WHERE task_id LIKE ?1",
            params![pattern],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}
