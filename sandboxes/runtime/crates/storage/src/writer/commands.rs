use bridge_core::{AgentDefinition, BridgeEvent, Message, MetricsSnapshot};
use tokio::sync::oneshot;

/// Commands sent from the hot path to the background writer.
pub enum WriteCommand {
    SaveAgent(Box<AgentDefinition>),
    DeleteAgent(String),
    CreateConversation {
        agent_id: String,
        conversation_id: String,
        title: Option<String>,
        created_at: chrono::DateTime<chrono::Utc>,
    },
    DeleteConversation(String),
    AppendMessage {
        conversation_id: String,
        message_index: u64,
        message: Message,
    },
    ReplaceMessages {
        conversation_id: String,
        messages: Vec<Message>,
    },
    EnqueueEvent(BridgeEvent),
    MarkWebhookDelivered(String),
    SaveMetricsSnapshot {
        agent_id: String,
        snapshot: MetricsSnapshot,
    },
    SaveSession {
        task_id: String,
        agent_id: String,
        history_json: Vec<u8>,
    },
    DeleteSessionsForAgent(String),
    DeleteSessionsByPrefix(String),
    // ── Immortal conversations ──────────────────────────
    AppendJournalEntry {
        entry_id: String,
        conversation_id: String,
        chain_index: u32,
        entry_type: String,
        content: String,
        created_at: chrono::DateTime<chrono::Utc>,
    },
    SaveChainLink {
        conversation_id: String,
        chain_index: u32,
        started_at: chrono::DateTime<chrono::Utc>,
        trigger_token_count: Option<usize>,
        checkpoint_text: Option<String>,
    },
    CompleteChainLink {
        conversation_id: String,
        chain_index: u32,
        ended_at: chrono::DateTime<chrono::Utc>,
    },
    /// Wait until all pending writes ahead of this command have completed.
    Drain(oneshot::Sender<()>),
    /// Flush all pending writes, then signal the caller.
    Flush(oneshot::Sender<()>),
}
