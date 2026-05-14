use bridge_core::AgentSummary;
use dashmap::DashMap;
use std::sync::Arc;

use crate::agent_state::AgentState;

/// Thread-safe map of agent states, keyed by agent ID.
pub struct AgentMap {
    inner: DashMap<String, Arc<AgentState>>,
}

impl AgentMap {
    pub fn new() -> Self {
        Self {
            inner: DashMap::new(),
        }
    }

    pub fn get(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.inner.get(agent_id).map(|entry| entry.value().clone())
    }

    pub fn insert(&self, agent_id: String, state: Arc<AgentState>) {
        self.inner.insert(agent_id, state);
    }

    pub fn remove(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.inner.remove(agent_id).map(|(_, state)| state)
    }

    pub async fn list(&self) -> Vec<AgentSummary> {
        let mut summaries = Vec::new();
        for entry in self.inner.iter() {
            let state = entry.value();
            summaries.push(AgentSummary {
                id: state.id().await,
                name: state.name().await,
                version: state.version().await,
            });
        }
        summaries
    }

    pub fn list_states(&self) -> Vec<Arc<AgentState>> {
        self.inner.iter().map(|e| e.value().clone()).collect()
    }

    pub fn len(&self) -> usize {
        self.inner.len()
    }

    pub fn is_empty(&self) -> bool {
        self.inner.is_empty()
    }

    pub fn agent_ids(&self) -> Vec<String> {
        self.inner.iter().map(|e| e.key().clone()).collect()
    }
}

impl Default for AgentMap {
    fn default() -> Self {
        Self::new()
    }
}
