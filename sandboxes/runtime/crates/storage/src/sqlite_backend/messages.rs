use bridge_core::Message;
use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::compression;
use crate::error::StorageError;

pub(super) async fn append_message(
    conn: &Connection,
    conversation_id: &str,
    message_index: u64,
    message: &Message,
) -> Result<(), StorageError> {
    let content_json = serde_json::to_vec(&message.content)?;
    let content_blob = compression::compress(&content_json)?;
    let role_str = serde_json::to_value(&message.role)?
        .as_str()
        .unwrap_or("user")
        .to_string();
    let timestamp = message.timestamp.to_rfc3339();
    let conversation_id = conversation_id.to_string();

    conn.call(move |conn| {
        conn.execute(
            "INSERT OR REPLACE INTO messages
                 (conversation_id, message_index, role, content, timestamp)
             VALUES (?1, ?2, ?3, ?4, ?5)",
            params![
                conversation_id,
                message_index as i64,
                role_str,
                content_blob,
                timestamp
            ],
        )?;
        let now = chrono::Utc::now().to_rfc3339();
        conn.execute(
            "UPDATE conversations SET updated_at = ?1 WHERE conversation_id = ?2",
            params![now, conversation_id],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn replace_messages(
    conn: &Connection,
    conversation_id: &str,
    messages: &[Message],
) -> Result<(), StorageError> {
    let conversation_id = conversation_id.to_string();
    let mut prepared: Vec<(String, Vec<u8>, String)> = Vec::new();
    for msg in messages {
        let content_json = serde_json::to_vec(&msg.content)?;
        let content_blob = compression::compress(&content_json)?;
        let role_str = serde_json::to_value(&msg.role)?
            .as_str()
            .unwrap_or("user")
            .to_string();
        let timestamp = msg.timestamp.to_rfc3339();
        prepared.push((role_str, content_blob, timestamp));
    }

    conn.call(move |conn| {
        conn.execute(
            "DELETE FROM messages WHERE conversation_id = ?1",
            params![conversation_id],
        )?;

        for (idx, (role_str, content_blob, timestamp)) in prepared.into_iter().enumerate() {
            conn.execute(
                "INSERT INTO messages
                     (conversation_id, message_index, role, content, timestamp)
                 VALUES (?1, ?2, ?3, ?4, ?5)",
                params![
                    conversation_id,
                    idx as i64,
                    role_str,
                    content_blob,
                    timestamp
                ],
            )?;
        }

        let now = chrono::Utc::now().to_rfc3339();
        conn.execute(
            "UPDATE conversations SET updated_at = ?1 WHERE conversation_id = ?2",
            params![now, conversation_id],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}
