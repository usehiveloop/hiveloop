use async_trait::async_trait;
use bridge_core::{AgentDefinition, ConversationRecord, Message, MetricsSnapshot, WebhookPayload};
use libsql::{params, Builder, Connection, Database};
use std::collections::HashMap;
use std::time::Duration;
use tracing::{error, info};

use crate::backend::StorageBackend;
use crate::compression;
use crate::config::StorageConfig;
use crate::error::StorageError;
use crate::schema;

/// libSQL-backed storage implementation.
///
/// Uses an embedded replica: reads from local SQLite, writes go through
/// the remote primary, and a background sync keeps the local copy current.
pub struct LibSqlBackend {
    db: Database,
    conn: Connection,
}

impl LibSqlBackend {
    /// Connect to an embedded replica, run migrations, and perform initial sync.
    pub async fn new(config: &StorageConfig) -> Result<Self, StorageError> {
        let mut builder = Builder::new_remote_replica(
            &config.path,
            config.url.clone(),
            config.auth_token.clone(),
        );

        builder = builder
            .sync_interval(Duration::from_secs(config.sync_interval_secs))
            .read_your_writes(true);

        if let Some(ref key) = config.encryption_key {
            builder = builder.encryption_config(libsql::EncryptionConfig {
                cipher: libsql::Cipher::Aes256Cbc,
                encryption_key: key.clone().into(),
            });
        }

        let db = builder
            .build()
            .await
            .map_err(|e| StorageError::Database(format!("failed to open database: {e}")))?;

        // Initial sync to pull remote state
        if let Err(e) = db.sync().await {
            // Non-fatal: local DB may be empty on first run
            info!("initial sync (may be first run): {e}");
        }

        let conn = db.connect()?;

        // Run schema migrations
        schema::run_migrations(&conn).await?;

        info!(path = %config.path, "storage backend initialized");
        Ok(Self { db, conn })
    }

    /// Create a backend from an existing database (for testing).
    pub fn from_database(db: Database, conn: Connection) -> Self {
        Self { db, conn }
    }

    /// Get a reference to the underlying database for manual sync.
    pub fn database(&self) -> &Database {
        &self.db
    }
}

#[async_trait]
impl StorageBackend for LibSqlBackend {
    // ── Agents ──────────────────────────────────────────────

    async fn save_agent(&self, definition: &AgentDefinition) -> Result<(), StorageError> {
        let json = serde_json::to_vec(definition)?;
        let blob = compression::compress(&json)?;
        let now = chrono::Utc::now().to_rfc3339();

        self.conn
            .execute(
                "INSERT INTO agents (agent_id, name, version, definition, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5)
                 ON CONFLICT(agent_id) DO UPDATE SET
                     name = excluded.name,
                     version = excluded.version,
                     definition = excluded.definition,
                     updated_at = excluded.updated_at",
                params![
                    definition.id.clone(),
                    definition.name.clone(),
                    definition.version.as_deref().unwrap_or("").to_string(),
                    blob,
                    now,
                ],
            )
            .await?;
        Ok(())
    }

