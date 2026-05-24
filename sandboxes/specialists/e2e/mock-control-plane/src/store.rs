use bridge_core::AgentDefinition;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::RwLock;

/// A received webhook payload logged by the mock server.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReceivedWebhook {
    /// When the webhook was received.
    pub timestamp: chrono::DateTime<chrono::Utc>,
    /// HTTP headers from the request.
    pub headers: HashMap<String, String>,
    /// Parsed JSON body.
    pub body: serde_json::Value,
}

/// In-memory store for the mock control plane.
pub struct MockStore {
    agents: RwLock<HashMap<String, AgentDefinition>>,
    webhooks: RwLock<Vec<ReceivedWebhook>>,
    version: AtomicU64,
}

impl MockStore {
    /// Create a new empty store.
    pub fn new() -> Self {
        Self {
            agents: RwLock::new(HashMap::new()),
            webhooks: RwLock::new(Vec::new()),
            version: AtomicU64::new(1),
        }
    }

    /// Load initial agents into the store.
    #[allow(dead_code)]
    pub fn load_agents(&self, agents: Vec<AgentDefinition>) {
        let mut map = self.agents.write().expect("lock poisoned");
        for agent in agents {
            map.insert(agent.id.clone(), agent);
        }
    }

    /// Get all agents.
    pub fn get_all_agents(&self) -> Vec<AgentDefinition> {
        let map = self.agents.read().expect("lock poisoned");
        map.values().cloned().collect()
    }

    /// Get a single agent by ID.
    pub fn get_agent(&self, id: &str) -> Option<AgentDefinition> {
        let map = self.agents.read().expect("lock poisoned");
        map.get(id).cloned()
    }

    /// Create a new agent. Returns the version.
    pub fn create_agent(&self, agent: AgentDefinition) -> u64 {
        let version = self.version.fetch_add(1, Ordering::SeqCst);
        let mut map = self.agents.write().expect("lock poisoned");
        map.insert(agent.id.clone(), agent);
        version
    }

    /// Update an existing agent. Returns Some(version) if found.
    pub fn update_agent(&self, id: &str, agent: AgentDefinition) -> Option<u64> {
        let mut map = self.agents.write().expect("lock poisoned");
        if map.contains_key(id) {
            let version = self.version.fetch_add(1, Ordering::SeqCst);
            map.insert(id.to_string(), agent);
            Some(version)
        } else {
            None
        }
    }

    /// Delete an agent by ID. Returns true if found.
    pub fn delete_agent(&self, id: &str) -> bool {
        let mut map = self.agents.write().expect("lock poisoned");
        self.version.fetch_add(1, Ordering::SeqCst);
        map.remove(id).is_some()
    }

    /// Log a received webhook.
    pub fn push_webhook(&self, webhook: ReceivedWebhook) {
        let mut wh = self.webhooks.write().expect("lock poisoned");
        wh.push(webhook);
    }

    /// Get all received webhooks.
    pub fn get_all_webhooks(&self) -> Vec<ReceivedWebhook> {
        let wh = self.webhooks.read().expect("lock poisoned");
        wh.clone()
    }

    /// Clear all received webhooks.
    pub fn clear_webhooks(&self) {
        let mut wh = self.webhooks.write().expect("lock poisoned");
        wh.clear();
    }
}
