use dashmap::DashMap;
use storage::StorageHandle;
use tracing::warn;

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
pub(super) fn extract_conversation_id(task_id: &str) -> Option<String> {
    // UUID v4 is 36 chars: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    // Task IDs are formatted as: "{conv_id}-{uuid}" (see generate_task_id)
    if task_id.len() > 37 && task_id.as_bytes().get(36) == Some(&b'-') {
        Some(task_id[..36].to_string())
    } else {
        None
    }
}
