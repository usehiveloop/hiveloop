use serde::{Deserialize, Serialize};

mod lsp;
mod webhook;

pub use lsp::{LspConfig, LspServerConfig};
pub use webhook::WebhookConfig;

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

    /// Enable auto-discovery of skills from .claude/skills/, .cursor/rules/,
    /// .github/copilot-instructions.md, .windsurf/rules/, and .agent/skills/.
    /// Configured via `BRIDGE_SKILL_DISCOVERY_ENABLED` env var.
    #[serde(default)]
    pub skill_discovery_enabled: bool,

    /// Working directory for skill discovery. Defaults to current working directory.
    /// Configured via `BRIDGE_SKILL_DISCOVERY_DIR` env var.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skill_discovery_dir: Option<String>,

    /// Allow API clients to attach `stdio` MCP servers per conversation.
    /// Off by default — stdio transport runs an arbitrary subprocess, which is
    /// a foot-gun in anything but a trusted/sandboxed deployment. When false,
    /// only `streamable_http` is accepted in `CreateConversationRequest::mcp_servers`.
    /// Configured via `BRIDGE_ALLOW_STDIO_MCP_FROM_API` env var.
    #[serde(default)]
    pub allow_stdio_mcp_from_api: bool,

    /// When true, Bridge injects an environment system reminder into every
    /// conversation that describes the sandbox runtime: pre-installed tools,
    /// resource limits (CPU, memory, disk from cgroups), and OS version.
    /// Intended for standalone agents running in a Daytona dev-box sandbox.
    /// Configured via `BRIDGE_STANDALONE_AGENT` env var.
    #[serde(default)]
    pub standalone_agent: bool,

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

fn default_otel_service_name() -> String {
    "bridge".to_string()
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
            skill_discovery_enabled: false,
            skill_discovery_dir: None,
            allow_stdio_mcp_from_api: false,
            standalone_agent: false,
            otel_endpoint: None,
            otel_service_name: default_otel_service_name(),
        }
    }
}

#[cfg(test)]
mod tests;
