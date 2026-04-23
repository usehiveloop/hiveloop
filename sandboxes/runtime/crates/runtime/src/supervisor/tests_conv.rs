use super::*;
use bridge_core::AgentDefinition;
use mcp::McpManager;
use std::sync::Arc;
use tokio_util::sync::CancellationToken;

pub(super) fn make_test_supervisor() -> AgentSupervisor {
    let mcp_manager = Arc::new(McpManager::new());
    let cancel = CancellationToken::new();
    let event_bus = Arc::new(webhooks::EventBus::new(
        None,
        None,
        String::new(),
        String::new(),
    ));
    AgentSupervisor::new(mcp_manager, cancel).with_event_bus(Some(event_bus))
}

pub(super) fn make_test_definition(id: &str) -> AgentDefinition {
    AgentDefinition {
        id: id.to_string(),
        name: format!("Test Agent {}", id),
        description: None,
        system_prompt: "You are a test agent.".to_string(),
        provider: bridge_core::provider::ProviderConfig {
            provider_type: bridge_core::provider::ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "test-key".to_string(),
            base_url: Some("https://api.openai.com/v1".to_string()),
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        },
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        config: bridge_core::agent::AgentConfig::default(),
        subagents: vec![],
        permissions: std::collections::HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: Some("1".to_string()),
        updated_at: None,
    }
}

#[tokio::test]
async fn supervisor_create_conversation_no_filters_succeeds() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let result = supervisor
        .create_conversation("agent1", None, None, None, None, None, None)
        .await;

    assert!(result.is_ok());
    let (conv_id, _sse_rx) = result.unwrap();
    assert!(!conv_id.is_empty());

    // Cleanup
    supervisor.end_conversation("agent1", &conv_id).unwrap();
}

#[tokio::test]
async fn supervisor_create_conversation_with_valid_tool_filter_succeeds() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    // Agent has builtin tools (bash, read, write, etc.) because tools: [] means all builtins.
    // Pick some known builtin tool names.
    let state = supervisor.get_agent("agent1").unwrap();
    let all_tools: Vec<String> = state.tool_registry.tool_names();
    assert!(!all_tools.is_empty(), "agent should have builtin tools");

    // Request only the first two tools
    let filter = all_tools.iter().take(2).cloned().collect::<Vec<_>>();

    let result = supervisor
        .create_conversation("agent1", Some(filter.clone()), None, None, None, None, None)
        .await;

    assert!(result.is_ok());
    let (conv_id, _sse_rx) = result.unwrap();
    assert!(!conv_id.is_empty());

    supervisor.end_conversation("agent1", &conv_id).unwrap();
}

#[tokio::test]
async fn supervisor_create_conversation_with_invalid_tool_returns_error() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let result = supervisor
        .create_conversation(
            "agent1",
            Some(vec!["totally_fake_tool".to_string()]),
            None,
            None,
            None,
            None,
            None,
        )
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("totally_fake_tool"));
    assert!(err.contains("agent1"));
}

#[tokio::test]
async fn supervisor_create_conversation_with_invalid_mcp_server_returns_error() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let result = supervisor
        .create_conversation(
            "agent1",
            None,
            Some(vec!["nonexistent-mcp".to_string()]),
            None,
            None,
            None,
            None,
        )
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("nonexistent-mcp"));
}

#[tokio::test]
async fn supervisor_create_conversation_unknown_agent_returns_error() {
    let supervisor = make_test_supervisor();

    let result = supervisor
        .create_conversation("no_such_agent", None, None, None, None, None, None)
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("no_such_agent"));
}

// ── per-conversation API key override ──────────────────────────────────

#[tokio::test]
async fn supervisor_create_conversation_with_api_key_override_succeeds() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let result = supervisor
        .create_conversation(
            "agent1",
            None,
            None,
            Some("sk-custom-override-key".to_string()),
            None,
            None,
            None,
        )
        .await;

    assert!(result.is_ok());
    let (conv_id, _sse_rx) = result.unwrap();
    assert!(!conv_id.is_empty());

    supervisor.end_conversation("agent1", &conv_id).unwrap();
}

#[tokio::test]
async fn supervisor_create_conversation_with_empty_api_key_returns_error() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let result = supervisor
        .create_conversation("agent1", None, None, Some("".to_string()), None, None, None)
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("api_key cannot be empty"));
}

#[tokio::test]
async fn supervisor_create_conversation_with_whitespace_api_key_returns_error() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let result = supervisor
        .create_conversation(
            "agent1",
            None,
            None,
            Some("   ".to_string()),
            None,
            None,
            None,
        )
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("api_key cannot be empty"));
}

#[tokio::test]
async fn supervisor_create_conversation_with_api_key_and_tool_filter_succeeds() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let state = supervisor.get_agent("agent1").unwrap();
    let all_tools: Vec<String> = state.tool_registry.tool_names();
    let filter = all_tools.iter().take(2).cloned().collect::<Vec<_>>();

    let result = supervisor
        .create_conversation(
            "agent1",
            Some(filter),
            None,
            Some("sk-custom-key".to_string()),
            None,
            None,
            None,
        )
        .await;

    assert!(result.is_ok());
    let (conv_id, _sse_rx) = result.unwrap();
    supervisor.end_conversation("agent1", &conv_id).unwrap();
}
