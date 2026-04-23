use pretty_assertions::assert_eq;
use std::collections::HashMap;

use crate::agent::{AgentConfig, AgentDefinition};
use crate::mcp::{McpServerDefinition, McpTransport};
use crate::provider::{ProviderConfig, ProviderType};
use crate::skill::SkillDefinition;
use crate::tool::ToolDefinition;

#[test]
fn agent_definition_roundtrip_all_fields_present() {
    let agent = AgentDefinition {
        id: "agent-001".to_string(),
        name: "Test Agent".to_string(),
        description: Some("A test agent for roundtrip testing".to_string()),
        system_prompt: "You are a helpful assistant.".to_string(),
        provider: ProviderConfig {
            provider_type: ProviderType::Anthropic,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: "<provider-api-key>-key".to_string(),
            base_url: Some("https://api.anthropic.com".to_string()),
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        },
        tools: vec![ToolDefinition {
            name: "calculator".to_string(),
            description: "Performs arithmetic".to_string(),
            parameters_schema: serde_json::json!({
                "type": "object",
                "properties": {
                    "expression": { "type": "string" }
                }
            }),
        }],
        mcp_servers: vec![McpServerDefinition {
            name: "filesystem".to_string(),
            transport: McpTransport::Stdio {
                command: "mcp-fs".to_string(),
                args: vec!["--root".to_string(), "/tmp".to_string()],
                env: HashMap::from([("HOME".to_string(), "/root".to_string())]),
            },
        }],
        skills: vec![SkillDefinition {
            id: "skill-1".to_string(),
            title: "Code Review".to_string(),
            description: "Reviews code for quality".to_string(),
            content: "You are a code review expert.".to_string(),
            ..Default::default()
        }],
        integrations: vec![],
        config: AgentConfig {
            max_tokens: Some(4096),
            max_turns: Some(10),
            temperature: Some(0.7),
            json_schema: Some(serde_json::json!({"type": "object"})),
            rate_limit_rpm: Some(60),
            compaction: None,
            max_tasks_per_conversation: None,
            max_concurrent_conversations: None,
            tool_calls_only: None,
            immortal: None,
            history_strip: None,
            disabled_tools: vec![],
            tool_requirements: vec![],
            subagent_timeout_foreground_secs: None,
            subagent_timeout_background_secs: None,
        },
        subagents: vec![AgentDefinition {
            id: "sub-agent-001".to_string(),
            name: "Sub Agent".to_string(),
            description: Some("A sub agent for testing".to_string()),
            system_prompt: "Sub agent prompt".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "sk-openai-key".to_string(),
                base_url: None,
                prompt_caching_enabled: true,
                cache_ttl: Default::default(),
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            config: AgentConfig::default(),
            subagents: vec![],
            integrations: vec![],
            permissions: HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        }],
        permissions: HashMap::new(),
        webhook_url: Some("https://example.com/webhook".to_string()),
        webhook_secret: Some("<webhook-secret>".to_string()),
        version: Some("1.0.0".to_string()),
        updated_at: Some("2026-03-02T00:00:00Z".to_string()),
    };

    let json = serde_json::to_string_pretty(&agent).expect("serialize AgentDefinition");
    let deserialized: AgentDefinition =
        serde_json::from_str(&json).expect("deserialize AgentDefinition");
    assert_eq!(agent, deserialized);
}

#[test]
fn agent_definition_roundtrip_optional_fields_absent() {
    let json = r#"{
        "id": "agent-002",
        "name": "Minimal Agent",
        "system_prompt": "Be helpful.",
        "provider": {
            "provider_type": "open_ai",
            "model": "gpt-4o",
            "api_key": "<provider-api-key>"
        }
    }"#;

    let agent: AgentDefinition =
        serde_json::from_str(json).expect("deserialize minimal AgentDefinition");
    assert_eq!(agent.id, "agent-002");
    assert_eq!(agent.name, "Minimal Agent");
    assert!(agent.tools.is_empty());
    assert!(agent.mcp_servers.is_empty());
    assert!(agent.skills.is_empty());
    assert!(agent.subagents.is_empty());
    assert!(agent.webhook_url.is_none());
    assert!(agent.webhook_secret.is_none());
    assert!(agent.version.is_none());
    assert!(agent.updated_at.is_none());
    assert_eq!(agent.config, AgentConfig::default());

    // Re-serialize and deserialize to confirm roundtrip
    let json2 = serde_json::to_string_pretty(&agent).expect("re-serialize");
    let agent2: AgentDefinition = serde_json::from_str(&json2).expect("re-deserialize");
    assert_eq!(agent, agent2);
}

#[test]
fn agent_definition_skip_serializing_none_optional_fields() {
    let agent = AgentDefinition {
        id: "agent-003".to_string(),
        name: "No Optionals".to_string(),
        description: None,
        system_prompt: "Prompt".to_string(),
        provider: ProviderConfig {
            provider_type: ProviderType::Google,
            model: "gemini-pro".to_string(),
            api_key: "key".to_string(),
            base_url: None,
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        },
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        config: AgentConfig::default(),
        subagents: vec![],
        permissions: HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };

    let json = serde_json::to_string(&agent).expect("serialize");
    // None fields should be skipped
    assert!(!json.contains("description"));
    assert!(!json.contains("webhook_url"));
    assert!(!json.contains("webhook_secret"));
    assert!(!json.contains("version"));
    assert!(!json.contains("updated_at"));
}
