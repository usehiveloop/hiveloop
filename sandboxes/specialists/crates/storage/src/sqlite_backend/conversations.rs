use bridge_core::{ConversationRecord, Message};
use rusqlite::params;
use std::collections::HashMap;
use tokio_rusqlite::Connection;

use crate::compression;
use crate::error::StorageError;

pub(super) async fn create_conversation(
    conn: &Connection,
    agent_id: &str,
    conversation_id: &str,
    title: Option<&str>,
    created_at: chrono::DateTime<chrono::Utc>,
) -> Result<(), StorageError> {
    let now = created_at.to_rfc3339();
    let agent_id = agent_id.to_string();
    let conversation_id = conversation_id.to_string();
    let title = title.unwrap_or("").to_string();

    conn.call(move |conn| {
        conn.execute(
            "INSERT OR IGNORE INTO conversations
                 (conversation_id, agent_id, title, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, ?4)",
            params![conversation_id, agent_id, title, now],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn delete_conversation(
    conn: &Connection,
    conversation_id: &str,
) -> Result<(), StorageError> {
    let conversation_id = conversation_id.to_string();
    conn.call(move |conn| {
        conn.execute(
            "DELETE FROM messages WHERE conversation_id = ?1",
            params![conversation_id],
        )?;
        conn.execute(
            "DELETE FROM conversations WHERE conversation_id = ?1",
            params![conversation_id],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn load_conversations(
    conn: &Connection,
    agent_id: &str,
) -> Result<Vec<ConversationRecord>, StorageError> {
    let agent_id = agent_id.to_string();
    conn.call(move |conn| {
        let mut conv_stmt = conn.prepare(
            "SELECT conversation_id, title, created_at, updated_at
             FROM conversations WHERE agent_id = ?1",
        )?;

        let mut records: Vec<ConversationRecord> = Vec::new();
        let mut conv_map: HashMap<String, usize> = HashMap::new();

        let rows = conv_stmt.query_map(params![agent_id], |row| {
            Ok((
                row.get::<_, String>(0)?,
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, String>(3)?,
            ))
        })?;

        for row in rows.flatten() {
            let (conv_id, title, created_at, updated_at) = row;
            let idx = records.len();
            conv_map.insert(conv_id.clone(), idx);
            records.push(ConversationRecord {
                id: conv_id,
                agent_id: agent_id.clone(),
                title: if title.is_empty() { None } else { Some(title) },
                created_at: created_at.parse().unwrap_or_else(|_| chrono::Utc::now()),
                updated_at: updated_at.parse().unwrap_or_else(|_| chrono::Utc::now()),
                messages: Vec::new(),
            });
        }

        // Load messages for all conversations
        let mut msg_stmt = conn.prepare(
            "SELECT role, content, timestamp FROM messages
             WHERE conversation_id = ?1
             ORDER BY message_index ASC",
        )?;

        for (conv_id, idx) in &conv_map {
            let msg_rows = msg_stmt.query_map(params![conv_id], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, Vec<u8>>(1)?,
                    row.get::<_, String>(2)?,
                ))
            })?;

            for msg_row in msg_rows.flatten() {
                let (role_str, content_blob, timestamp_str) = msg_row;
                let content_json = match compression::decompress(&content_blob) {
                    Ok(j) => j,
                    Err(_) => continue,
                };
                let content: Vec<bridge_core::ContentBlock> =
                    match serde_json::from_slice(&content_json) {
                        Ok(c) => c,
                        Err(_) => continue,
                    };
                let role: bridge_core::Role =
                    serde_json::from_value(serde_json::Value::String(role_str))
                        .unwrap_or(bridge_core::Role::User);
                let timestamp = timestamp_str.parse().unwrap_or_else(|_| chrono::Utc::now());

                records[*idx].messages.push(Message {
                    role,
                    content,
                    timestamp,
                    system_reminder: None,
                });
            }
        }

        Ok(records)
    })
    .await
    .map_err(StorageError::from)
}
