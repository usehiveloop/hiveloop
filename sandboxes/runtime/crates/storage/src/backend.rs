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

    // ── Lifecycle ───────────────────────────────────────────

    /// Force a sync with the remote replica.
    async fn sync(&self) -> Result<(), StorageError>;
}
