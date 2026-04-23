use pretty_assertions::assert_eq;

use crate::agent::{AgentConfig, AgentSummary};

#[test]
fn agent_config_default_is_all_none() {
    let config = AgentConfig::default();
    assert!(config.max_tokens.is_none());
    assert!(config.max_turns.is_none());
    assert!(config.temperature.is_none());
    assert!(config.json_schema.is_none());
    assert!(config.rate_limit_rpm.is_none());
}

#[test]
fn agent_config_roundtrip_with_all_fields() {
    let config = AgentConfig {
        max_tokens: Some(2048),
        max_turns: Some(5),
        temperature: Some(0.9),
        json_schema: Some(serde_json::json!({"type": "string"})),
        rate_limit_rpm: Some(120),
        compaction: None,
        max_tasks_per_conversation: Some(100),
        max_concurrent_conversations: Some(50),
        tool_calls_only: None,
        immortal: None,
        history_strip: None,
        disabled_tools: vec!["bash".to_string(), "write".to_string()],
        tool_requirements: vec![],
        subagent_timeout_foreground_secs: Some(60),
        subagent_timeout_background_secs: Some(600),
    };

    let json = serde_json::to_string_pretty(&config).expect("serialize AgentConfig");
    let deserialized: AgentConfig = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(config, deserialized);
}

#[test]
fn agent_config_skip_serializing_none_fields() {
    let config = AgentConfig::default();
    let json = serde_json::to_string(&config).expect("serialize default AgentConfig");
    assert_eq!(json, "{}");
}

// ──────────────────────────────────────────────
// AgentSummary
// ──────────────────────────────────────────────

#[test]
fn agent_summary_roundtrip_with_version() {
    let summary = AgentSummary {
        id: "agent-1".to_string(),
        name: "Agent One".to_string(),
        version: Some("2.0.0".to_string()),
    };

    let json = serde_json::to_string_pretty(&summary).expect("serialize");
    let deserialized: AgentSummary = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(summary, deserialized);
}

#[test]
fn agent_summary_roundtrip_without_version() {
    let json = r#"{"id": "agent-2", "name": "Agent Two"}"#;
    let summary: AgentSummary = serde_json::from_str(json).expect("deserialize");
    assert_eq!(summary.id, "agent-2");
    assert!(summary.version.is_none());

    let json2 = serde_json::to_string_pretty(&summary).expect("re-serialize");
    let summary2: AgentSummary = serde_json::from_str(&json2).expect("re-deserialize");
    assert_eq!(summary, summary2);
}

// ──────────────────────────────────────────────
// ProviderType
// ──────────────────────────────────────────────
