use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::mcp::McpServerDefinition;
use crate::permission::ToolPermission;
use crate::provider::ProviderConfig;
use crate::skill::SkillDefinition;

/// Type alias for agent identifiers.
pub type AgentId = String;

/// Which underlying coding-agent harness this agent runs against.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum Harness {
    /// Claude Code CLI driven via the Agent Client Protocol (ACP).
    Claude,
    /// OpenCode CLI driven via the Agent Client Protocol (ACP).
    OpenCode,
}

/// Complete definition of an AI agent fetched from the control plane.
///
/// Bridge persists the definition; the harness adapter (one of the
/// [`Harness`] variants) reads it and drives the underlying CLI process.
/// MCP servers and skills are passed through to the harness — bridge does
/// not execute tools itself.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[cfg_attr(feature = "openapi", schema(no_recursion))]
pub struct AgentDefinition {
    /// Unique agent identifier
    pub id: AgentId,
    /// Human-readable agent name
    pub name: String,
    /// Human-readable description of the agent's purpose and capabilities.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    /// Which coding-agent harness to drive for this agent.
    pub harness: Harness,
    /// System prompt for the agent (mapped to the harness's system-prompt input)
    pub system_prompt: String,
    /// LLM provider configuration — credentials and model choice are
    /// materialized into the harness's expected env vars at session start.
    pub provider: ProviderConfig,
    /// MCP server connections passed through to the harness as `--mcp-config`.
    #[serde(default)]
    pub mcp_servers: Vec<McpServerDefinition>,
    /// Skills written to the harness's skill discovery directory at session start
    /// (e.g. `~/.claude/skills/<id>/SKILL.md` for Claude Code).
    #[serde(default)]
    pub skills: Vec<SkillDefinition>,
    /// Per-tool permission overrides. Drives the bridge approvals API
    /// (`/agents/{id}/conversations/{cid}/approvals`). Tools not listed
    /// default to `Allow`.
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub permissions: HashMap<String, ToolPermission>,
    /// Slim runtime configuration forwarded to the harness.
    #[serde(default)]
    pub config: AgentConfig,
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
    /// Lightweight semantic validation. Runs at push time so the caller
    /// gets a clean 400 instead of a silently-broken agent.
    pub fn validate(&self) -> Result<(), String> {
        if self.id.is_empty() {
            return Err("id must be non-empty".into());
        }
        if self.name.is_empty() {
            return Err("name must be non-empty".into());
        }
        if self.system_prompt.is_empty() {
            return Err("system_prompt must be non-empty".into());
        }
        Ok(())
    }
}

/// Per-agent configuration written into the harness's settings file at
/// session start. Every field is harness-agnostic in shape — the adapter
/// is responsible for translating it into the harness-native config
/// (`~/.claude/settings.json`, `~/.config/opencode/opencode.json`, etc.).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentConfig {
    // ── Sampling / loop ─────────────────────────────────────
    /// Maximum tokens per assistant response.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_tokens: Option<u32>,

    /// Maximum conversation turns before bridge stops the loop.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_turns: Option<u32>,

    /// Sampling temperature for the underlying model.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub temperature: Option<f64>,

    /// Reasoning effort hint. Forwarded to harnesses that support it
    /// (Claude Code thinking budget, OpenCode reasoning effort).
    /// Conventional values: `"low"`, `"medium"`, `"high"`.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reasoning_effort: Option<String>,

    // ── Model layout ────────────────────────────────────────
    /// Optional faster/cheaper model used by the harness for utility
    /// calls (summarization, title generation, etc.).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub small_fast_model: Option<String>,

    /// Optional fallback model the harness routes to when the primary
    /// model errors or is rate-limited.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub fallback_model: Option<String>,

    // ── Tool gating (written to harness config) ─────────────
    /// Tool allowlist. Empty = every tool the harness exposes is allowed.
    /// Names are harness-native (e.g. `"Read"`, `"Bash"`, `"mcp__github__*"`).
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub allowed_tools: Vec<String>,

    /// Tool denylist. Always wins over [`Self::allowed_tools`].
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub disabled_tools: Vec<String>,

    /// Permission mode written to the harness config. Harness-specific
    /// string. Claude Code: `"default"` / `"acceptEdits"` /
    /// `"bypassPermissions"` / `"plan"`. OpenCode has its own set.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub permission_mode: Option<String>,

    // ── Process env ─────────────────────────────────────────
    /// Extra environment variables merged into the harness process at
    /// session start. Useful for custom proxy endpoints, telemetry
    /// flags, or harness-specific feature flags.
    #[serde(default, skip_serializing_if = "HashMap::is_empty")]
    pub env: HashMap<String, String>,
}

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
