use serde::{Deserialize, Serialize};

/// Runtime configuration for the bridge binary.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeConfig {
    /// URL of the control plane API
    pub control_plane_url: String,
    /// API key for authenticating with the control plane
    pub control_plane_api_key: String,
    /// Address to listen on (e.g., "0.0.0.0:8080")
    pub listen_addr: String,
    /// Interval in seconds between control plane sync polls
    pub sync_interval_secs: u64,
    /// Maximum time in seconds to wait for graceful drain
    pub drain_timeout_secs: u64,
    /// Maximum number of concurrent conversations (None = unlimited)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_concurrent_conversations: Option<usize>,
    /// Log level (e.g., "info", "debug", "warn")
    pub log_level: String,
    /// Log output format
    pub log_format: LogFormat,
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
            sync_interval_secs: 30,
            drain_timeout_secs: 60,
            max_concurrent_conversations: None,
            log_level: "info".to_string(),
            log_format: LogFormat::Text,
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
        assert_eq!(config.sync_interval_secs, 30);
        assert_eq!(config.drain_timeout_secs, 60);
        assert!(config.max_concurrent_conversations.is_none());
        assert_eq!(config.log_level, "info");
        assert_eq!(config.log_format, LogFormat::Text);
    }
}
