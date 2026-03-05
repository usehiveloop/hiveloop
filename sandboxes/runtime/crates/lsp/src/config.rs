use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// User-defined LSP server configuration.
///
/// Allows users to add custom LSP servers or override built-in ones
/// via the runtime config file.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LspServerConfig {
    /// Command and arguments to launch the server (e.g., ["rust-analyzer"])
    pub command: Vec<String>,

    /// File extensions this server handles (e.g., ["rs"])
    #[serde(default)]
    pub extensions: Vec<String>,

    /// Environment variables to set when spawning the server
    #[serde(default)]
    pub env: HashMap<String, String>,

    /// Custom initialization options sent to the server
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub initialization_options: Option<serde_json::Value>,

    /// Whether this server is disabled
    #[serde(default)]
    pub disabled: bool,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_deserialize_full() {
        let json = r#"{
            "command": ["rust-analyzer"],
            "extensions": ["rs"],
            "env": {"RUST_LOG": "debug"},
            "initialization_options": {"checkOnSave": true},
            "disabled": false
        }"#;
        let config: LspServerConfig = serde_json::from_str(json).unwrap();
        assert_eq!(config.command, vec!["rust-analyzer"]);
        assert_eq!(config.extensions, vec!["rs"]);
        assert_eq!(config.env.get("RUST_LOG").unwrap(), "debug");
        assert!(config.initialization_options.is_some());
        assert!(!config.disabled);
    }

    #[test]
    fn test_deserialize_minimal() {
        let json = r#"{"command": ["gopls"]}"#;
        let config: LspServerConfig = serde_json::from_str(json).unwrap();
        assert_eq!(config.command, vec!["gopls"]);
        assert!(config.extensions.is_empty());
        assert!(config.env.is_empty());
        assert!(config.initialization_options.is_none());
        assert!(!config.disabled);
    }

    #[test]
    fn test_disabled_flag() {
        let json = r#"{"command": ["gopls"], "disabled": true}"#;
        let config: LspServerConfig = serde_json::from_str(json).unwrap();
        assert!(config.disabled);
    }
}
