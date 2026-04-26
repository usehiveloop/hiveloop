use crate::providers::build::{
    build_anthropic_client, build_cohere_client, build_gemini_client, build_openai_client,
};
use crate::providers::*;
use bridge_core::provider::{ProviderConfig, ProviderType};

#[test]
fn test_build_openai_client_requires_base_url() {
    let config = ProviderConfig {
        provider_type: ProviderType::OpenAI,
        model: "gpt-4o".to_string(),
        api_key: "test-key".to_string(),
        base_url: None,
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    assert!(build_openai_client(&config).is_err());
}

#[test]
fn test_build_openai_client_with_base_url() {
    let config = ProviderConfig {
        provider_type: ProviderType::OpenAI,
        model: "gpt-4o".to_string(),
        api_key: "test-key".to_string(),
        base_url: Some("https://api.openai.com/v1".to_string()),
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    assert!(build_openai_client(&config).is_ok());
}

#[test]
fn test_build_anthropic_client() {
    let config = ProviderConfig {
        provider_type: ProviderType::Anthropic,
        model: "claude-sonnet-4-20250514".to_string(),
        api_key: "test-key".to_string(),
        base_url: None,
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    assert!(build_anthropic_client(&config).is_ok());
}

#[test]
fn test_build_anthropic_client_with_one_hour_ttl_succeeds() {
    // Setting the 1-hour TTL is always safe — the beta header is just
    // attached. Actual cache_control TTL wiring awaits a newer rig.
    let config = ProviderConfig {
        provider_type: ProviderType::Anthropic,
        model: "claude-sonnet-4-20250514".to_string(),
        api_key: "test-key".to_string(),
        base_url: None,
        prompt_caching_enabled: true,
        cache_ttl: bridge_core::provider::CacheTtl::OneHour,
    };
    assert!(build_anthropic_client(&config).is_ok());
}

#[tokio::test]
async fn test_create_agent_anthropic_computes_prefix_hash() {
    // Build a minimal agent and confirm the prefix hash is populated
    // (non-empty, 64 hex chars). The hash must be the same for two
    // identical builds — the cache-bust invariant this P0.3 work
    // protects.
    let config = ProviderConfig {
        provider_type: ProviderType::Anthropic,
        model: "claude-sonnet-4-20250514".to_string(),
        api_key: "test-key".to_string(),
        base_url: None,
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    let definition = bridge_core::agent::AgentDefinition {
        id: "t".into(),
        name: "t".into(),
        description: None,
        system_prompt: "you are helpful".into(),
        provider: config.clone(),
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        artifacts: None,
        config: bridge_core::agent::AgentConfig::default(),
        subagents: vec![],
        permissions: std::collections::HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };
    let a = create_agent(&config, vec![], "you are helpful", &definition).unwrap();
    let b = create_agent(&config, vec![], "you are helpful", &definition).unwrap();
    assert_eq!(a.prefix_hash().len(), 64);
    assert_eq!(
        a.prefix_hash(),
        b.prefix_hash(),
        "two identical builds must produce the same prefix hash"
    );
}

#[tokio::test]
async fn test_create_agent_prefix_hash_changes_with_preamble() {
    let config = ProviderConfig {
        provider_type: ProviderType::OpenAI,
        model: "gpt-4o".to_string(),
        api_key: "k".to_string(),
        base_url: Some("https://api.openai.com/v1".to_string()),
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    let definition = bridge_core::agent::AgentDefinition {
        id: "t".into(),
        name: "t".into(),
        description: None,
        system_prompt: "ignored".into(),
        provider: config.clone(),
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        artifacts: None,
        config: bridge_core::agent::AgentConfig::default(),
        subagents: vec![],
        permissions: std::collections::HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };
    let a = create_agent(&config, vec![], "preamble A", &definition).unwrap();
    let b = create_agent(&config, vec![], "preamble B", &definition).unwrap();
    assert_ne!(
        a.prefix_hash(),
        b.prefix_hash(),
        "preamble diff must surface as a prefix-hash diff"
    );
}

#[test]
fn test_build_gemini_client() {
    let config = ProviderConfig {
        provider_type: ProviderType::Google,
        model: "gemini-2.0-flash".to_string(),
        api_key: "test-key".to_string(),
        base_url: None,
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    assert!(build_gemini_client(&config).is_ok());
}

#[test]
fn test_build_cohere_client() {
    let config = ProviderConfig {
        provider_type: ProviderType::Cohere,
        model: "command-r-plus".to_string(),
        api_key: "test-key".to_string(),
        base_url: None,
        prompt_caching_enabled: true,
        cache_ttl: Default::default(),
    };
    assert!(build_cohere_client(&config).is_ok());
}
