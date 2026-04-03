use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Runtime configuration for the bridge binary.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeConfig {
    /// URL of the control plane API
    pub control_plane_url: String,
    /// API key for authenticating with the control plane
    pub control_plane_api_key: String,
    /// Address to listen on (e.g., "0.0.0.0:8080")
    pub listen_addr: String,
    /// Maximum time in seconds to wait for graceful drain
    pub drain_timeout_secs: u64,
    /// Maximum number of concurrent conversations (None = unlimited)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_concurrent_conversations: Option<usize>,
    /// Log level (e.g., "info", "debug", "warn")
    pub log_level: String,
    /// Log output format
    pub log_format: LogFormat,
    /// LSP configuration.
    /// Can be `false` to disable all LSP, or a map of server configs.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub lsp: Option<LspConfig>,
    /// Optional webhook URL. When set, all SSE events are also dispatched as
    /// webhooks to this URL, signed with the control plane API key.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub webhook_url: Option<String>,

    /// Maximum concurrent outbound LLM API calls across all agents.
    /// Controls the global ceiling on simultaneous requests to LLM providers.
    /// Default: 500 when not set.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_concurrent_llm_calls: Option<usize>,

    /// Webhook delivery configuration. Ignored when webhook_url is not set.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub webhook_config: Option<WebhookConfig>,

    /// Enable the WebSocket event stream endpoint (`/ws/events`).
    /// When true, all events are broadcast over a single WebSocket connection.
    /// Configured via `BRIDGE_WEBSOCKET_ENABLED` env var.
    #[serde(default)]
    pub websocket_enabled: bool,

    /// Replace built-in filesystem search tools (Grep, Read, Glob) with codedb MCP server.
    /// When true, codedb is auto-injected as an MCP server for all agents and
    /// the built-in Grep, Read, and Glob tools are not registered.
    /// Configured via `BRIDGE_CODEDB_ENABLED` env var.
    #[serde(default)]
    pub codedb_enabled: bool,

    /// Path to the codedb binary. Defaults to "codedb" (looked up in PATH).
    /// Configured via `BRIDGE_CODEDB_BINARY` env var.
    #[serde(default = "default_codedb_binary")]
    pub codedb_binary: String,

    /// OpenTelemetry OTLP endpoint for trace export.
    /// When set, Bridge exports all spans via OTLP gRPC to this endpoint.
    /// Configured via `BRIDGE_OTEL_ENDPOINT` env var.
    /// Example: `http://localhost:4317` (Jaeger, Grafana Tempo, Datadog Agent, etc.)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub otel_endpoint: Option<String>,

    /// OpenTelemetry service name. Defaults to "bridge".
    /// Configured via `BRIDGE_OTEL_SERVICE_NAME` env var.
    #[serde(default = "default_otel_service_name")]
    pub otel_service_name: String,
}

/// Webhook delivery configuration for tuning throughput and resilience.
///
/// The internal queue is unbounded (zero data loss guarantee), so there is
/// no channel capacity setting. Memory is the buffer — webhook payloads are
/// ~1KB each so even 100K queued events is only ~100MB.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookConfig {
    /// Max concurrent HTTP deliveries. Default: 50.
    #[serde(default = "default_webhook_max_concurrent")]
    pub max_concurrent_deliveries: usize,
    /// Max idle HTTP connections per host. Default: 20.
    #[serde(default = "default_webhook_max_idle")]
    pub max_idle_connections: usize,
    /// Delivery timeout in seconds. Default: 10.
    #[serde(default = "default_webhook_delivery_timeout")]
    pub delivery_timeout_secs: u64,
    /// Max retry attempts. Default: 5.
    #[serde(default = "default_webhook_max_retries")]
    pub max_retries: usize,
    /// How long a per-conversation delivery worker stays alive with no events,
    /// in seconds. Default: 300 (5 minutes).
    #[serde(default = "default_webhook_worker_idle_timeout")]
    pub worker_idle_timeout_secs: u64,
}

