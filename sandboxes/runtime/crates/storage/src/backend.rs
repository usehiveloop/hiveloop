use async_trait::async_trait;
use bridge_core::{AgentDefinition, BridgeEvent, ConversationRecord, Message, MetricsSnapshot};

use crate::error::StorageError;

/// Trait defining the persistence interface.
///
/// All methods are async. Implementations must be `Send + Sync + 'static`
/// for use across tokio tasks.
#[async_trait]
pub trait StorageBackend: Send + Sync + 'static {
    // ── Agents ──────────────────────────────────────────────

    /// Persist an agent definition (upsert).
    async fn save_agent(&self, definition: &AgentDefinition) -> Result<(), StorageError>;

    /// Remove an agent and all its associated data (CASCADE).
    async fn delete_agent(&self, agent_id: &str) -> Result<(), StorageError>;

    /// Load all stored agent definitions.
    async fn load_all_agents(&self) -> Result<Vec<AgentDefinition>, StorageError>;

    // ── Conversations ───────────────────────────────────────

    /// Create a conversation metadata row.
    async fn create_conversation(
        &self,
        agent_id: &str,
        conversation_id: &str,
        title: Option<&str>,
        created_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError>;

    /// Delete a conversation and all its messages.
    async fn delete_conversation(&self, conversation_id: &str) -> Result<(), StorageError>;

    /// Load all conversations for an agent, including full message history.
    async fn load_conversations(
        &self,
        agent_id: &str,
    ) -> Result<Vec<ConversationRecord>, StorageError>;

    // ── Messages ────────────────────────────────────────────

    /// Append a single message to a conversation.
    async fn append_message(
        &self,
        conversation_id: &str,
        message_index: u64,
        message: &Message,
    ) -> Result<(), StorageError>;

    /// Replace all messages in a conversation (e.g. after compaction).
    async fn replace_messages(
        &self,
        conversation_id: &str,
        messages: &[Message],
    ) -> Result<(), StorageError>;

    // ── Event outbox ───────────────────────────────────────

    /// Insert a BridgeEvent into the outbox (with sequence_number).
    async fn enqueue_event(&self, event: &BridgeEvent) -> Result<String, StorageError>;

    /// Mark an event as delivered.
    async fn mark_webhook_delivered(&self, event_id: &str) -> Result<(), StorageError>;

    /// Load events with sequence_number > `after_sequence`, up to `limit`.
    async fn load_events_since(
        &self,
        after_sequence: u64,
        limit: u32,
    ) -> Result<Vec<BridgeEvent>, StorageError>;

    /// Load all undelivered events for replay after restart.
    async fn load_pending_events(&self) -> Result<Vec<BridgeEvent>, StorageError>;

    /// Delete delivered events older than the given age.
    async fn cleanup_delivered_events(&self, older_than_secs: u64) -> Result<u64, StorageError>;

    // ── Metrics ─────────────────────────────────────────────

    /// Persist a metrics snapshot.
    async fn save_metrics_snapshot(
        &self,
        agent_id: &str,
        snapshot: &MetricsSnapshot,
    ) -> Result<(), StorageError>;

    // ── Session store ───────────────────────────────────────

    /// Save subagent session history (pre-serialised JSON, will be compressed).
    async fn save_session(
        &self,
        task_id: &str,
        agent_id: &str,
        history_json: &[u8],
    ) -> Result<(), StorageError>;

    /// Load all sessions for an agent. Returns `(task_id, decompressed_json)`.
    async fn load_sessions(&self, agent_id: &str) -> Result<Vec<(String, Vec<u8>)>, StorageError>;

    /// Delete all sessions for an agent.
    async fn delete_sessions_for_agent(&self, agent_id: &str) -> Result<(), StorageError>;

    /// Delete all sessions whose task ids start with the given prefix.
    async fn delete_sessions_by_prefix(&self, prefix: &str) -> Result<(), StorageError>;

    // ── Journal (immortal conversations) ──────────────────

    /// Append a journal entry for an immortal conversation.
    async fn append_journal_entry(
        &self,
        entry_id: &str,
        conversation_id: &str,
        chain_index: u32,
        entry_type: &str,
        content: &str,
        created_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError>;

    /// Load all journal entries for a conversation, ordered by creation time.
    async fn load_journal(
        &self,
        conversation_id: &str,
    ) -> Result<Vec<JournalEntryRow>, StorageError>;

    // ── Chain links (immortal conversations) ────────────────

    /// Save a chain link record when a conversation chains to a new context.
    async fn save_chain_link(
        &self,
        conversation_id: &str,
        chain_index: u32,
        started_at: chrono::DateTime<chrono::Utc>,
        trigger_token_count: Option<usize>,
        checkpoint_text: Option<&str>,
    ) -> Result<(), StorageError>;

    /// Mark a chain link as completed.
    async fn complete_chain_link(
        &self,
        conversation_id: &str,
        chain_index: u32,
        ended_at: chrono::DateTime<chrono::Utc>,
    ) -> Result<(), StorageError>;

    /// Load all chain links for a conversation, ordered by chain_index.
    async fn load_chain_links(
        &self,
        conversation_id: &str,
    ) -> Result<Vec<ChainLinkRow>, StorageError>;

    // ── Artifact uploads ────────────────────────────────────

    /// Look up an in-progress or completed artifact upload by idempotency key.
    async fn get_artifact_upload(
        &self,
        idempotency_key: &str,
    ) -> Result<Option<ArtifactUploadRow>, StorageError>;

    /// Insert (or refresh) an in-progress upload row. The supplied
    /// `bytes_sent` is treated as a floor — callers may seed it from a
    /// fresh creation (`0`) or from a resumed `HEAD` response.
    async fn upsert_artifact_upload_in_progress(
        &self,
        row: ArtifactUploadRow,
    ) -> Result<(), StorageError>;

    /// Persist a new offset after a successful chunk PATCH.
    async fn update_artifact_upload_offset(
        &self,
        idempotency_key: &str,
        bytes_sent: u64,
    ) -> Result<(), StorageError>;

    /// Mark an upload as completed and cache the control plane response.
    async fn mark_artifact_upload_completed(
        &self,
        idempotency_key: &str,
        bytes_sent: u64,
        response_json: &str,
    ) -> Result<(), StorageError>;

    /// Mark an upload as failed with a terminal error message.
    async fn mark_artifact_upload_failed(
        &self,
        idempotency_key: &str,
        error: &str,
    ) -> Result<(), StorageError>;

    // ── Lifecycle ───────────────────────────────────────────

    /// Force a sync with the remote replica.
    async fn sync(&self) -> Result<(), StorageError>;
}

/// A journal entry row as returned from storage.
#[derive(Debug, Clone)]
pub struct JournalEntryRow {
    pub id: String,
    pub conversation_id: String,
    pub chain_index: u32,
    pub entry_type: String,
    pub content: String,
    pub created_at: String,
}

/// A row from the `artifact_uploads` table.
#[derive(Debug, Clone)]
pub struct ArtifactUploadRow {
    pub idempotency_key: String,
    pub agent_id: String,
    pub conversation_id: String,
    pub location: String,
    pub total_size: u64,
    pub file_sha256: String,
    pub bytes_sent: u64,
    pub status: String,
    pub response_json: Option<String>,
    pub last_error: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

/// A chain link row as returned from storage.
#[derive(Debug, Clone)]
pub struct ChainLinkRow {
    pub conversation_id: String,
    pub chain_index: u32,
    pub started_at: String,
    pub ended_at: Option<String>,
    pub trigger_token_count: Option<usize>,
    pub checkpoint_text: Option<String>,
}
