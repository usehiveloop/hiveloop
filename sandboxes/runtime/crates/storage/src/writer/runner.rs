use std::sync::Arc;

use tokio::sync::mpsc;
use tracing::{debug, error};

use super::commands::WriteCommand;
use crate::backend::StorageBackend;

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
            None => break,
        };

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

    rx.close();
    while let Some(cmd) = rx.recv().await {
        process_command(&backend, cmd).await;
    }

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
        WriteCommand::EnqueueEvent(event) => {
            if let Err(e) = backend.enqueue_event(&event).await {
                error!(event_id = %event.event_id, error = %e, "storage: enqueue_event failed");
            }
        }
        WriteCommand::MarkWebhookDelivered(event_id) => {
            if let Err(e) = backend.mark_webhook_delivered(&event_id).await {
                error!(event_id = %event_id, error = %e, "storage: mark_webhook_delivered failed");
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
        WriteCommand::DeleteSessionsByPrefix(prefix) => {
            if let Err(e) = backend.delete_sessions_by_prefix(&prefix).await {
                error!(prefix = %prefix, error = %e, "storage: delete_sessions_by_prefix failed");
            }
        }
        WriteCommand::Drain(reply) => {
            let _ = reply.send(());
        }
        WriteCommand::Flush(reply) => {
            if let Err(e) = backend.sync().await {
                error!(error = %e, "storage: flush sync failed");
            }
            let _ = reply.send(());
        }
    }
}
