use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::backend::JournalEntryRow;
use crate::compression;
use crate::error::StorageError;

pub(super) async fn append_journal_entry(
    conn: &Connection,
    entry_id: &str,
    conversation_id: &str,
    chain_index: u32,
    entry_type: &str,
    content: &str,
    created_at: chrono::DateTime<chrono::Utc>,
) -> Result<(), StorageError> {
    let entry_id = entry_id.to_string();
    let conversation_id = conversation_id.to_string();
    let chain_index = chain_index as i64;
    let entry_type = entry_type.to_string();
    let compressed = compression::compress(content.as_bytes())
        .map_err(|e| StorageError::Compression(e.to_string()))?;
    let created_at = created_at.to_rfc3339();

    conn.call(move |conn| {
        conn.execute(
            "INSERT OR REPLACE INTO journal_entries
                 (id, conversation_id, chain_index, entry_type, content, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![
                entry_id,
                conversation_id,
                chain_index,
                entry_type,
                compressed,
                created_at
            ],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn load_journal(
    conn: &Connection,
    conversation_id: &str,
) -> Result<Vec<JournalEntryRow>, StorageError> {
    let conversation_id = conversation_id.to_string();
    conn.call(move |conn| {
        let mut stmt = conn.prepare(
            "SELECT id, conversation_id, chain_index, entry_type, content, created_at
             FROM journal_entries
             WHERE conversation_id = ?1
             ORDER BY created_at ASC",
        )?;
        let rows = stmt
            .query_map(params![conversation_id], |row| {
                let compressed: Vec<u8> = row.get(4)?;
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, i64>(2)?,
                    row.get::<_, String>(3)?,
                    compressed,
                    row.get::<_, String>(5)?,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()?;

        let mut entries = Vec::with_capacity(rows.len());
        for (id, conv_id, chain_index, entry_type, compressed, created_at) in rows {
            let decompressed = compression::decompress(&compressed)
                .map_err(|e| rusqlite::Error::ToSqlConversionFailure(Box::new(e)))?;
            let content = String::from_utf8(decompressed)
                .map_err(|e| rusqlite::Error::ToSqlConversionFailure(Box::new(e)))?;
            entries.push(JournalEntryRow {
                id,
                conversation_id: conv_id,
                chain_index: chain_index as u32,
                entry_type,
                content,
                created_at,
            });
        }
        Ok(entries)
    })
    .await
    .map_err(StorageError::from)
}
