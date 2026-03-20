use bridge_core::AgentSummary;
use dashmap::DashMap;
use std::sync::Arc;

use crate::agent_state::AgentState;

/// Thread-safe map of agent states, keyed by agent ID.
///
/// Wraps a DashMap for concurrent access from multiple request handlers
/// and the sync poller.
pub struct AgentMap {
    inner: DashMap<String, Arc<AgentState>>,
}

impl AgentMap {
    /// Create a new empty agent map.
    pub fn new() -> Self {
        Self {
            inner: DashMap::new(),
        }
    }

    /// Get an agent state by ID.
    pub fn get(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.inner.get(agent_id).map(|entry| entry.value().clone())
    }

    /// Insert or replace an agent state.
    pub fn insert(&self, agent_id: String, state: Arc<AgentState>) {
        self.inner.insert(agent_id, state);
    }

    /// Remove an agent state, returning it if it existed.
    pub fn remove(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.inner.remove(agent_id).map(|(_, state)| state)
    }

    /// List all agents as summaries.
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

    /// Get the number of loaded agents.
    pub fn len(&self) -> usize {
        self.inner.len()
    }

    /// Check if the map is empty.
    pub fn is_empty(&self) -> bool {
        self.inner.is_empty()
    }

    /// Iterate over all agent IDs.
    pub fn agent_ids(&self) -> Vec<String> {
        self.inner.iter().map(|e| e.key().clone()).collect()
    }
}

impl Default for AgentMap {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::agent::{AgentConfig, AgentDefinition};
    use bridge_core::provider::{ProviderConfig, ProviderType};
    use llm::build_agent;
    use tools::ToolRegistry;

    fn make_test_state(id: &str, name: &str) -> Arc<AgentState> {
        let definition = AgentDefinition {
            id: id.to_string(),
            name: name.to_string(),
            description: None,
            system_prompt: "test".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "test-key".to_string(),
                base_url: Some("https://api.openai.com/v1".to_string()),
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: AgentConfig::default(),
            subagents: vec![],
            permissions: std::collections::HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: Some("1".to_string()),
            updated_at: None,
        };
        let agent = build_agent(&definition, vec![]).expect("build agent");
        Arc::new(AgentState::new(
            definition,
            agent,
            ToolRegistry::new(),
            Arc::new(dashmap::DashMap::new()),
            Arc::new(tools::join::TaskRegistry::new()),
        ))
    }

    #[test]
    fn test_new_map_is_empty() {
        let map = AgentMap::new();
        assert!(map.is_empty());
        assert_eq!(map.len(), 0);
    }

    #[tokio::test]
    async fn test_insert_and_get() {
        let map = AgentMap::new();
        let state = make_test_state("agent1", "Agent One");
        map.insert("agent1".to_string(), state);

        assert_eq!(map.len(), 1);
        assert!(!map.is_empty());

        let retrieved = map.get("agent1");
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().id().await, "agent1");
    }

    #[test]
    fn test_get_nonexistent_returns_none() {
        let map = AgentMap::new();
        assert!(map.get("nonexistent").is_none());
    }

    #[tokio::test]
    async fn test_remove() {
        let map = AgentMap::new();
        let state = make_test_state("agent1", "Agent One");
        map.insert("agent1".to_string(), state);

        let removed = map.remove("agent1");
        assert!(removed.is_some());
        assert_eq!(map.len(), 0);
        assert!(map.get("agent1").is_none());
    }

    #[tokio::test]
    async fn test_list_agents() {
        let map = AgentMap::new();
        map.insert("agent1".to_string(), make_test_state("agent1", "Agent One"));
        map.insert("agent2".to_string(), make_test_state("agent2", "Agent Two"));

        let summaries = map.list().await;
        assert_eq!(summaries.len(), 2);

        let ids: Vec<&str> = summaries.iter().map(|s| s.id.as_str()).collect();
        assert!(ids.contains(&"agent1"));
        assert!(ids.contains(&"agent2"));
    }

    #[tokio::test]
    async fn test_agent_ids() {
        let map = AgentMap::new();
        map.insert("agent1".to_string(), make_test_state("agent1", "Agent One"));
        map.insert("agent2".to_string(), make_test_state("agent2", "Agent Two"));

        let ids = map.agent_ids();
        assert_eq!(ids.len(), 2);
        assert!(ids.contains(&"agent1".to_string()));
        assert!(ids.contains(&"agent2".to_string()));
    }
}
