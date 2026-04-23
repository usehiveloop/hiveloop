use super::tests_conv::{make_test_definition, make_test_supervisor};
use bridge_core::AgentDefinition;

#[tokio::test]
async fn supervisor_create_conversation_with_invalid_subagent_name_returns_error() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let mut overrides = std::collections::HashMap::new();
    overrides.insert("nonexistent_subagent".to_string(), "<provider-api-key>".to_string());

    let result = supervisor
        .create_conversation("agent1", None, None, None, Some(overrides), None, None)
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("nonexistent_subagent"));
}

#[tokio::test]
async fn supervisor_create_conversation_with_empty_subagent_api_key_returns_error() {
    let supervisor = make_test_supervisor();

    let mut def = make_test_definition("agent1");
    def.subagents.push(AgentDefinition {
        id: "sub1".to_string(),
        name: "sub1".to_string(),
        description: Some("A test subagent".to_string()),
        system_prompt: "You are a sub agent.".to_string(),
        provider: bridge_core::provider::ProviderConfig {
            provider_type: bridge_core::provider::ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "sub-key".to_string(),
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
        version: None,
        updated_at: None,
    });

    supervisor.load_agents(vec![def]).await.unwrap();

    let mut overrides = std::collections::HashMap::new();
    overrides.insert("sub1".to_string(), "".to_string());

    let result = supervisor
        .create_conversation("agent1", None, None, None, Some(overrides), None, None)
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(err.contains("cannot be empty"));
}

#[tokio::test]
async fn supervisor_create_conversation_with_subagent_api_key_override_succeeds() {
    let supervisor = make_test_supervisor();

    let mut def = make_test_definition("agent1");
    def.subagents.push(AgentDefinition {
        id: "sub1".to_string(),
        name: "sub1".to_string(),
        description: Some("A test subagent".to_string()),
        system_prompt: "You are a sub agent.".to_string(),
        provider: bridge_core::provider::ProviderConfig {
            provider_type: bridge_core::provider::ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "sub-key".to_string(),
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
        version: None,
        updated_at: None,
    });

    supervisor.load_agents(vec![def]).await.unwrap();

    let mut overrides = std::collections::HashMap::new();
    overrides.insert("sub1".to_string(), "sk-overridden-sub-key".to_string());

    let result = supervisor
        .create_conversation("agent1", None, None, None, Some(overrides), None, None)
        .await;

    assert!(result.is_ok());
    let (conv_id, _sse_rx) = result.unwrap();
    supervisor.end_conversation("agent1", &conv_id).unwrap();
}
