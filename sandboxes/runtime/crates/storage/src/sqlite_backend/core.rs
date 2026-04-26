use async_trait::async_trait;
use bridge_core::{AgentDefinition, BridgeEvent, ConversationRecord, Message, MetricsSnapshot};
use tokio_rusqlite::Connection;
use tracing::info;

use super::{
    agents, artifacts, chain, conversations, events, journal, messages, metrics, sessions,
};
use crate::backend::{ArtifactUploadRow, ChainLinkRow, JournalEntryRow, StorageBackend};
use crate::config::StorageConfig;
use crate::error::StorageError;
use crate::schema;

/// SQLite-backed storage implementation.
///
/// Uses `tokio-rusqlite` for async access to a local SQLite file.
/// All data lives on disk — no remote sync or replication.
pub struct SqliteBackend {
    pub(super) conn: Connection,
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
        agents::save_agent(&self.conn, definition).await
    }

    async fn delete_agent(&self, agent_id: &str) -> Result<(), StorageError> {
        agents::delete_agent(&self.conn, agent_id).await
    }

    async fn load_all_agents(&self) -> Result<Vec<AgentDefinition>, StorageError> {
        agents::load_all_agents(&self.conn).await
    }

    // ── Conversations ───────────────────────────────────────

    async fn create_conversation(
        &self,
        agent_id: &str,
        conversation_id: &str,
        title: Option<&str>,
        created_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError> {
        conversations::create_conversation(&self.conn, agent_id, conversation_id, title, created_at)
            .await
    }

    async fn delete_conversation(&self, conversation_id: &str) -> Result<(), StorageError> {
        conversations::delete_conversation(&self.conn, conversation_id).await
    }

    async fn load_conversations(
        &self,
        agent_id: &str,
    ) -> Result<Vec<ConversationRecord>, StorageError> {
        conversations::load_conversations(&self.conn, agent_id).await
    }

    // ── Messages ────────────────────────────────────────────

    async fn append_message(
        &self,
        conversation_id: &str,
        message_index: u64,
        message: &Message,
    ) -> Result<(), StorageError> {
        messages::append_message(&self.conn, conversation_id, message_index, message).await
    }

    async fn replace_messages(
        &self,
        conversation_id: &str,
        msgs: &[Message],
    ) -> Result<(), StorageError> {
        messages::replace_messages(&self.conn, conversation_id, msgs).await
    }

    // ── Event outbox ───────────────────────────────────────

    async fn enqueue_event(&self, event: &BridgeEvent) -> Result<String, StorageError> {
        events::enqueue_event(&self.conn, event).await
    }

    async fn mark_webhook_delivered(&self, event_id: &str) -> Result<(), StorageError> {
        events::mark_webhook_delivered(&self.conn, event_id).await
    }

    async fn load_pending_events(&self) -> Result<Vec<BridgeEvent>, StorageError> {
        events::load_pending_events(&self.conn).await
    }

    async fn cleanup_delivered_events(&self, older_than_secs: u64) -> Result<u64, StorageError> {
        events::cleanup_delivered_events(&self.conn, older_than_secs).await
    }

    async fn load_events_since(
        &self,
        after_sequence: u64,
        limit: u32,
    ) -> Result<Vec<BridgeEvent>, StorageError> {
        events::load_events_since(&self.conn, after_sequence, limit).await
    }

    // ── Metrics ─────────────────────────────────────────────

    async fn save_metrics_snapshot(
        &self,
        agent_id: &str,
        snapshot: &MetricsSnapshot,
    ) -> Result<(), StorageError> {
        metrics::save_metrics_snapshot(&self.conn, agent_id, snapshot).await
    }

    // ── Session store ───────────────────────────────────────

    async fn save_session(
        &self,
        task_id: &str,
        agent_id: &str,
        history_json: &[u8],
    ) -> Result<(), StorageError> {
        sessions::save_session(&self.conn, task_id, agent_id, history_json).await
    }

    async fn load_sessions(&self, agent_id: &str) -> Result<Vec<(String, Vec<u8>)>, StorageError> {
        sessions::load_sessions(&self.conn, agent_id).await
    }

    async fn delete_sessions_for_agent(&self, agent_id: &str) -> Result<(), StorageError> {
        sessions::delete_sessions_for_agent(&self.conn, agent_id).await
    }

    async fn delete_sessions_by_prefix(&self, prefix: &str) -> Result<(), StorageError> {
        sessions::delete_sessions_by_prefix(&self.conn, prefix).await
    }

    // ── Journal (immortal conversations) ──────────────────

    async fn append_journal_entry(
        &self,
        entry_id: &str,
        conversation_id: &str,
        chain_index: u32,
        entry_type: &str,
        content: &str,
        created_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError> {
        journal::append_journal_entry(
            &self.conn,
            entry_id,
            conversation_id,
            chain_index,
            entry_type,
            content,
            created_at,
        )
        .await
    }

    async fn load_journal(
        &self,
        conversation_id: &str,
    ) -> Result<Vec<JournalEntryRow>, StorageError> {
        journal::load_journal(&self.conn, conversation_id).await
    }

    // ── Chain links (immortal conversations) ────────────────

    async fn save_chain_link(
        &self,
        conversation_id: &str,
        chain_index: u32,
        started_at: chrono::DateTime<chrono::Utc>,
        trigger_token_count: Option<usize>,
        checkpoint_text: Option<&str>,
    ) -> Result<(), StorageError> {
        chain::save_chain_link(
            &self.conn,
            conversation_id,
            chain_index,
            started_at,
            trigger_token_count,
            checkpoint_text,
        )
        .await
    }

    async fn complete_chain_link(
        &self,
        conversation_id: &str,
        chain_index: u32,
        ended_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError> {
        chain::complete_chain_link(&self.conn, conversation_id, chain_index, ended_at).await
    }

    async fn load_chain_links(
        &self,
        conversation_id: &str,
    ) -> Result<Vec<ChainLinkRow>, StorageError> {
        chain::load_chain_links(&self.conn, conversation_id).await
    }

    // ── Artifact uploads ────────────────────────────────────

    async fn get_artifact_upload(
        &self,
        idempotency_key: &str,
    ) -> Result<Option<ArtifactUploadRow>, StorageError> {
        artifacts::get_artifact_upload(&self.conn, idempotency_key).await
    }

    async fn upsert_artifact_upload_in_progress(
        &self,
        row: ArtifactUploadRow,
    ) -> Result<(), StorageError> {
        artifacts::upsert_in_progress(&self.conn, row).await
    }

    async fn update_artifact_upload_offset(
        &self,
        idempotency_key: &str,
        bytes_sent: u64,
    ) -> Result<(), StorageError> {
        artifacts::update_offset(&self.conn, idempotency_key, bytes_sent).await
    }

    async fn mark_artifact_upload_completed(
        &self,
        idempotency_key: &str,
        bytes_sent: u64,
        response_json: &str,
    ) -> Result<(), StorageError> {
        artifacts::mark_completed(&self.conn, idempotency_key, bytes_sent, response_json).await
    }

    async fn mark_artifact_upload_failed(
        &self,
        idempotency_key: &str,
        error: &str,
    ) -> Result<(), StorageError> {
        artifacts::mark_failed(&self.conn, idempotency_key, error).await
    }

    // ── Lifecycle ───────────────────────────────────────────

    async fn sync(&self) -> Result<(), StorageError> {
        // No remote sync needed for local SQLite — this is a no-op.
        Ok(())
    }
}
