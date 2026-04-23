use bridge_core::{AgentDefinition, BridgeEvent, Message, MetricsSnapshot};
use tokio::sync::{mpsc, oneshot};

use super::commands::WriteCommand;

/// Clonable, non-blocking handle for sending write commands.
///
/// Every method is fire-and-forget — the caller never blocks on I/O.
#[derive(Clone)]
pub struct StorageHandle {
    tx: mpsc::UnboundedSender<WriteCommand>,
}

impl StorageHandle {
    pub fn new(tx: mpsc::UnboundedSender<WriteCommand>) -> Self {
        Self { tx }
    }

    pub fn save_agent(&self, def: AgentDefinition) {
        let _ = self.tx.send(WriteCommand::SaveAgent(Box::new(def)));
    }

    pub fn delete_agent(&self, id: String) {
        let _ = self.tx.send(WriteCommand::DeleteAgent(id));
    }

    pub fn create_conversation(
        &self,
        agent_id: String,
        conversation_id: String,
        title: Option<String>,
        created_at: chrono::DateTime<chrono::Utc>,
    ) {
        let _ = self.tx.send(WriteCommand::CreateConversation {
            agent_id,
            conversation_id,
            title,
            created_at,
        });
    }

    pub fn delete_conversation(&self, id: String) {
        let _ = self.tx.send(WriteCommand::DeleteConversation(id));
    }

    pub fn append_message(&self, conversation_id: String, message_index: u64, message: Message) {
        let _ = self.tx.send(WriteCommand::AppendMessage {
            conversation_id,
            message_index,
            message,
        });
    }

    pub fn replace_messages(&self, conversation_id: String, messages: Vec<Message>) {
        let _ = self.tx.send(WriteCommand::ReplaceMessages {
            conversation_id,
            messages,
        });
    }

    pub fn enqueue_event(&self, event: BridgeEvent) {
        let _ = self.tx.send(WriteCommand::EnqueueEvent(event));
    }

    pub fn mark_webhook_delivered(&self, event_id: String) {
        let _ = self.tx.send(WriteCommand::MarkWebhookDelivered(event_id));
    }

    pub fn save_metrics_snapshot(&self, agent_id: String, snapshot: MetricsSnapshot) {
        let _ = self
            .tx
            .send(WriteCommand::SaveMetricsSnapshot { agent_id, snapshot });
    }

    pub fn save_session(&self, task_id: String, agent_id: String, history_json: Vec<u8>) {
        let _ = self.tx.send(WriteCommand::SaveSession {
            task_id,
            agent_id,
            history_json,
        });
    }

    pub fn delete_sessions_for_agent(&self, agent_id: String) {
        let _ = self.tx.send(WriteCommand::DeleteSessionsForAgent(agent_id));
    }

    pub fn delete_sessions_by_prefix(&self, prefix: String) {
        let _ = self.tx.send(WriteCommand::DeleteSessionsByPrefix(prefix));
    }

    // ── Immortal conversations ──────────────────────────

    pub fn append_journal_entry(
        &self,
        entry_id: String,
        conversation_id: String,
        chain_index: u32,
        entry_type: String,
        content: String,
        created_at: chrono::DateTime<chrono::Utc>,
    ) {
        let _ = self.tx.send(WriteCommand::AppendJournalEntry {
            entry_id,
            conversation_id,
            chain_index,
            entry_type,
            content,
            created_at,
        });
    }

    pub fn save_chain_link(
        &self,
        conversation_id: String,
        chain_index: u32,
        started_at: chrono::DateTime<chrono::Utc>,
        trigger_token_count: Option<usize>,
        checkpoint_text: Option<String>,
    ) {
        let _ = self.tx.send(WriteCommand::SaveChainLink {
            conversation_id,
            chain_index,
            started_at,
            trigger_token_count,
            checkpoint_text,
        });
    }

    pub fn complete_chain_link(
        &self,
        conversation_id: String,
        chain_index: u32,
        ended_at: chrono::DateTime<chrono::Utc>,
    ) {
        let _ = self.tx.send(WriteCommand::CompleteChainLink {
            conversation_id,
            chain_index,
            ended_at,
        });
    }

    /// Block until all queued writes have been executed.
    pub async fn drain(&self) {
        let (tx, rx) = oneshot::channel();
        let _ = self.tx.send(WriteCommand::Drain(tx));
        let _ = rx.await;
    }

    /// Block until all pending writes have been flushed to the database.
    pub async fn flush(&self) {
        let (tx, rx) = oneshot::channel();
        let _ = self.tx.send(WriteCommand::Flush(tx));
        let _ = rx.await;
    }
}
