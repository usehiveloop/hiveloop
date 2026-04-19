use async_trait::async_trait;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use dashmap::DashMap;
use llm::{BridgeAgent, ToolCallEmitter};
use std::sync::Arc;
use std::time::Duration;
use storage::StorageHandle;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::{
    AgentContext, AgentTaskHandle, AgentTaskNotification, AgentTaskResult, SubAgentRunner,
    TaskBudget, AGENT_CONTEXT,
};
use tracing::{debug, warn};
use webhooks::EventBus;

/// Resolve (foreground, background) subagent timeouts from an `AgentConfig`,
/// falling back to [`bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS`].
pub fn resolve_subagent_timeouts(
    config: &bridge_core::agent::AgentConfig,
) -> (Duration, Duration) {
    let default_secs = bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS;
    let fg = config
        .subagent_timeout_foreground_secs
        .unwrap_or(default_secs);
    let bg = config
        .subagent_timeout_background_secs
        .unwrap_or(default_secs);
    (Duration::from_secs(fg), Duration::from_secs(bg))
}

/// A pre-built subagent entry ready for invocation.
pub struct SubAgentEntry {
    pub name: String,
    pub description: String,
    pub agent: Arc<BridgeAgent>,
    /// Tool names and descriptions registered for this subagent at build time.
    pub registered_tools: Vec<(String, String)>,
    /// Wall-clock timeout for foreground invocations of this subagent,
    /// resolved from its `AgentConfig.subagent_timeout_foreground_secs`
    /// (falling back to [`bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS`]).
    pub foreground_timeout: Duration,
    /// Wall-clock timeout for background invocations of this subagent,
    /// resolved from its `AgentConfig.subagent_timeout_background_secs`
    /// (falling back to [`bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS`]).
    pub background_timeout: Duration,
}

/// Session store for subagent history persistence and resumption.
///
/// Uses a primary store (task_id -> history) and a secondary index
/// (conversation_id -> task_ids) for O(k) cleanup instead of O(n) scans.
pub struct AgentSessionStore {
    /// Primary store: task_id -> history
    sessions: DashMap<String, Vec<rig::message::Message>>,
    /// Secondary index: conversation_id prefix -> set of task_ids
    conv_index: DashMap<String, Vec<String>>,
    agent_id: String,
    storage: Option<StorageHandle>,
}

impl Default for AgentSessionStore {
    fn default() -> Self {
        Self::new(String::new(), None)
    }
}

impl AgentSessionStore {
    pub fn new(agent_id: String, storage: Option<StorageHandle>) -> Self {
        Self {
            sessions: DashMap::new(),
            conv_index: DashMap::new(),
            agent_id,
            storage,
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
        // Maintain secondary index: extract conversation_id from task_id
        if let Some(conv_id) = extract_conversation_id(&task_id) {
            self.conv_index
                .entry(conv_id)
                .or_default()
                .push(task_id.clone());
        }
        self.sessions.insert(task_id.clone(), history.clone());

        if let Some(storage) = &self.storage {
            match serde_json::to_vec(&history) {
                Ok(history_json) => {
                    storage.save_session(task_id, self.agent_id.clone(), history_json);
                }
                Err(e) => {
                    warn!(error = %e, "failed to serialize session history for persistence");
                }
            }
        }
    }

    /// Restore history for a task_id without persisting again.
    pub fn restore(&self, task_id: String, history: Vec<rig::message::Message>) {
        if let Some(conv_id) = extract_conversation_id(&task_id) {
            self.conv_index
                .entry(conv_id)
                .or_default()
                .push(task_id.clone());
        }
        self.sessions.insert(task_id, history);
    }

    /// Remove all sessions belonging to a conversation.
    ///
    /// Uses the secondary index for O(k) removal where k is the number of
    /// sessions for this conversation, instead of scanning all entries.
    pub fn remove_by_prefix(&self, prefix: &str) {
        // Fast path: use the index
        if let Some((_, task_ids)) = self.conv_index.remove(prefix) {
            for task_id in &task_ids {
                self.sessions.remove(task_id);
            }
            if let Some(storage) = &self.storage {
                storage.delete_sessions_by_prefix(prefix.to_string());
            }
            return;
        }

        // Fallback: prefix scan (for task_ids that predate the index)
        let keys_to_remove: Vec<String> = self
            .sessions
            .iter()
            .filter(|entry| entry.key().starts_with(prefix))
            .map(|entry| entry.key().clone())
            .collect();
        for key in keys_to_remove {
            self.sessions.remove(&key);
        }

        if let Some(storage) = &self.storage {
            storage.delete_sessions_by_prefix(prefix.to_string());
        }
    }

    /// Returns the number of stored sessions.
    pub fn len(&self) -> usize {
        self.sessions.len()
    }

    /// Returns true if the store is empty.
    pub fn is_empty(&self) -> bool {
        self.sessions.is_empty()
    }
}

/// Extract the conversation ID (first UUID) from a task_id of the form
/// "{conv_uuid}-{task_uuid}". Returns None if the format doesn't match.
fn extract_conversation_id(task_id: &str) -> Option<String> {
    // UUID v4 is 36 chars: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    // Task IDs are formatted as: "{conv_id}-{uuid}" (see generate_task_id)
    if task_id.len() > 37 && task_id.as_bytes().get(36) == Some(&b'-') {
        Some(task_id[..36].to_string())
    } else {
        None
    }
}

/// Runtime implementation of [`SubAgentRunner`] that uses rig-core agents.
pub struct ConversationSubAgentRunner {
    subagents: Arc<DashMap<String, SubAgentEntry>>,
    session_store: Arc<AgentSessionStore>,
    notification_tx: mpsc::Sender<AgentTaskNotification>,
    cancel: CancellationToken,
    event_bus: Arc<EventBus>,
    conversation_id: String,
    depth: usize,
    max_depth: usize,
    compaction_config: Option<bridge_core::agent::CompactionConfig>,
    task_budget: Arc<TaskBudget>,
    metrics: Arc<bridge_core::AgentMetrics>,
    /// Agent ID for event payloads.
    agent_id: String,
}

impl ConversationSubAgentRunner {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        subagents: Arc<DashMap<String, SubAgentEntry>>,
        session_store: Arc<AgentSessionStore>,
        notification_tx: mpsc::Sender<AgentTaskNotification>,
        cancel: CancellationToken,
        event_bus: Arc<EventBus>,
        conversation_id: String,
        depth: usize,
        max_depth: usize,
        metrics: Arc<bridge_core::AgentMetrics>,
    ) -> Self {
        Self {
            subagents,
            session_store,
            notification_tx,
            cancel,
            event_bus,
            conversation_id,
            depth,
            max_depth,
            compaction_config: None,
            task_budget: Arc::new(TaskBudget::new(50)),
            metrics,
            agent_id: String::new(),
        }
    }

