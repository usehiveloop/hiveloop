use bridge_core::BridgeError;
use std::sync::Arc;
use std::time::Duration;
use tracing::{info, warn};

use crate::agent_map::AgentMap;
use crate::agent_state::AgentState;

/// Drain an existing agent and replace it with a new state.
///
/// This performs a graceful transition:
/// 1. Cancel the old agent's tasks
/// 2. Close the old task tracker (no new tasks)
/// 3. Wait for in-flight conversations to complete (with timeout)
/// 4. Swap the agent state in the map
///
/// If the timeout is reached, the old conversations are forcefully dropped.
pub async fn drain_and_replace(
    agent_map: &AgentMap,
    agent_id: &str,
    new_state: Arc<AgentState>,
    timeout: Duration,
) -> Result<(), BridgeError> {
    let old_state = agent_map.get(agent_id);

    if let Some(old) = old_state {
        info!(
            agent_id = agent_id,
            active_conversations = old.active_conversation_count(),
            "draining agent"
        );

        // Signal cancellation to the old agent's conversations
        old.cancel.cancel();
        old.tracker.close();

        // Wait for in-flight tasks with a timeout
        let wait_result = tokio::time::timeout(timeout, old.tracker.wait()).await;

        if wait_result.is_err() {
            warn!(
                agent_id = agent_id,
                timeout_secs = timeout.as_secs(),
                "drain timeout reached, forcing shutdown"
            );
        }

        info!(
            agent_id = agent_id,
            "agent drained, replacing with new state"
        );
    }

    // Swap in the new state
    agent_map.insert(agent_id.to_string(), new_state);

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::agent::{AgentConfig, AgentDefinition};
    use bridge_core::provider::{ProviderConfig, ProviderType};
    use llm::build_agent;
    use tools::ToolRegistry;

    fn make_test_definition(id: &str) -> AgentDefinition {
        AgentDefinition {
            id: id.to_string(),
            name: format!("Agent {}", id),
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
        }
    }

    fn make_test_state(id: &str, version: &str) -> Arc<AgentState> {
        let mut def = make_test_definition(id);
        def.version = Some(version.to_string());
        let agent = build_agent(&def, vec![]).expect("build agent");
        Arc::new(AgentState::new(
            def,
            agent,
            ToolRegistry::new(),
            Arc::new(dashmap::DashMap::new()),
            None,
            std::collections::HashMap::new(),
        ))
    }

    #[tokio::test]
    async fn test_drain_and_replace_swaps_state() {
        let map = AgentMap::new();

        let old_state = make_test_state("agent1", "v1");
        map.insert("agent1".to_string(), old_state);

        let new_state = make_test_state("agent1", "v2");
        let result = drain_and_replace(&map, "agent1", new_state, Duration::from_secs(5)).await;
        assert!(result.is_ok());

        let current = map.get("agent1").expect("agent should exist");
        assert_eq!(current.version().await.as_deref(), Some("v2"));
    }

    #[tokio::test]
    async fn test_drain_and_replace_nonexistent_inserts() {
        let map = AgentMap::new();

        let new_state = make_test_state("agent1", "v1");
        let result = drain_and_replace(&map, "agent1", new_state, Duration::from_secs(5)).await;
        assert!(result.is_ok());
        assert!(map.get("agent1").is_some());
    }
}
