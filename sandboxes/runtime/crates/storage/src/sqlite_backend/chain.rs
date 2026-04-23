use rusqlite::params;
use tokio_rusqlite::Connection;

use crate::backend::ChainLinkRow;
use crate::compression;
use crate::error::StorageError;

pub(super) async fn save_chain_link(
    conn: &Connection,
    conversation_id: &str,
    chain_index: u32,
    started_at: chrono::DateTime<chrono::Utc>,
    trigger_token_count: Option<usize>,
    checkpoint_text: Option<&str>,
) -> Result<(), StorageError> {
    let conversation_id = conversation_id.to_string();
    let chain_index = chain_index as i64;
    let started_at = started_at.to_rfc3339();
    let trigger_token_count = trigger_token_count.map(|t| t as i64);
    let compressed_checkpoint = checkpoint_text
        .map(|t| compression::compress(t.as_bytes()))
        .transpose()
        .map_err(|e| StorageError::Compression(e.to_string()))?;

    conn.call(move |conn| {
        conn.execute(
            "INSERT OR REPLACE INTO chain_links
                 (conversation_id, chain_index, started_at, trigger_token_count, checkpoint_text)
             VALUES (?1, ?2, ?3, ?4, ?5)",
            params![
                conversation_id,
                chain_index,
                started_at,
                trigger_token_count,
                compressed_checkpoint
            ],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn complete_chain_link(
    conn: &Connection,
    conversation_id: &str,
    chain_index: u32,
    ended_at: chrono::DateTime<chrono::Utc>,
) -> Result<(), StorageError> {
    let conversation_id = conversation_id.to_string();
    let chain_index = chain_index as i64;
    let ended_at = ended_at.to_rfc3339();

    conn.call(move |conn| {
        conn.execute(
            "UPDATE chain_links SET ended_at = ?1
             WHERE conversation_id = ?2 AND chain_index = ?3",
            params![ended_at, conversation_id, chain_index],
        )?;
        Ok(())
    })
    .await?;
    Ok(())
}

pub(super) async fn load_chain_links(
    conn: &Connection,
    conversation_id: &str,
) -> Result<Vec<ChainLinkRow>, StorageError> {
    let conversation_id = conversation_id.to_string();
    conn.call(move |conn| {
        let mut stmt = conn.prepare(
            "SELECT conversation_id, chain_index, started_at, ended_at,
                    trigger_token_count, checkpoint_text
             FROM chain_links
             WHERE conversation_id = ?1
             ORDER BY chain_index ASC",
        )?;
        let rows = stmt
            .query_map(params![conversation_id], |row| {
                let compressed_checkpoint: Option<Vec<u8>> = row.get(5)?;
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, Option<String>>(3)?,
                    row.get::<_, Option<i64>>(4)?,
                    compressed_checkpoint,
                ))
            })?
            .collect::<Result<Vec<_>, _>>()?;

        let mut links = Vec::with_capacity(rows.len());
        for (conv_id, chain_index, started_at, ended_at, trigger_tokens, compressed_cp) in rows {
            let checkpoint_text = compressed_cp
                .map(|c| {
                    let decompressed = compression::decompress(&c)
                        .map_err(|e| rusqlite::Error::ToSqlConversionFailure(Box::new(e)))?;
                    String::from_utf8(decompressed)
                        .map_err(|e| rusqlite::Error::ToSqlConversionFailure(Box::new(e)))
                })
                .transpose()?;
            links.push(ChainLinkRow {
                conversation_id: conv_id,
                chain_index: chain_index as u32,
                started_at,
                ended_at,
                trigger_token_count: trigger_tokens.map(|t| t as usize),
                checkpoint_text,
            });
        }
        Ok(links)
    })
    .await
    .map_err(StorageError::from)
}
