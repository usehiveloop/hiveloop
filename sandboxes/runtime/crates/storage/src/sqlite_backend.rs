use async_trait::async_trait;
use bridge_core::{AgentDefinition, BridgeEvent, ConversationRecord, Message, MetricsSnapshot};
use rusqlite::params;
use std::collections::HashMap;
use tokio_rusqlite::Connection;
use tracing::{error, info};

use crate::backend::StorageBackend;
use crate::compression;
use crate::config::StorageConfig;
use crate::error::StorageError;
use crate::schema;

/// SQLite-backed storage implementation.
///
/// Uses `tokio-rusqlite` for async access to a local SQLite file.
/// All data lives on disk — no remote sync or replication.
pub struct SqliteBackend {
    conn: Connection,
}

impl SqliteBackend {
    /// Open (or create) a SQLite database at the configured path and run migrations.
    pub async fn new(config: &StorageConfig) -> Result<Self, StorageError> {
        let conn = Connection::open(&config.path)
            .await
            .map_err(|e| StorageError::Database(format!("failed to open database: {e}")))?;

        // Enable WAL mode and foreign keys
        conn.call(|conn| {
            conn.execute_batch(
                "PRAGMA journal_mode = WAL;
                 PRAGMA foreign_keys = ON;
                 PRAGMA busy_timeout = 5000;",
            )?;
            schema::run_migrations(conn)?;
            Ok(())
        })
        .await?;

        info!(path = %config.path, "sqlite storage backend initialized");
        Ok(Self { conn })
    }

    /// Create a backend from an existing connection (for testing).
    pub async fn from_connection(conn: Connection) -> Result<Self, StorageError> {
        conn.call(|conn| {
            conn.execute_batch("PRAGMA foreign_keys = ON;")?;
            schema::run_migrations(conn)?;
            Ok(())
        })
        .await?;
        Ok(Self { conn })
    }
}

#[async_trait]
impl StorageBackend for SqliteBackend {
    // ── Agents ──────────────────────────────────────────────

