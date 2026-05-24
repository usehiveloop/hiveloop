use serde::{Deserialize, Serialize};
use std::collections::HashMap;

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
