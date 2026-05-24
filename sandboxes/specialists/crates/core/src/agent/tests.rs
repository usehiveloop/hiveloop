use super::*;
use crate::provider::ProviderType;

const SIMPLE_AGENT: &str = r#"{
    "id": "agent_simple",
    "name": "Simple Agent",
    "harness": "open_code",
    "system_prompt": "You are a helpful assistant.",
    "provider": {
        "provider_type": "open_ai",
        "model": "gpt-4o",
        "api_key": "<provider-api-key>"
    }
}"#;

#[test]
fn parse_simple_agent() {
    let agent: AgentDefinition =
        serde_json::from_str(SIMPLE_AGENT).expect("simple agent JSON should deserialize");

    assert_eq!(agent.id, "agent_simple");
    assert_eq!(agent.name, "Simple Agent");
    assert_eq!(agent.harness, Harness::OpenCode);
    assert_eq!(agent.system_prompt, "You are a helpful assistant.");
    assert_eq!(agent.provider.provider_type, ProviderType::OpenAI);
    assert_eq!(agent.provider.model, "gpt-4o");
    assert!(agent.mcp_servers.is_empty());
    assert!(agent.skills.is_empty());
    assert!(agent.permissions.is_empty());
    assert!(agent.webhook_url.is_none());
}

#[test]
fn rejects_removed_claude_harness() {
    let json = r#"{
        "id": "agent_old",
        "name": "Old Agent",
        "harness": "claude",
        "system_prompt": "hi",
        "provider": {
            "provider_type": "open_ai",
            "model": "gpt-4o",
            "api_key": "k"
        }
    }"#;
    let err = serde_json::from_str::<AgentDefinition>(json).unwrap_err();
    assert!(err.to_string().contains("unknown variant"));
}

#[test]
fn agent_config_full_payload_roundtrip() {
    let json = r#"{
        "id": "ag",
        "name": "Ag",
        "harness": "open_code",
        "system_prompt": "hi",
        "provider": {
            "provider_type": "open_ai",
            "model": "gpt-4o",
            "api_key": "k"
        },
        "config": {
            "max_tokens": 8192,
            "max_turns": 50,
            "temperature": 0.3,
            "reasoning_effort": "high",
            "small_fast_model": "gpt-4o-mini",
            "fallback_model": "gpt-4.1",
            "allowed_tools": ["Read", "Bash"],
            "disabled_tools": ["WebFetch"],
            "permission_mode": "acceptEdits",
            "env": {"FOO": "bar"}
        }
    }"#;
    let agent: AgentDefinition = serde_json::from_str(json).unwrap();
    assert_eq!(agent.config.max_tokens, Some(8192));
    assert_eq!(agent.config.reasoning_effort.as_deref(), Some("high"));
    assert_eq!(
        agent.config.small_fast_model.as_deref(),
        Some("gpt-4o-mini")
    );
    assert_eq!(agent.config.allowed_tools, vec!["Read", "Bash"]);
    assert_eq!(agent.config.disabled_tools, vec!["WebFetch"]);
    assert_eq!(agent.config.permission_mode.as_deref(), Some("acceptEdits"));
    assert_eq!(agent.config.env.get("FOO").map(|s| s.as_str()), Some("bar"));

    let serialized = serde_json::to_string(&agent).unwrap();
    let roundtripped: AgentDefinition = serde_json::from_str(&serialized).unwrap();
    assert_eq!(agent, roundtripped);
}

#[test]
fn validate_rejects_empty_required_fields() {
    let mut agent: AgentDefinition = serde_json::from_str(SIMPLE_AGENT).unwrap();
    assert!(agent.validate().is_ok());

    agent.id.clear();
    assert!(agent.validate().is_err());
}
