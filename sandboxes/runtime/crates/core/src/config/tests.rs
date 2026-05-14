use super::*;

#[test]
fn test_default_values() {
    let config = RuntimeConfig::default();
    assert_eq!(config.listen_addr, "0.0.0.0:8080");
    assert_eq!(config.drain_timeout_secs, 60);
    assert!(config.max_concurrent_conversations.is_none());
    assert_eq!(config.log_level, "info");
    assert_eq!(config.log_format, LogFormat::Text);
}

#[test]
fn test_lsp_config_disabled() {
    let json = r#"false"#;
    let config: LspConfig = serde_json::from_str(json).unwrap();
    assert!(config.is_disabled());
    assert!(config.into_servers().is_none());
}

#[test]
fn test_lsp_config_servers() {
    let json = r#"{"rust": {"command": ["rust-analyzer"]}}"#;
    let config: LspConfig = serde_json::from_str(json).unwrap();
    assert!(!config.is_disabled());
    let servers = config.into_servers().unwrap();
    assert!(servers.contains_key("rust"));
}

#[test]
fn test_lsp_config_in_runtime_config() {
    let json = r#"{
        "control_plane_url": "http://localhost",
        "control_plane_api_key": "key",
        "listen_addr": "0.0.0.0:8080",
        "drain_timeout_secs": 60,
        "log_level": "info",
        "log_format": "text",
        "lsp": false
    }"#;
    let config: RuntimeConfig = serde_json::from_str(json).unwrap();
    assert!(config.lsp.as_ref().unwrap().is_disabled());
}

#[test]
fn test_lsp_config_with_servers_in_runtime_config() {
    let json = r#"{
        "control_plane_url": "http://localhost",
        "control_plane_api_key": "key",
        "listen_addr": "0.0.0.0:8080",
        "drain_timeout_secs": 60,
        "log_level": "info",
        "log_format": "text",
        "lsp": {
            "custom": {
                "command": ["my-lsp", "--stdio"],
                "extensions": ["xyz"]
            }
        }
    }"#;
    let config: RuntimeConfig = serde_json::from_str(json).unwrap();
    let servers = config.lsp.unwrap().into_servers().unwrap();
    assert!(servers.contains_key("custom"));
}

// ── Fix #3/#5: New config fields tests ─────────────────────────────

#[test]
fn test_default_new_capacity_fields_are_none() {
    let config = RuntimeConfig::default();
    assert!(config.max_concurrent_llm_calls.is_none());
    assert!(config.webhook_config.is_none());
}

#[test]
fn test_webhook_config_defaults() {
    let config = WebhookConfig::default();
    assert_eq!(config.max_concurrent_deliveries, 50);
    assert_eq!(config.max_idle_connections, 20);
    assert_eq!(config.delivery_timeout_secs, 10);
    assert_eq!(config.max_retries, 5);
}

#[test]
fn test_webhook_config_serde_roundtrip() {
    let config = WebhookConfig {
        max_concurrent_deliveries: 100,
        max_idle_connections: 10,
        delivery_timeout_secs: 30,
        max_retries: 3,
        worker_idle_timeout_secs: 300,
    };
    let json = serde_json::to_string(&config).unwrap();
    let deserialized: WebhookConfig = serde_json::from_str(&json).unwrap();
    assert_eq!(deserialized.max_concurrent_deliveries, 100);
    assert_eq!(deserialized.max_retries, 3);
}

#[test]
fn test_runtime_config_with_all_new_fields() {
    let json = r#"{
        "control_plane_url": "http://localhost",
        "control_plane_api_key": "key",
        "listen_addr": "0.0.0.0:8080",
        "drain_timeout_secs": 60,
        "log_level": "info",
        "log_format": "text",
        "max_concurrent_llm_calls": 200,
        "webhook_config": {
            "max_concurrent_deliveries": 25
        }
    }"#;
    let config: RuntimeConfig = serde_json::from_str(json).unwrap();
    assert_eq!(config.max_concurrent_llm_calls, Some(200));
    let wh = config.webhook_config.unwrap();
    assert_eq!(wh.max_concurrent_deliveries, 25);
    // Defaults for unset fields
    assert_eq!(wh.max_idle_connections, 20);
    assert_eq!(wh.max_retries, 5);
}

#[test]
fn test_runtime_config_backwards_compatible_without_new_fields() {
    // Old configs without the new fields should still deserialize
    let json = r#"{
        "control_plane_url": "http://localhost",
        "control_plane_api_key": "key",
        "listen_addr": "0.0.0.0:8080",
        "drain_timeout_secs": 60,
        "log_level": "info",
        "log_format": "text"
    }"#;
    let config: RuntimeConfig = serde_json::from_str(json).unwrap();
    assert!(config.max_concurrent_llm_calls.is_none());
    assert!(config.webhook_config.is_none());
}
