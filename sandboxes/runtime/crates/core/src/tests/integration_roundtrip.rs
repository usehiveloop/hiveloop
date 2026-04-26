use pretty_assertions::assert_eq;
use std::collections::HashMap;

use crate::agent::{AgentConfig, AgentDefinition};
use crate::integration::{IntegrationAction, IntegrationDefinition};
use crate::permission::ToolPermission;
use crate::provider::{ProviderConfig, ProviderType};

#[test]
fn integration_definition_roundtrip() {
    let integration = IntegrationDefinition {
        name: "github".to_string(),
        description: "GitHub integration".to_string(),
        actions: vec![
            IntegrationAction {
                name: "create_pull_request".to_string(),
                description: "Create a new pull request".to_string(),
                parameters_schema: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "title": { "type": "string" },
                        "head": { "type": "string" },
                        "base": { "type": "string" }
                    },
                    "required": ["title", "head", "base"]
                }),
                permission: ToolPermission::RequireApproval,
            },
            IntegrationAction {
                name: "list_issues".to_string(),
                description: "List issues".to_string(),
                parameters_schema: serde_json::json!({"type": "object"}),
                permission: ToolPermission::Allow,
            },
            IntegrationAction {
                name: "delete_repository".to_string(),
                description: "Delete a repository".to_string(),
                parameters_schema: serde_json::json!({"type": "object"}),
                permission: ToolPermission::Deny,
            },
        ],
    };

    let json = serde_json::to_string_pretty(&integration).expect("serialize");
    let deserialized: IntegrationDefinition = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(integration, deserialized);
}

#[test]
fn integration_action_permissions_serialize_correctly() {
    let action = IntegrationAction {
        name: "test".to_string(),
        description: "test".to_string(),
        parameters_schema: serde_json::json!({}),
        permission: ToolPermission::RequireApproval,
    };
    let json = serde_json::to_string(&action).expect("serialize");
    assert!(json.contains("\"require_approval\""));

    let action2 = IntegrationAction {
        name: "test".to_string(),
        description: "test".to_string(),
        parameters_schema: serde_json::json!({}),
        permission: ToolPermission::Deny,
    };
    let json2 = serde_json::to_string(&action2).expect("serialize");
    assert!(json2.contains("\"deny\""));
}

#[test]
fn agent_definition_with_integrations_roundtrip() {
    let agent = AgentDefinition {
        id: "agent-int".to_string(),
        name: "Integration Agent".to_string(),
        description: None,
        system_prompt: "You have integrations.".to_string(),
        provider: ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "key".to_string(),
            base_url: None,
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        },
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![IntegrationDefinition {
            name: "slack".to_string(),
            description: "Slack".to_string(),
            actions: vec![IntegrationAction {
                name: "send_message".to_string(),
                description: "Send a message".to_string(),
                parameters_schema: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "channel": { "type": "string" },
                        "text": { "type": "string" }
                    },
                    "required": ["channel", "text"]
                }),
                permission: ToolPermission::Allow,
            }],
        }],
        artifacts: None,
        config: AgentConfig::default(),
        subagents: vec![],
        permissions: HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };

    let json = serde_json::to_string_pretty(&agent).expect("serialize");
    assert!(json.contains("integrations"));
    assert!(json.contains("slack"));
    assert!(json.contains("send_message"));

    let deserialized: AgentDefinition = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(agent, deserialized);
}

#[test]
fn agent_definition_empty_integrations_omitted_in_json() {
    let agent = AgentDefinition {
        id: "agent-no-int".to_string(),
        name: "No Integrations".to_string(),
        description: None,
        system_prompt: "Prompt".to_string(),
        provider: ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "key".to_string(),
            base_url: None,
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        },
        tools: vec![],
        mcp_servers: vec![],
        skills: vec![],
        integrations: vec![],
        artifacts: None,
        config: AgentConfig::default(),
        subagents: vec![],
        permissions: HashMap::new(),
        webhook_url: None,
        webhook_secret: None,
        version: None,
        updated_at: None,
    };

    let json = serde_json::to_string(&agent).expect("serialize");
    assert!(
        !json.contains("integrations"),
        "empty integrations should be omitted via skip_serializing_if"
    );
}

// ──────────────────────────────────────────────
// BridgeEvent & BridgeEventType
// ──────────────────────────────────────────────
