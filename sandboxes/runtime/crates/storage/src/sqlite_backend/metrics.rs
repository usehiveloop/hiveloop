use bridge_core::MetricsSnapshot;
use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::compression;
use crate::error::StorageError;

pub(super) async fn save_metrics_snapshot(
    conn: &Connection,
    agent_id: &str,
    snapshot: &MetricsSnapshot,
) -> Result<(), StorageError> {
    let json = serde_json::to_vec(snapshot)?;
    let blob = compression::compress(&json)?;
    let agent_id = agent_id.to_string();
    conn.call(move |conn| {
        conn.execute(
            "INSERT INTO metrics_snapshots (agent_id, snapshot) VALUES (?1, ?2)",
            params![agent_id, blob],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}
