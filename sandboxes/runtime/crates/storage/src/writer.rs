use bridge_core::{AgentDefinition, Message, MetricsSnapshot, WebhookPayload};
use std::sync::Arc;
use tokio::sync::{mpsc, oneshot};
use tracing::{debug, error};

use crate::backend::StorageBackend;

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
    EnqueueWebhook(WebhookPayload),
    MarkWebhookDelivered(i64),
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
    /// Flush all pending writes, then signal the caller.
    Flush(oneshot::Sender<()>),
}

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

    pub fn enqueue_webhook(&self, payload: WebhookPayload) {
        let _ = self.tx.send(WriteCommand::EnqueueWebhook(payload));
    }

    pub fn mark_webhook_delivered(&self, outbox_id: i64) {
        let _ = self.tx.send(WriteCommand::MarkWebhookDelivered(outbox_id));
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

    /// Block until all pending writes have been flushed to the database.
    pub async fn flush(&self) {
        let (tx, rx) = oneshot::channel();
        let _ = self.tx.send(WriteCommand::Flush(tx));
        let _ = rx.await;
    }
}

/// Background writer loop. Receives commands and executes them against the backend.
///
/// Batches commands when multiple are available to reduce per-item overhead.
pub async fn run_writer(
    mut rx: mpsc::UnboundedReceiver<WriteCommand>,
    backend: Arc<dyn StorageBackend>,
) {
    loop {
        let cmd = match rx.recv().await {
            Some(cmd) => cmd,
            None => break, // channel closed
        };

        // Drain any additional queued commands for batching
        let mut batch = vec![cmd];
        while batch.len() < 100 {
            match rx.try_recv() {
                Ok(cmd) => batch.push(cmd),
                Err(_) => break,
            }
        }

        let count = batch.len();
        if count > 1 {
            debug!(commands = count, "processing write batch");
        }

        for cmd in batch {
            process_command(&backend, cmd).await;
        }
    }

    // Drain remaining commands on shutdown
    rx.close();
    while let Some(cmd) = rx.recv().await {
        process_command(&backend, cmd).await;
    }

    // Final sync
    if let Err(e) = backend.sync().await {
        error!(error = %e, "final sync failed during writer shutdown");
    }
}

async fn process_command(backend: &Arc<dyn StorageBackend>, cmd: WriteCommand) {
    match cmd {
        WriteCommand::SaveAgent(def) => {
            if let Err(e) = backend.save_agent(def.as_ref()).await {
                error!(agent_id = %def.id, error = %e, "storage: save_agent failed");
            }
        }
        WriteCommand::DeleteAgent(id) => {
            if let Err(e) = backend.delete_agent(&id).await {
                error!(agent_id = %id, error = %e, "storage: delete_agent failed");
            }
        }
        WriteCommand::CreateConversation {
            agent_id,
            conversation_id,
            title,
            created_at,
        } => {
            if let Err(e) = backend
                .create_conversation(&agent_id, &conversation_id, title.as_deref(), created_at)
                .await
            {
                error!(conversation_id = %conversation_id, error = %e, "storage: create_conversation failed");
            }
        }
        WriteCommand::DeleteConversation(id) => {
            if let Err(e) = backend.delete_conversation(&id).await {
                error!(conversation_id = %id, error = %e, "storage: delete_conversation failed");
            }
        }
        WriteCommand::AppendMessage {
            conversation_id,
            message_index,
            message,
        } => {
            if let Err(e) = backend
                .append_message(&conversation_id, message_index, &message)
                .await
            {
                error!(conversation_id = %conversation_id, error = %e, "storage: append_message failed");
            }
        }
        WriteCommand::ReplaceMessages {
            conversation_id,
            messages,
        } => {
            if let Err(e) = backend.replace_messages(&conversation_id, &messages).await {
                error!(conversation_id = %conversation_id, error = %e, "storage: replace_messages failed");
            }
        }
        WriteCommand::EnqueueWebhook(payload) => {
            if let Err(e) = backend.enqueue_webhook(&payload).await {
                error!(error = %e, "storage: enqueue_webhook failed");
            }
        }
        WriteCommand::MarkWebhookDelivered(id) => {
            if let Err(e) = backend.mark_webhook_delivered(id).await {
                error!(outbox_id = id, error = %e, "storage: mark_webhook_delivered failed");
            }
        }
        WriteCommand::SaveMetricsSnapshot { agent_id, snapshot } => {
            if let Err(e) = backend.save_metrics_snapshot(&agent_id, &snapshot).await {
                error!(agent_id = %agent_id, error = %e, "storage: save_metrics_snapshot failed");
            }
        }
        WriteCommand::SaveSession {
            task_id,
            agent_id,
            history_json,
        } => {
            if let Err(e) = backend
                .save_session(&task_id, &agent_id, &history_json)
                .await
            {
                error!(task_id = %task_id, error = %e, "storage: save_session failed");
            }
        }
        WriteCommand::DeleteSessionsForAgent(agent_id) => {
            if let Err(e) = backend.delete_sessions_for_agent(&agent_id).await {
                error!(agent_id = %agent_id, error = %e, "storage: delete_sessions_for_agent failed");
            }
        }
        WriteCommand::Flush(reply) => {
            if let Err(e) = backend.sync().await {
                error!(error = %e, "storage: flush sync failed");
            }
            let _ = reply.send(());
        }
    }
}