impl Default for WebhookConfig {
    fn default() -> Self {
        Self {
            max_concurrent_deliveries: default_webhook_max_concurrent(),
            max_idle_connections: default_webhook_max_idle(),
            delivery_timeout_secs: default_webhook_delivery_timeout(),
            max_retries: default_webhook_max_retries(),
            worker_idle_timeout_secs: default_webhook_worker_idle_timeout(),
        }
    }
}

fn default_webhook_max_concurrent() -> usize {
    50
}
fn default_webhook_max_idle() -> usize {
    20
}
fn default_webhook_delivery_timeout() -> u64 {
    10
}
fn default_webhook_max_retries() -> usize {
    5
}
fn default_webhook_worker_idle_timeout() -> u64 {
    300
}
fn default_codedb_binary() -> String {
    "codedb".to_string()
}
fn default_otel_service_name() -> String {
    "bridge".to_string()
}

/// LSP configuration: either disabled entirely or per-server config map.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum LspConfig {
    /// Set to `false` to disable all LSP servers
    Disabled(bool),
    /// Per-server configuration map keyed by server ID
    Servers(HashMap<String, LspServerConfig>),
}

impl LspConfig {
    /// Returns true if LSP is explicitly disabled.
    pub fn is_disabled(&self) -> bool {
        matches!(self, LspConfig::Disabled(false))
    }

    /// Extract the server config map, or None if disabled.
    pub fn into_servers(self) -> Option<HashMap<String, LspServerConfig>> {
        match self {
            LspConfig::Disabled(false) => None,
            LspConfig::Disabled(true) => Some(HashMap::new()),
            LspConfig::Servers(map) => Some(map),
        }
    }
}

/// User-defined LSP server configuration entry.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LspServerConfig {
    /// Command and arguments to launch the server
    pub command: Vec<String>,
    /// File extensions this server handles
    #[serde(default)]
    pub extensions: Vec<String>,
    /// Environment variables for the server process
    #[serde(default)]
    pub env: HashMap<String, String>,
    /// Custom initialization options
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub initialization_options: Option<serde_json::Value>,
    /// Whether this server is disabled
    #[serde(default)]
    pub disabled: bool,
}

/// Log output format.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum LogFormat {
    /// Human-readable text format
    Text,
    /// Structured JSON format
    Json,
}

impl Default for RuntimeConfig {
    fn default() -> Self {
        Self {
            control_plane_url: String::new(),
            control_plane_api_key: String::new(),
            listen_addr: "0.0.0.0:8080".to_string(),
            drain_timeout_secs: 60,
            max_concurrent_conversations: None,
            log_level: "info".to_string(),
            log_format: LogFormat::Text,
            lsp: None,
            webhook_url: None,
            max_concurrent_llm_calls: None,
            webhook_config: None,
            websocket_enabled: false,
            codedb_enabled: false,
            codedb_binary: default_codedb_binary(),
            otel_endpoint: None,
            otel_service_name: default_otel_service_name(),
        }
    }
}

#[cfg(test)]
mod tests {
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
    fn test_codedb_defaults() {
        let config = RuntimeConfig::default();
        assert!(!config.codedb_enabled);
        assert_eq!(config.codedb_binary, "codedb");
    }

    #[test]
    fn test_codedb_enabled_from_json() {
        let json = r#"{
            "control_plane_url": "http://localhost",
            "control_plane_api_key": "key",
            "listen_addr": "0.0.0.0:8080",
            "drain_timeout_secs": 60,
            "log_level": "info",
            "log_format": "text",
            "codedb_enabled": true,
            "codedb_binary": "/usr/local/bin/codedb"
        }"#;
        let config: RuntimeConfig = serde_json::from_str(json).unwrap();
        assert!(config.codedb_enabled);
        assert_eq!(config.codedb_binary, "/usr/local/bin/codedb");
    }

    #[test]
    fn test_backwards_compat_without_codedb_fields() {
        let json = r#"{
            "control_plane_url": "http://localhost",
            "control_plane_api_key": "key",
            "listen_addr": "0.0.0.0:8080",
            "drain_timeout_secs": 60,
            "log_level": "info",
            "log_format": "text"
        }"#;
        let config: RuntimeConfig = serde_json::from_str(json).unwrap();
        assert!(!config.codedb_enabled);
        assert_eq!(config.codedb_binary, "codedb");
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
}
