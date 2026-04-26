use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::artifacts::ArtifactsConfig;
use crate::integration::IntegrationDefinition;
use crate::mcp::McpServerDefinition;
use crate::permission::ToolPermission;
use crate::provider::ProviderConfig;
use crate::skill::SkillDefinition;
use crate::tool::ToolDefinition;

mod requirements;
mod runtime;

pub use requirements::{
    RequirementCadence, RequirementEnforcement, RequirementPosition, ToolRequirement,
};
pub use runtime::{HistoryStripConfig, ImmortalConfig};

/// Type alias for agent identifiers.
pub type AgentId = String;

/// Complete definition of an AI agent fetched from the control plane.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[cfg_attr(feature = "openapi", schema(no_recursion))]
pub struct AgentDefinition {
    /// Unique agent identifier
    pub id: AgentId,
    /// Human-readable agent name
    pub name: String,
    /// Human-readable description of the agent's purpose and capabilities.
    /// Used in tool documentation when this agent is a subagent.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    /// System prompt for the agent
    pub system_prompt: String,
    /// LLM provider configuration
    pub provider: ProviderConfig,
    /// Agent-defined tools
    #[serde(default)]
    pub tools: Vec<ToolDefinition>,
    /// MCP server connections
    #[serde(default)]
    pub mcp_servers: Vec<McpServerDefinition>,
    /// Available skills
    #[serde(default)]
    pub skills: Vec<SkillDefinition>,
    /// External integrations (e.g., GitHub, Slack, Mailchimp).
    /// Each integration's actions become individual tools for the agent.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub integrations: Vec<IntegrationDefinition>,
    /// Workspace artifact upload configuration. When present, bridge
    /// auto-registers an `upload_to_workspace` tool that streams files
    /// from the agent's sandbox to the control plane with resume support.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub artifacts: Option<ArtifactsConfig>,
    /// Agent configuration options
    #[serde(default)]
    pub config: AgentConfig,
    /// Nested subagent definitions
    #[serde(default)]
    pub subagents: Vec<AgentDefinition>,
    /// Per-tool permission overrides. Key = tool name, Value = permission level.
    /// Tools not listed default to `Allow`.
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub permissions: HashMap<String, ToolPermission>,
    /// Webhook URL for event delivery
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub webhook_url: Option<String>,
    /// Webhook secret for HMAC signing
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub webhook_secret: Option<String>,
    /// Version field for change detection during sync
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
    /// Last updated timestamp for change detection
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<String>,
}

impl AgentDefinition {
    /// Static semantic validation of an agent definition — intended to run
    /// at push time before the agent is loaded into the supervisor. Returns
    /// the first problem it finds.
    ///
    /// Callers should translate the returned message into an
    /// `InvalidRequest` error (400).
    pub fn validate(&self) -> Result<(), String> {
        // `tool_requirements.tool` cannot overlap `disabled_tools` — the
        // agent would never be able to call a tool it's configured to
        // require. Reject explicitly rather than silently deadlock.
        for req in &self.config.tool_requirements {
            if self.config.disabled_tools.iter().any(|d| d == &req.tool) {
                return Err(format!(
                    "tool_requirements[{}] conflicts with disabled_tools: \
                     tool '{}' is both required per turn and disabled",
                    self.config
                        .tool_requirements
                        .iter()
                        .position(|r| r.tool == req.tool)
                        .unwrap_or(0),
                    req.tool
                ));
            }
        }

        if let Some(artifacts) = &self.artifacts {
            artifacts.validate()?;
        }

        Ok(())
    }
}

/// Configuration options for an agent.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentConfig {
    /// Maximum tokens for LLM response
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,
    /// Maximum conversation turns
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_turns: Option<u32>,
    /// Temperature for LLM sampling
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,
    /// JSON schema for structured output
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub json_schema: Option<serde_json::Value>,
    /// Rate limit in requests per minute
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub rate_limit_rpm: Option<u32>,

    /// Maximum total subagent tasks per conversation. Limits resource consumption
    /// from recursive/parallel subagent spawning. Default: 50.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_tasks_per_conversation: Option<u32>,

    /// Maximum concurrent conversations for this specific agent.
    /// Takes precedence over the global max_concurrent_conversations.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_concurrent_conversations: Option<u32>,

    /// When true, the agent can complete a turn with only tool calls and no text.
    /// Empty text responses are treated as success if tool calls were executed.
    /// Default: false.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub tool_calls_only: Option<bool>,

    /// Immortal conversation configuration (chain-based context management).
    /// When set, the conversation chains into fresh context
    /// windows transparently while maintaining a living journal.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub immortal: Option<ImmortalConfig>,

    /// Strip tool-result bodies from old messages before sending history to the
    /// LLM. Full output already lives on disk via the spill pipeline; the
    /// stripped message keeps a pointer so the agent can read it via RipGrep
    /// if needed. Omit to use defaults (stripping enabled). Set `enabled:
    /// false` to turn off entirely.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub history_strip: Option<HistoryStripConfig>,

    /// How often (in turns) to re-emit the stable system reminder (skills,
    /// subagents, todos) at the head of the user message. Always emitted on
    /// turn 0; thereafter on turns where `turn_count % N == 0`. Lower values
    /// reinforce the reminders more often at the cost of bigger uncached
    /// tails on those turns; higher values let the reminder go stale but
    /// keep prompt sizes lean.
    ///
    /// Default: 10. Set to 1 for every-turn refresh; values <1 are clamped
    /// to 1.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub system_reminder_refresh_turns: Option<u32>,

    /// Tools to disable for this agent. Takes priority over everything else —
    /// disabled tools are removed from the registry before the agent is built,
    /// so the LLM never sees them. Works for built-in tools, MCP tools,
    /// integration tools, and spider tools.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub disabled_tools: Vec<String>,

    /// Declarative tool-call requirements evaluated at the end of every agent
    /// turn. Each entry describes a tool that must be called (with optional
    /// cadence, position, and min-call constraints) and what bridge should do
    /// if the requirement is violated. See [`ToolRequirement`] for the shape
    /// and [`RequirementEnforcement`] for the dispatch options.
    ///
    /// Default: empty (no enforcement).
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tool_requirements: Vec<ToolRequirement>,

    /// Wall-clock timeout (seconds) applied when this agent is invoked as a
    /// foreground subagent. Default: 300 (5 minutes).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub subagent_timeout_foreground_secs: Option<u64>,

    /// Wall-clock timeout (seconds) applied when this agent is invoked as a
    /// background subagent. Default: 300 (5 minutes).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub subagent_timeout_background_secs: Option<u64>,
}

/// Default subagent execution timeout (5 minutes) used when an agent config
/// does not specify its own `subagent_timeout_*_secs`.
pub const DEFAULT_SUBAGENT_TIMEOUT_SECS: u64 = 300;

/// Lightweight agent summary for listing endpoints.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentSummary {
    /// Agent identifier
    pub id: AgentId,
    /// Agent name
    pub name: String,
    /// Agent version
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
}

#[cfg(test)]
mod tests;
