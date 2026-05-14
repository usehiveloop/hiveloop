use pretty_assertions::assert_eq;

use crate::config::{LogFormat, RuntimeConfig};
use crate::metrics::{AgentMetrics, GlobalMetrics, MetricsResponse, MetricsSnapshot};

#[test]
fn metrics_snapshot_serializes_correctly() {
    let snapshot = MetricsSnapshot {
        agent_id: "agent-1".to_string(),
        agent_name: "Test Agent".to_string(),
        input_tokens: 500,
        cached_input_tokens: 0,
        output_tokens: 200,
        total_tokens: 700,
        cache_hit_ratio: 0.0,
        total_requests: 10,
        failed_requests: 1,
        active_conversations: 3,
        total_conversations: 15,
        tool_calls: 25,
        avg_latency_ms: 150.5,
        tool_call_details: vec![],
    };

    let json = serde_json::to_string_pretty(&snapshot).expect("serialize MetricsSnapshot");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");

    assert_eq!(value["agent_id"], "agent-1");
    assert_eq!(value["agent_name"], "Test Agent");
    assert_eq!(value["input_tokens"], 500);
    assert_eq!(value["output_tokens"], 200);
    assert_eq!(value["total_tokens"], 700);
    assert_eq!(value["total_requests"], 10);
    assert_eq!(value["failed_requests"], 1);
    assert_eq!(value["active_conversations"], 3);
    assert_eq!(value["total_conversations"], 15);
    assert_eq!(value["tool_calls"], 25);
    assert_eq!(value["avg_latency_ms"], 150.5);
}

#[test]
fn metrics_snapshot_total_tokens_equals_input_plus_output() {
    let metrics = AgentMetrics::new();
    metrics
        .input_tokens
        .store(1234, std::sync::atomic::Ordering::Relaxed);
    metrics
        .output_tokens
        .store(5678, std::sync::atomic::Ordering::Relaxed);

    let snap = metrics.snapshot("a", "A");
    assert_eq!(snap.total_tokens, 1234 + 5678);
    assert_eq!(snap.total_tokens, snap.input_tokens + snap.output_tokens);
}

#[test]
fn metrics_snapshot_avg_latency_computation() {
    let metrics = AgentMetrics::new();
    metrics
        .latency_sum_ms
        .store(3000, std::sync::atomic::Ordering::Relaxed);
    metrics
        .latency_count
        .store(12, std::sync::atomic::Ordering::Relaxed);

    let snap = metrics.snapshot("a", "A");
    assert!((snap.avg_latency_ms - 250.0).abs() < f64::EPSILON);
}

#[test]
fn metrics_snapshot_avg_latency_zero_when_no_measurements() {
    let metrics = AgentMetrics::new();
    let snap = metrics.snapshot("a", "A");
    assert!((snap.avg_latency_ms - 0.0).abs() < f64::EPSILON);
}

// ──────────────────────────────────────────────
// GlobalMetrics
// ──────────────────────────────────────────────

#[test]
fn global_metrics_serializes_correctly() {
    let global = GlobalMetrics {
        total_agents: 5,
        total_active_conversations: 12,
        uptime_secs: 3600,
    };

    let json = serde_json::to_string_pretty(&global).expect("serialize");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["total_agents"], 5);
    assert_eq!(value["total_active_conversations"], 12);
    assert_eq!(value["uptime_secs"], 3600);
}

// ──────────────────────────────────────────────
// MetricsResponse
// ──────────────────────────────────────────────

#[test]
fn metrics_response_serializes_correctly() {
    let response = MetricsResponse {
        timestamp: chrono::Utc::now(),
        agents: vec![MetricsSnapshot {
            agent_id: "agent-1".to_string(),
            agent_name: "Agent One".to_string(),
            input_tokens: 100,
            cached_input_tokens: 0,
            output_tokens: 50,
            total_tokens: 150,
            cache_hit_ratio: 0.0,
            total_requests: 5,
            failed_requests: 0,
            active_conversations: 1,
            total_conversations: 3,
            tool_calls: 10,
            avg_latency_ms: 200.0,
            tool_call_details: vec![],
        }],
        global: GlobalMetrics {
            total_agents: 1,
            total_active_conversations: 1,
            uptime_secs: 120,
        },
    };

    let json = serde_json::to_string_pretty(&response).expect("serialize MetricsResponse");
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert!(value["timestamp"].is_string());
    assert!(value["agents"].is_array());
    assert_eq!(value["agents"].as_array().unwrap().len(), 1);
    assert!(value["global"].is_object());
}

