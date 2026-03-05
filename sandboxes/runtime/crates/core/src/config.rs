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
}