    /// Set the agent ID for subagent trace events.
    pub fn with_agent_id(mut self, agent_id: String) -> Self {
        self.agent_id = agent_id;
        self
    }

    /// Set the compaction configuration for subagent sessions.
    pub fn with_compaction(mut self, config: Option<bridge_core::agent::CompactionConfig>) -> Self {
        self.compaction_config = config;
        self
    }

    /// Set the task budget for limiting subagent spawning.
    pub fn with_task_budget(mut self, budget: Arc<TaskBudget>) -> Self {
        self.task_budget = budget;
        self
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
            .filter(|entry| entry.key() != tools::self_agent::SELF_AGENT_NAME)
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
        let start = std::time::Instant::now();

        debug!(
            subagent = subagent,
            parent_conversation_id = %self.conversation_id,
            mode = "foreground",
            "gen_ai.agent.execute"
        );

        // Emit SubAgentStarted event
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::SubAgentStarted,
            &self.agent_id,
            &self.conversation_id,
            serde_json::json!({
                "subagent_name": subagent,
                "mode": "foreground",
                "parent_conversation_id": &self.conversation_id,
                "depth": self.depth,
            }),
        ));

        let entry = self
            .subagents
            .get(subagent)
            .ok_or_else(|| format!("Subagent '{}' not found", subagent))?;

        let agent = entry.agent.clone();
        let foreground_timeout = entry.foreground_timeout;
        let task_id = task_id
            .map(|id| id.to_string())
            .unwrap_or_else(|| self.generate_task_id());

        let mut history = self.session_store.get_or_create(&task_id);

        // Compact subagent history if configured
        if let Some(ref config) = self.compaction_config {
            if let Ok(Some(result)) = crate::compaction::maybe_compact(&history, config).await {
                history = result.compacted_history;
                self.session_store.save(task_id.clone(), history.clone());
            }
        }

        let cancel = self.cancel.clone();
        let emitter = ToolCallEmitter {
            event_bus: self.event_bus.clone(),
            cancel: cancel.clone(),
            tool_names: std::collections::HashSet::new(),
            tool_executors: std::collections::HashMap::new(),
            agent_id: self.agent_id.clone(),
            conversation_id: self.conversation_id.clone(),
            permission_manager: std::sync::Arc::new(llm::PermissionManager::new()),
            agent_permissions: std::collections::HashMap::new(),
            metrics: self.metrics.clone(),
            conversation_metrics: None,
            pending_tool_timings: std::sync::Arc::new(dashmap::DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
        };

        let prompt_owned = prompt.to_string();

        let result = tokio::select! {
            _ = cancel.cancelled() => {
                Err("Subagent cancelled".to_string())
            }
            result = async {
                tokio::time::timeout(
                    foreground_timeout,
                    agent.prompt_standard_with_hook(&prompt_owned, &mut history, emitter),
                ).await
            } => {
                match result {
                    Err(_) => Err(format!("Subagent timed out after {}s", foreground_timeout.as_secs())),
                    Ok(Ok(output)) => Ok(output),
                    Ok(Err(e)) => Err(format!("Subagent error: {}", e)),
                }
            }
        };

        // Save history regardless of outcome (for resumption)
        self.session_store.save(task_id.clone(), history);

        let duration_ms = start.elapsed().as_millis() as u64;
        let is_error = result.is_err();

        // Emit SubAgentCompleted event
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::SubAgentCompleted,
            &self.agent_id,
            &self.conversation_id,
            serde_json::json!({
                "subagent_name": subagent,
                "mode": "foreground",
                "task_id": &task_id,
                "parent_conversation_id": &self.conversation_id,
                "duration_ms": duration_ms,
                "is_error": is_error,
            }),
        ));

        match result {
            Ok(output) => Ok(AgentTaskResult { task_id, output }),
            Err(e) => Err(e),
        }
    }

    async fn run_background(
        &self,
        subagent: &str,
        prompt: &str,
        description: &str,
    ) -> Result<AgentTaskHandle, String> {
        debug!(
            subagent = subagent,
            parent_conversation_id = %self.conversation_id,
            mode = "background",
            "gen_ai.agent.execute"
        );

        // Emit SubAgentStarted event
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::SubAgentStarted,
            &self.agent_id,
            &self.conversation_id,
            serde_json::json!({
                "subagent_name": subagent,
                "mode": "background",
                "parent_conversation_id": &self.conversation_id,
                "depth": self.depth,
            }),
        ));

        let entry = self
            .subagents
            .get(subagent)
            .ok_or_else(|| format!("Subagent '{}' not found", subagent))?;

        let agent = entry.agent.clone();
        let background_timeout = entry.background_timeout;
        let task_id = self.generate_task_id();
        let task_id_clone = task_id.clone();

        let mut history = self.session_store.get_or_create(&task_id);
        let compaction_config = self.compaction_config.clone();

        // Compact subagent history if configured
        if let Some(ref config) = compaction_config {
            if let Ok(Some(result)) = crate::compaction::maybe_compact(&history, config).await {
                history = result.compacted_history;
                self.session_store.save(task_id.clone(), history.clone());
            }
        }

        let session_store = self.session_store.clone();
        let notification_tx = self.notification_tx.clone();
        let cancel = self.cancel.clone();
        let event_bus = self.event_bus.clone();
        let prompt_owned = prompt.to_string();
        let description_owned = description.to_string();
        let subagents = self.subagents.clone();
        let conversation_id = self.conversation_id.clone();
        let depth = self.depth;
        let max_depth = self.max_depth;
        let task_budget = self.task_budget.clone();
        let metrics_clone = self.metrics.clone();
        let agent_id_clone = self.agent_id.clone();
        let subagent_name = subagent.to_string();

        tokio::spawn(async move {
            let bg_start = std::time::Instant::now();
            let emitter_conv_id = conversation_id.clone();
            let event_conv_id = conversation_id.clone();
            // Build nested AgentContext for the background task
            let nested_runner = Arc::new(
                ConversationSubAgentRunner::new(
                    subagents,
                    session_store.clone(),
                    notification_tx.clone(),
                    cancel.clone(),
                    event_bus.clone(),
                    conversation_id,
                    depth + 1,
                    max_depth,
                    metrics_clone.clone(),
                )
                .with_task_budget(task_budget.clone()),
            );
            let nested_ctx = AgentContext {
                runner: nested_runner,
                notification_tx: notification_tx.clone(),
                depth: depth + 1,
                max_depth,
                task_budget,
            };

            let mut history = history;
            let emitter = ToolCallEmitter {
                event_bus: event_bus.clone(),
                cancel: cancel.clone(),
                tool_names: std::collections::HashSet::new(),
                tool_executors: std::collections::HashMap::new(),
                agent_id: agent_id_clone.clone(),
                conversation_id: emitter_conv_id,
                permission_manager: std::sync::Arc::new(llm::PermissionManager::new()),
                agent_permissions: std::collections::HashMap::new(),
                metrics: metrics_clone,
                conversation_metrics: None,
                pending_tool_timings: std::sync::Arc::new(dashmap::DashMap::new()),
                storage: None,
                persisted_messages: None,
                pressure_threshold_bytes: None,
                pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
                pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
            };

            let result = AGENT_CONTEXT
                .scope(nested_ctx, async {
                    tokio::select! {
                        _ = cancel.cancelled() => {
                            Err("Background subagent cancelled".to_string())
                        }
                        result = async {
                            tokio::time::timeout(
                                background_timeout,
                                agent.prompt_standard_with_hook(&prompt_owned, &mut history, emitter),
                            ).await
                        } => {
                            match result {
                                Err(_) => Err(format!("Background subagent timed out after {}s", background_timeout.as_secs())),
                                Ok(Ok(output)) => Ok(output),
                                Ok(Err(e)) => Err(format!("Background subagent error: {}", e)),
                            }
                        }
                    }
                })
                .await;

            // Save history
            session_store.save(task_id_clone.clone(), history);

            // Emit SubAgentCompleted event
            {
                let duration_ms = bg_start.elapsed().as_millis() as u64;
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::SubAgentCompleted,
                    &agent_id_clone,
                    &event_conv_id,
                    serde_json::json!({
                        "subagent_name": &subagent_name,
                        "mode": "background",
                        "task_id": &task_id_clone,
                        "duration_ms": duration_ms,
                        "is_error": result.is_err(),
                    }),
                ));
            }

            // Send notification
            let notification = AgentTaskNotification {
                task_id: task_id_clone.clone(),
                description: description_owned,
                output: result,
            };

            if notification_tx.send(notification).await.is_err() {
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
        let store = AgentSessionStore::new(String::new(), None);
        let history = store.get_or_create("task-1");
        assert!(history.is_empty());
    }

    #[test]
    fn test_session_store_save_and_retrieve() {
        let store = AgentSessionStore::new(String::new(), None);
        let history = vec![rig::message::Message::user("hello")];
        store.save("task-1".to_string(), history.clone());
        let retrieved = store.get_or_create("task-1");
        assert_eq!(retrieved.len(), 1);
    }

    #[test]
    fn test_session_store_remove_by_prefix() {
        let store = AgentSessionStore::new(String::new(), None);
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

    // ── Fix #6: Indexed session store tests ────────────────────────────

    #[test]
    fn test_session_store_indexed_removal_with_uuid_keys() {
        let store = AgentSessionStore::new(String::new(), None);
        // Use realistic UUID-format task_ids: "{conv_uuid}-{task_uuid}"
        let conv_id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
        let task1 = format!("{}-{}", conv_id, "11111111-1111-1111-1111-111111111111");
        let task2 = format!("{}-{}", conv_id, "22222222-2222-2222-2222-222222222222");
        let other = format!(
            "{}-{}",
            "ffffffff-ffff-ffff-ffff-ffffffffffff", "33333333-3333-3333-3333-333333333333"
        );

        store.save(task1.clone(), vec![rig::message::Message::user("a")]);
        store.save(task2.clone(), vec![rig::message::Message::user("b")]);
        store.save(other.clone(), vec![rig::message::Message::user("c")]);

        assert_eq!(store.len(), 3);

        // Remove by conversation prefix (UUID = 36 chars)
        store.remove_by_prefix(conv_id);

        assert!(store.get_or_create(&task1).is_empty());
        assert!(store.get_or_create(&task2).is_empty());
        assert_eq!(store.get_or_create(&other).len(), 1);
        assert_eq!(store.len(), 1);
    }

    #[test]
    fn test_session_store_len_and_is_empty() {
        let store = AgentSessionStore::new(String::new(), None);
        assert!(store.is_empty());
        assert_eq!(store.len(), 0);

        store.save("task-1".to_string(), vec![]);
        assert!(!store.is_empty());
        assert_eq!(store.len(), 1);
    }

    #[test]
    fn test_extract_conversation_id_valid() {
        // UUID is 36 chars: 8-4-4-4-12
        let conv_id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
        let task_uuid = "11111111-1111-1111-1111-111111111111";
        let task_id = format!("{}-{}", conv_id, task_uuid);
        let result = extract_conversation_id(&task_id);
        assert_eq!(result, Some(conv_id.to_string()));
    }

    #[test]
    fn test_extract_conversation_id_too_short() {
        assert_eq!(extract_conversation_id("short"), None);
        assert_eq!(extract_conversation_id(""), None);
    }

    #[test]
    fn test_session_store_fallback_for_non_uuid_keys() {
        let store = AgentSessionStore::new(String::new(), None);
        // Non-UUID keys that won't match the index — should still be cleaned up via fallback
        store.save(
            "myprefix-task-1".to_string(),
            vec![rig::message::Message::user("a")],
        );
        store.save(
            "myprefix-task-2".to_string(),
            vec![rig::message::Message::user("b")],
        );

        store.remove_by_prefix("myprefix");

        assert!(store.get_or_create("myprefix-task-1").is_empty());
        assert!(store.get_or_create("myprefix-task-2").is_empty());
    }
}
