use async_trait::async_trait;
use dashmap::DashMap;
use llm::{BridgeAgent, SseEvent, ToolCallEmitter};
use rig::completion::Prompt;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::{
    AgentContext, AgentTaskHandle, AgentTaskNotification, AgentTaskResult, SubAgentRunner,
    AGENT_CONTEXT,
};
use tracing::debug;

/// Timeout for a foreground subagent chat call.
const FOREGROUND_TIMEOUT: Duration = Duration::from_secs(120);
/// Timeout for a background subagent chat call.
const BACKGROUND_TIMEOUT: Duration = Duration::from_secs(300);

/// A pre-built subagent entry ready for invocation.
pub struct SubAgentEntry {
    pub name: String,
    pub description: String,
    pub agent: Arc<BridgeAgent>,
}

/// Session store for subagent history persistence and resumption.
///
/// Keyed by task_id, stores the conversation history for each subagent session.
pub struct AgentSessionStore {
    sessions: DashMap<String, Vec<rig::message::Message>>,
}

impl AgentSessionStore {
    pub fn new() -> Self {
        Self {
            sessions: DashMap::new(),
        }
    }

    /// Get existing history or create empty vec for a task_id.
    pub fn get_or_create(&self, task_id: &str) -> Vec<rig::message::Message> {
        self.sessions
            .get(task_id)
            .map(|h| h.value().clone())
            .unwrap_or_default()
    }

    /// Save history for a task_id.
    pub fn save(&self, task_id: String, history: Vec<rig::message::Message>) {
        self.sessions.insert(task_id, history);
    }

    /// Remove all sessions with keys starting with the given prefix.
    pub fn remove_by_prefix(&self, prefix: &str) {
        let keys_to_remove: Vec<String> = self
            .sessions
            .iter()
            .filter(|entry| entry.key().starts_with(prefix))
            .map(|entry| entry.key().clone())
            .collect();
        for key in keys_to_remove {
            self.sessions.remove(&key);
        }
    }
}

/// Runtime implementation of [`SubAgentRunner`] that uses rig-core agents.
pub struct ConversationSubAgentRunner {
    subagents: Arc<DashMap<String, SubAgentEntry>>,
    session_store: Arc<AgentSessionStore>,
    notification_tx: mpsc::Sender<AgentTaskNotification>,
    cancel: CancellationToken,
    sse_tx: mpsc::Sender<SseEvent>,
    conversation_id: String,
    depth: usize,
    max_depth: usize,
}

impl ConversationSubAgentRunner {
    pub fn new(
        subagents: Arc<DashMap<String, SubAgentEntry>>,
        session_store: Arc<AgentSessionStore>,
        notification_tx: mpsc::Sender<AgentTaskNotification>,
        cancel: CancellationToken,
        sse_tx: mpsc::Sender<SseEvent>,
        conversation_id: String,
        depth: usize,
        max_depth: usize,
    ) -> Self {
        Self {
            subagents,
            session_store,
            notification_tx,
            cancel,
            sse_tx,
            conversation_id,
            depth,
            max_depth,
        }
    }

    /// Generate a task_id scoped to this conversation.
    fn generate_task_id(&self) -> String {
        format!("{}-{}", self.conversation_id, uuid::Uuid::new_v4())
    }
}

#[async_trait]
impl SubAgentRunner for ConversationSubAgentRunner {
    fn available_subagents(&self) -> Vec<(String, String)> {
        self.subagents
            .iter()
            .map(|entry| {
                let e = entry.value();
                (e.name.clone(), e.description.clone())
            })
            .collect()
    }

    async fn run_foreground(
        &self,
        subagent: &str,
        prompt: &str,
        task_id: Option<&str>,
    ) -> Result<AgentTaskResult, String> {
        let entry = self
            .subagents
            .get(subagent)
            .ok_or_else(|| format!("Subagent '{}' not found", subagent))?;

        let agent = entry.agent.clone();
        let task_id = task_id
            .map(|id| id.to_string())
            .unwrap_or_else(|| self.generate_task_id());

        let mut history = self.session_store.get_or_create(&task_id);
        let emitter = ToolCallEmitter {
            sse_tx: self.sse_tx.clone(),
        };

        let prompt_owned = prompt.to_string();
        let cancel = self.cancel.clone();

        let result = tokio::select! {
            _ = cancel.cancelled() => {
                Err("Subagent cancelled".to_string())
            }
            result = async {
                tokio::time::timeout(
                    FOREGROUND_TIMEOUT,
                    agent.prompt(&prompt_owned).with_history(&mut history).with_hook(emitter),
                ).await
            } => {
                match result {
                    Err(_) => Err(format!("Subagent timed out after {}s", FOREGROUND_TIMEOUT.as_secs())),
                    Ok(Ok(output)) => Ok(output),
                    Ok(Err(e)) => Err(format!("Subagent error: {}", e)),
                }
            }
        };

        // Save history regardless of outcome (for resumption)
        self.session_store.save(task_id.clone(), history);

        match result {
            Ok(output) => Ok(AgentTaskResult {
                task_id,
                output,
            }),
            Err(e) => Err(e),
        }
    }