    async fn save_agent(&self, definition: &AgentDefinition) -> Result<(), StorageError> {
        let json = serde_json::to_vec(definition)?;
        let blob = compression::compress(&json)?;
        let now = chrono::Utc::now().to_rfc3339();
        let id = definition.id.clone();
        let name = definition.name.clone();
        let version = definition.version.clone().unwrap_or_default();

        self.conn
            .call(move |conn| {
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

    async fn delete_agent(&self, agent_id: &str) -> Result<(), StorageError> {
        let agent_id = agent_id.to_string();
        self.conn
            .call(move |conn| {
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

    async fn load_all_agents(&self) -> Result<Vec<AgentDefinition>, StorageError> {
        self.conn
            .call(|conn| {
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

    // ── Conversations ───────────────────────────────────────

    async fn create_conversation(
        &self,
        agent_id: &str,
        conversation_id: &str,
        title: Option<&str>,
        created_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError> {
        let now = created_at.to_rfc3339();
        let agent_id = agent_id.to_string();
        let conversation_id = conversation_id.to_string();
        let title = title.unwrap_or("").to_string();

        self.conn
            .call(move |conn| {
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

    async fn delete_conversation(&self, conversation_id: &str) -> Result<(), StorageError> {
        let conversation_id = conversation_id.to_string();
        self.conn
            .call(move |conn| {
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

    async fn load_conversations(
        &self,
        agent_id: &str,
    ) -> Result<Vec<ConversationRecord>, StorageError> {
        let agent_id = agent_id.to_string();
        self.conn
            .call(move |conn| {
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
                        let timestamp =
                            timestamp_str.parse().unwrap_or_else(|_| chrono::Utc::now());

                        records[*idx].messages.push(Message {
                            role,
                            content,
                            timestamp,
                        });
                    }
                }

                Ok(records)
            })
            .await
            .map_err(StorageError::from)
    }

    // ── Messages ────────────────────────────────────────────

    async fn append_message(
        &self,
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

        self.conn
            .call(move |conn| {
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

    async fn replace_messages(
        &self,
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

        self.conn
            .call(move |conn| {
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

    // ── Event outbox ───────────────────────────────────────

    async fn enqueue_event(&self, event: &BridgeEvent) -> Result<String, StorageError> {
        let json = serde_json::to_vec(event)?;
        let blob = compression::compress(&json)?;
        let event_type = serde_json::to_value(&event.event_type)?
            .as_str()
            .unwrap_or("unknown")
            .to_string();
        let event_id = event.event_id.clone();
        let conversation_id = event.conversation_id.clone();
        let sequence_number = event.sequence_number as i64;

        self.conn
            .call(move |conn| {
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

    async fn mark_webhook_delivered(&self, event_id: &str) -> Result<(), StorageError> {
        let now = chrono::Utc::now().to_rfc3339();
        let event_id = event_id.to_string();
        self.conn
            .call(move |conn| {
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

    async fn load_pending_events(&self) -> Result<Vec<BridgeEvent>, StorageError> {
        self.conn
            .call(|conn| {
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

    async fn cleanup_delivered_events(&self, older_than_secs: u64) -> Result<u64, StorageError> {
        let cutoff =
            (chrono::Utc::now() - chrono::Duration::seconds(older_than_secs as i64)).to_rfc3339();
        self.conn
            .call(move |conn| {
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

    async fn load_events_since(
        &self,
        after_sequence: u64,
        limit: u32,
    ) -> Result<Vec<BridgeEvent>, StorageError> {
        let after = after_sequence as i64;
        let lim = limit as i64;
        self.conn
            .call(move |conn| {
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

    // ── Metrics ─────────────────────────────────────────────

    async fn save_metrics_snapshot(
        &self,
        agent_id: &str,
        snapshot: &MetricsSnapshot,
    ) -> Result<(), StorageError> {
        let json = serde_json::to_vec(snapshot)?;
        let blob = compression::compress(&json)?;
        let agent_id = agent_id.to_string();
        self.conn
            .call(move |conn| {
                conn.execute(
                    "INSERT INTO metrics_snapshots (agent_id, snapshot) VALUES (?1, ?2)",
                    params![agent_id, blob],
                )?;
                Ok(())
            })
            .await?;
        Ok(())
    }

    // ── Session store ───────────────────────────────────────

    async fn save_session(
        &self,
        task_id: &str,
        agent_id: &str,
        history_json: &[u8],
    ) -> Result<(), StorageError> {
        let blob = compression::compress(history_json)?;
        let now = chrono::Utc::now().to_rfc3339();
        let task_id = task_id.to_string();
        let agent_id = agent_id.to_string();
        self.conn
            .call(move |conn| {
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

    async fn load_sessions(&self, agent_id: &str) -> Result<Vec<(String, Vec<u8>)>, StorageError> {
        let agent_id = agent_id.to_string();
        self.conn
            .call(move |conn| {
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

    async fn delete_sessions_for_agent(&self, agent_id: &str) -> Result<(), StorageError> {
        let agent_id = agent_id.to_string();
        self.conn
            .call(move |conn| {
                conn.execute(
                    "DELETE FROM session_store WHERE agent_id = ?1",
                    params![agent_id],
                )?;
                Ok(())
            })
            .await?;
        Ok(())
    }

    async fn delete_sessions_by_prefix(&self, prefix: &str) -> Result<(), StorageError> {
        let pattern = format!("{}%", prefix);
        self.conn
            .call(move |conn| {
                conn.execute(
                    "DELETE FROM session_store WHERE task_id LIKE ?1",
                    params![pattern],
                )?;
                Ok(())
            })
            .await?;
        Ok(())
    }

    // ── Lifecycle ───────────────────────────────────────────

    async fn sync(&self) -> Result<(), StorageError> {
        // No remote sync needed for local SQLite — this is a no-op.
        Ok(())
    }
}