// ──────────────────────────────────────────────
// RuntimeConfig
// ──────────────────────────────────────────────

#[test]
fn runtime_config_default_impl() {
    let config = RuntimeConfig::default();
    assert_eq!(config.control_plane_url, "");
    assert_eq!(config.control_plane_api_key, "");
    assert_eq!(config.listen_addr, "0.0.0.0:8080");
    assert_eq!(config.drain_timeout_secs, 60);
    assert!(config.max_concurrent_conversations.is_none());
    assert_eq!(config.log_level, "info");
    assert_eq!(config.log_format, LogFormat::Text);
}

#[test]
fn runtime_config_roundtrip_all_fields() {
    let config = RuntimeConfig {
        control_plane_url: "https://api.example.com".to_string(),
        control_plane_api_key: "cpk-test-key".to_string(),
        listen_addr: "127.0.0.1:9090".to_string(),
        drain_timeout_secs: 120,
        max_concurrent_conversations: Some(100),
        log_level: "debug".to_string(),
        log_format: LogFormat::Json,
        lsp: None,
        webhook_url: None,
        max_concurrent_llm_calls: Some(500),
        webhook_config: None,
        websocket_enabled: false,
        skill_discovery_enabled: false,
        skill_discovery_dir: None,
        allow_stdio_mcp_from_api: false,
        standalone_agent: false,
        otel_endpoint: None,
        otel_service_name: "bridge".to_string(),
    };

    let json = serde_json::to_string_pretty(&config).expect("serialize RuntimeConfig");
    let deserialized: RuntimeConfig = serde_json::from_str(&json).expect("deserialize");

    assert_eq!(config.control_plane_url, deserialized.control_plane_url);
    assert_eq!(
        config.control_plane_api_key,
        deserialized.control_plane_api_key
    );
    assert_eq!(config.listen_addr, deserialized.listen_addr);
    assert_eq!(config.drain_timeout_secs, deserialized.drain_timeout_secs);
    assert_eq!(
        config.max_concurrent_conversations,
        deserialized.max_concurrent_conversations
    );
    assert_eq!(config.log_level, deserialized.log_level);
    assert_eq!(config.log_format, deserialized.log_format);
}

#[test]
fn runtime_config_deserialize_with_missing_optional_fields() {
    let json = r#"{
        "control_plane_url": "https://api.example.com",
        "control_plane_api_key": "key",
        "listen_addr": "0.0.0.0:8080",
        "drain_timeout_secs": 60,
        "log_level": "info",
        "log_format": "text"
    }"#;

    let config: RuntimeConfig = serde_json::from_str(json).expect("deserialize");
    assert!(config.max_concurrent_conversations.is_none());
}

#[test]
fn runtime_config_skip_serializing_none_max_concurrent() {
    let config = RuntimeConfig {
        max_concurrent_conversations: None,
        ..RuntimeConfig::default()
    };

    let json = serde_json::to_string(&config).expect("serialize");
    assert!(!json.contains("max_concurrent_conversations"));
}

// ──────────────────────────────────────────────
// LogFormat
// ──────────────────────────────────────────────

#[test]
fn log_format_roundtrip() {
    let text_json = serde_json::to_string(&LogFormat::Text).expect("serialize");
    assert_eq!(text_json, "\"text\"");
    let text: LogFormat = serde_json::from_str(&text_json).expect("deserialize");
    assert_eq!(text, LogFormat::Text);

    let json_json = serde_json::to_string(&LogFormat::Json).expect("serialize");
    assert_eq!(json_json, "\"json\"");
    let json_val: LogFormat = serde_json::from_str(&json_json).expect("deserialize");
    assert_eq!(json_val, LogFormat::Json);
}

// ──────────────────────────────────────────────
// BridgeError Display
// ──────────────────────────────────────────────