    async fn run_background(
        &self,
        subagent: &str,
        prompt: &str,
        description: &str,
    ) -> Result<AgentTaskHandle, String> {
        let entry = self
            .subagents
            .get(subagent)
            .ok_or_else(|| format!("Subagent '{}' not found", subagent))?;

        let agent = entry.agent.clone();
        let task_id = self.generate_task_id();
        let task_id_clone = task_id.clone();

        let history = self.session_store.get_or_create(&task_id);
        let session_store = self.session_store.clone();
        let notification_tx = self.notification_tx.clone();
        let cancel = self.cancel.clone();
        let sse_tx = self.sse_tx.clone();
        let prompt_owned = prompt.to_string();
        let description_owned = description.to_string();
        let subagents = self.subagents.clone();
        let conversation_id = self.conversation_id.clone();
        let depth = self.depth;
        let max_depth = self.max_depth;

        tokio::spawn(async move {
            // Build nested AgentContext for the background task
            let nested_runner = Arc::new(ConversationSubAgentRunner::new(
                subagents,
                session_store.clone(),
                notification_tx.clone(),
                cancel.clone(),
                sse_tx.clone(),
                conversation_id,
                depth + 1,
                max_depth,
            ));
            let nested_ctx = AgentContext {
                runner: nested_runner,
                notification_tx: notification_tx.clone(),
                depth: depth + 1,
                max_depth,
            };

            let mut history = history;
            let emitter = ToolCallEmitter { sse_tx };

            let result = AGENT_CONTEXT
                .scope(nested_ctx, async {
                    tokio::select! {
                        _ = cancel.cancelled() => {
                            Err("Background subagent cancelled".to_string())
                        }
                        result = async {
                            tokio::time::timeout(
                                BACKGROUND_TIMEOUT,
                                agent.prompt(&prompt_owned).with_history(&mut history).with_hook(emitter),
                            ).await
                        } => {
                            match result {
                                Err(_) => Err(format!("Background subagent timed out after {}s", BACKGROUND_TIMEOUT.as_secs())),
                                Ok(Ok(output)) => Ok(output),
                                Ok(Err(e)) => Err(format!("Background subagent error: {}", e)),
                            }
                        }
                    }
                })
                .await;

            // Save history
            session_store.save(task_id_clone.clone(), history);

            // Send notification
            let output = match result {
                Ok(output) => Ok(output),
                Err(e) => Err(e),
            };

            let notification = AgentTaskNotification {
                task_id: task_id_clone.clone(),
                description: description_owned,
                output,
            };

            if let Err(_) = notification_tx.send(notification).await {
                debug!(
                    task_id = %task_id_clone,
                    "notification channel closed, conversation likely ended"
                );
            }
        });

        Ok(AgentTaskHandle { task_id })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_session_store_get_or_create_empty() {
        let store = AgentSessionStore::new();
        let history = store.get_or_create("task-1");
        assert!(history.is_empty());
    }

    #[test]
    fn test_session_store_save_and_retrieve() {
        let store = AgentSessionStore::new();
        let history = vec![rig::message::Message::user("hello")];
        store.save("task-1".to_string(), history.clone());
        let retrieved = store.get_or_create("task-1");
        assert_eq!(retrieved.len(), 1);
    }

    #[test]
    fn test_session_store_remove_by_prefix() {
        let store = AgentSessionStore::new();
        store.save(
            "conv-123-task-1".to_string(),
            vec![rig::message::Message::user("a")],
        );
        store.save(
            "conv-123-task-2".to_string(),
            vec![rig::message::Message::user("b")],
        );
        store.save(
            "conv-456-task-1".to_string(),
            vec![rig::message::Message::user("c")],
        );

        store.remove_by_prefix("conv-123");

        assert!(store.get_or_create("conv-123-task-1").is_empty());
        assert!(store.get_or_create("conv-123-task-2").is_empty());
        assert_eq!(store.get_or_create("conv-456-task-1").len(), 1);
    }
}
