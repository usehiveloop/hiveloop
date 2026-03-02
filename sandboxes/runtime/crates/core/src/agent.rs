use serde::{Deserialize, Serialize};

use crate::mcp::McpServerDefinition;
use crate::provider::ProviderConfig;
use crate::skill::SkillDefinition;
use crate::tool::ToolDefinition;

/// Type alias for agent identifiers.
pub type AgentId = String;

/// Complete definition of an AI agent fetched from the control plane.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct AgentDefinition {
    /// Unique agent identifier
    pub id: AgentId,
    /// Human-readable agent name
    pub name: String,
    /// System prompt for the agent
    pub system_prompt: String,
    /// LLM provider configuration
    pub provider: ProviderConfig,
    /// Agent-defined tools
    #[serde(default)]
    pub tools: Vec<ToolDefinition>,
    /// MCP server connections
    #[serde(default)]
    pub mcp_servers: Vec<McpServerDefinition>,
    /// Available skills
    #[serde(default)]
    pub skills: Vec<SkillDefinition>,
    /// Agent configuration options
    #[serde(default)]
    pub config: AgentConfig,
    /// Nested subagent definitions
    #[serde(default)]
    pub subagents: Vec<AgentDefinition>,
    /// Webhook URL for event delivery
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub webhook_url: Option<String>,
    /// Webhook secret for HMAC signing
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub webhook_secret: Option<String>,
    /// Version field for change detection during sync
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
    /// Last updated timestamp for change detection
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<String>,
}

/// Configuration options for an agent.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub struct AgentConfig {
    /// Maximum tokens for LLM response
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    /// Maximum conversation turns
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_turns: Option<u32>,
    /// Temperature for LLM sampling
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,
    /// JSON schema for structured output
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub json_schema: Option<serde_json::Value>,
    /// Rate limit in requests per minute
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub rate_limit_rpm: Option<u32>,
}

/// Lightweight agent summary for listing endpoints.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct AgentSummary {
    /// Agent identifier
    pub id: AgentId,
    /// Agent name
    pub name: String,
    /// Agent version
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
}

#[cfg(test)]
mod tests {
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
        assert!(agent.provider.base_url.is_none());
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
        assert_eq!(agent.skills.len(), 1);
        assert_eq!(agent.skills[0].id, "skill_code_review");

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
    fn parse_multi_provider_fixture() {
        let json = load_fixture("fixtures/agents/multi_provider.json");
        let agent: AgentDefinition =
            serde_json::from_str(&json).expect("multi_provider.json should deserialize");

        assert_eq!(agent.id, "agent_anthropic");
        assert_eq!(agent.name, "Anthropic Agent");
        assert_eq!(agent.provider.provider_type, ProviderType::Anthropic);
        assert_eq!(agent.provider.model, "claude-sonnet-4-20250514");
        assert!(agent.provider.base_url.is_none());
        assert_eq!(agent.config.max_tokens, Some(8192));
        assert_eq!(agent.config.temperature, Some(0.5));
        assert!(agent.config.max_turns.is_none());
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
}
