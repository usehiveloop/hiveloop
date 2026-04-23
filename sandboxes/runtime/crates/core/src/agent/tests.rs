use super::*;
use crate::mcp::McpTransport;
use crate::provider::ProviderType;

/// Helper to load a fixture file relative to the workspace root.
fn load_fixture(path: &str) -> String {
    let workspace_root = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .unwrap()
        .parent()
        .unwrap();
    let full_path = workspace_root.join(path);
    std::fs::read_to_string(&full_path)
        .unwrap_or_else(|e| panic!("Failed to read fixture {}: {}", full_path.display(), e))
}

#[test]
fn parse_simple_agent_fixture() {
    let json = load_fixture("fixtures/agents/simple_agent.json");
    let agent: AgentDefinition =
        serde_json::from_str(&json).expect("simple_agent.json should deserialize");

    assert_eq!(agent.id, "agent_simple");
    assert_eq!(agent.name, "Simple Agent");
    assert_eq!(agent.system_prompt, "You are a helpful assistant.");
    assert_eq!(agent.provider.provider_type, ProviderType::OpenAI);
    assert_eq!(agent.provider.model, "gpt-4o");
    assert_eq!(agent.provider.api_key, "test-key");
    assert!(agent.provider.base_url.is_some());
    assert!(agent.tools.is_empty());
    assert!(agent.mcp_servers.is_empty());
    assert!(agent.skills.is_empty());
    assert!(agent.webhook_url.is_none());
    assert!(agent.webhook_secret.is_none());
}

#[test]
fn parse_full_agent_fixture() {
    let json = load_fixture("fixtures/agents/full_agent.json");
    let agent: AgentDefinition =
        serde_json::from_str(&json).expect("full_agent.json should deserialize");

    assert_eq!(agent.id, "agent_full");
    assert_eq!(agent.name, "Full Agent");
    assert_eq!(agent.system_prompt, "You are a coding assistant.");

    // Provider
    assert_eq!(agent.provider.provider_type, ProviderType::OpenAI);
    assert_eq!(agent.provider.model, "gpt-4o");
    assert_eq!(
        agent.provider.base_url.as_deref(),
        Some("https://api.openai.com/v1")
    );

    // Tools
    assert_eq!(agent.tools.len(), 1);
    assert_eq!(agent.tools[0].name, "calculator");
    assert!(agent.tools[0].parameters_schema.is_object());

    // MCP servers
    assert_eq!(agent.mcp_servers.len(), 1);
    assert_eq!(agent.mcp_servers[0].name, "filesystem");
    match &agent.mcp_servers[0].transport {
        McpTransport::Stdio { command, args, env } => {
            assert_eq!(command, "npx");
            assert_eq!(args.len(), 3);
            assert!(env.contains_key("NODE_ENV"));
        }
        other => panic!("Expected Stdio transport, got {:?}", other),
    }

    // Skills
    assert_eq!(agent.skills.len(), 2);
    assert_eq!(agent.skills[0].id, "skill_code_review");
    assert_eq!(agent.skills[1].id, "skill_deploy");
    assert!(!agent.skills[1].files.is_empty());
    assert_eq!(agent.skills[1].files.len(), 2);
    assert!(agent.skills[1].files.contains_key("runbook.md"));
    assert!(agent.skills[1].frontmatter.is_some());

    // Config
    assert_eq!(agent.config.max_tokens, Some(4096));
    assert_eq!(agent.config.max_turns, Some(10));
    assert_eq!(agent.config.temperature, Some(0.7));

    // Webhooks
    assert_eq!(
        agent.webhook_url.as_deref(),
        Some("https://example.com/webhooks/agent")
    );
    assert_eq!(
        agent.webhook_secret.as_deref(),
        Some("<webhook-secret>")
    );
}

#[test]
fn parse_anthropic_agent_fixture() {
    let json = load_fixture("fixtures/agents/anthropic_agent.json");
    let agent: AgentDefinition =
        serde_json::from_str(&json).expect("anthropic_agent.json should deserialize");

    assert_eq!(agent.id, "agent_anthropic");
    assert_eq!(agent.name, "Anthropic Agent");
    assert_eq!(agent.provider.provider_type, ProviderType::Anthropic);
    assert_eq!(agent.provider.model, "claude-haiku-4-5-20251001");
    assert_eq!(agent.config.max_tokens, Some(4096));
    assert_eq!(agent.config.temperature, Some(0.7));
}

#[test]
fn parse_gemini_agent_fixture() {
    let json = load_fixture("fixtures/agents/gemini_agent.json");
    let agent: AgentDefinition =
        serde_json::from_str(&json).expect("gemini_agent.json should deserialize");

    assert_eq!(agent.id, "agent_gemini");
    assert_eq!(agent.provider.provider_type, ProviderType::Google);
    assert_eq!(agent.provider.model, "gemini-2.5-flash");
}

#[test]
fn parse_cohere_agent_fixture() {
    let json = load_fixture("fixtures/agents/cohere_agent.json");
    let agent: AgentDefinition =
        serde_json::from_str(&json).expect("cohere_agent.json should deserialize");

    assert_eq!(agent.id, "agent_cohere");
    assert_eq!(agent.provider.provider_type, ProviderType::Cohere);
    assert_eq!(agent.provider.model, "command-a-03-2025");
}

#[test]
fn simple_agent_roundtrip() {
    let json = load_fixture("fixtures/agents/simple_agent.json");
    let agent: AgentDefinition = serde_json::from_str(&json).unwrap();
    let serialized = serde_json::to_string(&agent).unwrap();
    let roundtripped: AgentDefinition = serde_json::from_str(&serialized).unwrap();
    assert_eq!(agent, roundtripped);
}

#[test]
fn full_agent_roundtrip() {
    let json = load_fixture("fixtures/agents/full_agent.json");
    let agent: AgentDefinition = serde_json::from_str(&json).unwrap();
    let serialized = serde_json::to_string(&agent).unwrap();
    let roundtripped: AgentDefinition = serde_json::from_str(&serialized).unwrap();
    assert_eq!(agent, roundtripped);
}
