use bridge_core::AgentDefinition;
use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::compression;
use crate::error::StorageError;

pub(super) async fn save_agent(
    conn: &Connection,
    definition: &AgentDefinition,
) -> Result<(), StorageError> {
    let json = serde_json::to_vec(definition)?;
    let blob = compression::compress(&json)?;
    let now = chrono::Utc::now().to_rfc3339();
    let id = definition.id.clone();
    let name = definition.name.clone();
    let version = definition.version.clone().unwrap_or_default();

    conn.call(move |conn| {
        conn.execute(
            "INSERT INTO agents (agent_id, name, version, definition, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?5)
             ON CONFLICT(agent_id) DO UPDATE SET
                 name = excluded.name,
                 version = excluded.version,
                 definition = excluded.definition,
                 updated_at = excluded.updated_at",
            params![id, name, version, blob, now],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn delete_agent(conn: &Connection, agent_id: &str) -> Result<(), StorageError> {
    let agent_id = agent_id.to_string();
    conn.call(move |conn| {
        // Delete conversations' messages first
        let mut stmt =
            conn.prepare("SELECT conversation_id FROM conversations WHERE agent_id = ?1")?;
        let conv_ids: Vec<String> = stmt
            .query_map(params![agent_id], |row| row.get(0))?
            .filter_map(|r| r.ok())
            .collect();

        for conv_id in &conv_ids {
            conn.execute(
                "DELETE FROM messages WHERE conversation_id = ?1",
                params![conv_id],
            )?;
        }

        conn.execute(
            "DELETE FROM conversations WHERE agent_id = ?1",
            params![agent_id],
        )?;
        conn.execute(
            "DELETE FROM session_store WHERE agent_id = ?1",
            params![agent_id],
        )?;
        conn.execute(
            "DELETE FROM metrics_snapshots WHERE agent_id = ?1",
            params![agent_id],
        )?;
        conn.execute("DELETE FROM agents WHERE agent_id = ?1", params![agent_id])?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn load_all_agents(
    conn: &Connection,
) -> Result<Vec<AgentDefinition>, StorageError> {
    conn.call(|conn| {
        let mut stmt = conn.prepare("SELECT definition FROM agents")?;
        let agents: Vec<AgentDefinition> = stmt
            .query_map([], |row| {
                let blob: Vec<u8> = row.get(0)?;
                Ok(blob)
            })?
            .filter_map(|r| r.ok())
            .filter_map(|blob| {
                let json = compression::decompress(&blob).ok()?;
                serde_json::from_slice(&json).ok()
            })
            .collect();
        Ok(agents)
    })
    .await
    .map_err(StorageError::from)
}