    async fn delete_agent(&self, agent_id: &str) -> Result<(), StorageError> {
        // Delete conversations first (CASCADE may not be enforced in all SQLite modes)
        let mut rows = self
            .conn
            .query(
                "SELECT conversation_id FROM conversations WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;

        let mut conv_ids = Vec::new();
        while let Some(row) = rows.next().await? {
            conv_ids.push(row.get::<String>(0)?);
        }

        for conv_id in &conv_ids {
            self.conn
                .execute(
                    "DELETE FROM messages WHERE conversation_id = ?1",
                    params![conv_id.as_str()],
                )
                .await?;
        }

        self.conn
            .execute(
                "DELETE FROM conversations WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;
        self.conn
            .execute(
                "DELETE FROM session_store WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;
        self.conn
            .execute(
                "DELETE FROM metrics_snapshots WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;
        self.conn
            .execute("DELETE FROM agents WHERE agent_id = ?1", params![agent_id])
            .await?;
        Ok(())
    }

    async fn load_all_agents(&self) -> Result<Vec<AgentDefinition>, StorageError> {
        let mut rows = self.conn.query("SELECT definition FROM agents", ()).await?;

        let mut agents = Vec::new();
        while let Some(row) = rows.next().await? {
            let blob = row.get::<Vec<u8>>(0)?;
            let json = compression::decompress(&blob)?;
            let def: AgentDefinition = serde_json::from_slice(&json)?;
            agents.push(def);
        }
        Ok(agents)
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
        self.conn
            .execute(
                "INSERT OR IGNORE INTO conversations
                     (conversation_id, agent_id, title, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?4)",
                params![conversation_id, agent_id, title.unwrap_or(""), now],
            )
            .await?;
        Ok(())
    }

    async fn delete_conversation(&self, conversation_id: &str) -> Result<(), StorageError> {
        self.conn
            .execute(
                "DELETE FROM messages WHERE conversation_id = ?1",
                params![conversation_id],
            )
            .await?;
        self.conn
            .execute(
                "DELETE FROM conversations WHERE conversation_id = ?1",
                params![conversation_id],
            )
            .await?;
        Ok(())
    }

    async fn load_conversations(
        &self,
        agent_id: &str,
    ) -> Result<Vec<ConversationRecord>, StorageError> {
        let mut conv_rows = self
            .conn
            .query(
                "SELECT conversation_id, title, created_at, updated_at
                 FROM conversations WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;

        let mut records: Vec<ConversationRecord> = Vec::new();
        let mut conv_map: HashMap<String, usize> = HashMap::new();

        while let Some(row) = conv_rows.next().await? {
            let conv_id: String = row.get(0)?;
            let title: String = row.get(1)?;
            let created_at: String = row.get(2)?;
            let updated_at: String = row.get(3)?;

            let idx = records.len();
            conv_map.insert(conv_id.clone(), idx);
            records.push(ConversationRecord {
                id: conv_id,
                agent_id: agent_id.to_string(),
                title: if title.is_empty() { None } else { Some(title) },
                created_at: created_at.parse().unwrap_or_else(|_| chrono::Utc::now()),
                updated_at: updated_at.parse().unwrap_or_else(|_| chrono::Utc::now()),
                messages: Vec::new(),
            });
        }

        // Load messages for all conversations in this agent
        for (conv_id, idx) in &conv_map {
            let mut msg_rows = self
                .conn
                .query(
                    "SELECT role, content, timestamp FROM messages
                     WHERE conversation_id = ?1
                     ORDER BY message_index ASC",
                    params![conv_id.as_str()],
                )
                .await?;

            while let Some(row) = msg_rows.next().await? {
                let role_str: String = row.get(0)?;
                let content_blob: Vec<u8> = row.get(1)?;
                let timestamp_str: String = row.get(2)?;

                let content_json = compression::decompress(&content_blob)?;
                let content: Vec<bridge_core::ContentBlock> =
                    serde_json::from_slice(&content_json)?;

                let role: bridge_core::Role =
                    serde_json::from_value(serde_json::Value::String(role_str))
                        .unwrap_or(bridge_core::Role::User);

                let timestamp = timestamp_str.parse().unwrap_or_else(|_| chrono::Utc::now());

                records[*idx].messages.push(Message {
                    role,
                    content,
                    timestamp,
                });
            }
        }

        Ok(records)
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

        self.conn
            .execute(
                "INSERT OR REPLACE INTO messages
                     (conversation_id, message_index, role, content, timestamp)
                 VALUES (?1, ?2, ?3, ?4, ?5)",
                params![
                    conversation_id,
                    message_index as i64,
                    role_str,
                    content_blob,
                    timestamp,
                ],
            )
            .await?;

        // Update conversation timestamp
        let now = chrono::Utc::now().to_rfc3339();
        self.conn
            .execute(
                "UPDATE conversations SET updated_at = ?1 WHERE conversation_id = ?2",
                params![now, conversation_id],
            )
            .await?;

        Ok(())
    }

    async fn replace_messages(
        &self,
        conversation_id: &str,
        messages: &[Message],
    ) -> Result<(), StorageError> {
        // Delete all existing messages
        self.conn
            .execute(
                "DELETE FROM messages WHERE conversation_id = ?1",
                params![conversation_id],
            )
            .await?;

        // Re-insert all messages
        for (idx, msg) in messages.iter().enumerate() {
            let content_json = serde_json::to_vec(&msg.content)?;
            let content_blob = compression::compress(&content_json)?;
            let role_str = serde_json::to_value(&msg.role)?
                .as_str()
                .unwrap_or("user")
                .to_string();
            let timestamp = msg.timestamp.to_rfc3339();

            self.conn
                .execute(
                    "INSERT INTO messages
                         (conversation_id, message_index, role, content, timestamp)
                     VALUES (?1, ?2, ?3, ?4, ?5)",
                    params![
                        conversation_id,
                        idx as i64,
                        role_str,
                        content_blob,
                        timestamp,
                    ],
                )
                .await?;
        }

        let now = chrono::Utc::now().to_rfc3339();
        self.conn
            .execute(
                "UPDATE conversations SET updated_at = ?1 WHERE conversation_id = ?2",
                params![now, conversation_id],
            )
            .await?;

        Ok(())
    }

    // ── Webhook outbox ──────────────────────────────────────

    async fn enqueue_webhook(&self, payload: &WebhookPayload) -> Result<i64, StorageError> {
        let json = serde_json::to_vec(payload)?;
        let blob = compression::compress(&json)?;
        let event_type = serde_json::to_value(&payload.event_type)?
            .as_str()
            .unwrap_or("unknown")
            .to_string();

        self.conn
            .execute(
                "INSERT INTO webhook_outbox (conversation_id, event_type, payload)
                 VALUES (?1, ?2, ?3)",
                params![payload.conversation_id.clone(), event_type, blob],
            )
            .await?;

        Ok(self.conn.last_insert_rowid())
    }

    async fn mark_webhook_delivered(&self, outbox_id: i64) -> Result<(), StorageError> {
        let now = chrono::Utc::now().to_rfc3339();
        self.conn
            .execute(
                "UPDATE webhook_outbox SET delivered_at = ?1, attempts = attempts + 1
                 WHERE id = ?2",
                params![now, outbox_id],
            )
            .await?;
        Ok(())
    }

    async fn load_pending_webhooks(&self) -> Result<Vec<(i64, WebhookPayload)>, StorageError> {
        let mut rows = self
            .conn
            .query(
                "SELECT id, payload FROM webhook_outbox
                 WHERE delivered_at IS NULL
                 ORDER BY id ASC",
                (),
            )
            .await?;

        let mut results = Vec::new();
        while let Some(row) = rows.next().await? {
            let id: i64 = row.get(0)?;
            let blob: Vec<u8> = row.get(1)?;
            let json = compression::decompress(&blob)?;
            match serde_json::from_slice::<WebhookPayload>(&json) {
                Ok(payload) => results.push((id, payload)),
                Err(e) => {
                    error!(outbox_id = id, error = %e, "failed to deserialize webhook payload, skipping");
                }
            }
        }
        Ok(results)
    }

    async fn cleanup_delivered_webhooks(&self, older_than_secs: u64) -> Result<u64, StorageError> {
        let cutoff =
            (chrono::Utc::now() - chrono::Duration::seconds(older_than_secs as i64)).to_rfc3339();
        self.conn
            .execute(
                "DELETE FROM webhook_outbox
                 WHERE delivered_at IS NOT NULL AND delivered_at < ?1",
                params![cutoff],
            )
            .await?;
        Ok(self.conn.changes())
    }

    // ── Metrics ─────────────────────────────────────────────

    async fn save_metrics_snapshot(
        &self,
        agent_id: &str,
        snapshot: &MetricsSnapshot,
    ) -> Result<(), StorageError> {
        let json = serde_json::to_vec(snapshot)?;
        let blob = compression::compress(&json)?;
        self.conn
            .execute(
                "INSERT INTO metrics_snapshots (agent_id, snapshot) VALUES (?1, ?2)",
                params![agent_id, blob],
            )
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
        self.conn
            .execute(
                "INSERT INTO session_store (task_id, agent_id, content, updated_at)
                 VALUES (?1, ?2, ?3, ?4)
                 ON CONFLICT(task_id) DO UPDATE SET
                     content = excluded.content,
                     updated_at = excluded.updated_at",
                params![task_id, agent_id, blob, now],
            )
            .await?;
        Ok(())
    }

    async fn load_sessions(&self, agent_id: &str) -> Result<Vec<(String, Vec<u8>)>, StorageError> {
        let mut rows = self
            .conn
            .query(
                "SELECT task_id, content FROM session_store WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;

        let mut results = Vec::new();
        while let Some(row) = rows.next().await? {
            let task_id: String = row.get(0)?;
            let blob: Vec<u8> = row.get(1)?;
            let json = compression::decompress(&blob)?;
            results.push((task_id, json));
        }
        Ok(results)
    }

    async fn delete_sessions_for_agent(&self, agent_id: &str) -> Result<(), StorageError> {
        self.conn
            .execute(
                "DELETE FROM session_store WHERE agent_id = ?1",
                params![agent_id],
            )
            .await?;
        Ok(())
    }

    // ── Lifecycle ───────────────────────────────────────────

    async fn sync(&self) -> Result<(), StorageError> {
        self.db
            .sync()
            .await
            .map_err(|e| StorageError::Database(format!("sync failed: {e}")))?;
        Ok(())
    }
}
