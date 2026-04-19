use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::integration::IntegrationDefinition;
use crate::mcp::McpServerDefinition;
use crate::permission::ToolPermission;
use crate::provider::ProviderConfig;
use crate::skill::SkillDefinition;
use crate::tool::ToolDefinition;

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
    /// Conversation compaction configuration
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub compaction: Option<CompactionConfig>,

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
    /// When set, replaces compaction — the conversation chains into fresh context
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

/// A single tool-call requirement that bridge enforces at turn boundaries.
///
/// Typical configurations:
/// - `journal_write` every turn: `{ tool: "journal_write" }` (all defaults).
/// - `memory_recall` at the start of every turn:
///   `{ tool: "memory_recall", position: "turn_start" }`.
/// - `memory_retain` at most every 3 turns:
///   `{ tool: "memory_retain", cadence: { type: "every_n_turns", n: 3 }, position: "turn_end" }`.
///
/// Tool-name matching is flexible to reduce MCP verbosity: if `tool` contains
/// `__`, match it verbatim; otherwise match any registered tool whose full
/// name equals `tool` OR ends with `__<tool>`. So `"post_message"` matches
/// an MCP tool exposed as `slack__post_message` without the user having to
/// write the server prefix.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ToolRequirement {
    /// Tool name to require (built-in, MCP, integration, or custom).
    pub tool: String,

    /// When this requirement applies (which turns must satisfy it).
    /// Default: `EveryTurn`.
    #[serde(default)]
    pub cadence: RequirementCadence,

    /// Where in the turn the call must appear.
    /// Default: `Anywhere`. Evaluation is LENIENT: read-only/metadata tools
    /// (`todoread`, `journal_read`, `ls`, `read`, etc.) are exempt and do
    /// not disqualify a `TurnStart` position requirement.
    #[serde(default)]
    pub position: RequirementPosition,

    /// Minimum number of calls required in a qualifying turn. Default: 1.
    #[serde(default = "default_min_calls")]
    pub min_calls: u32,

    /// What bridge does when this requirement is violated.
    /// Default: `NextTurnReminder` — attach a system reminder to the next
    /// user message. Zero extra LLM cost this turn.
    #[serde(default)]
    pub enforcement: RequirementEnforcement,

    /// Custom reminder text injected when the requirement is violated.
    /// Falls back to a generated default when unset.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reminder_message: Option<String>,
}

fn default_min_calls() -> u32 {
    1
}

/// Describes which turns a tool requirement applies to.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum RequirementCadence {
    /// Required on every turn.
    #[default]
    EveryTurn,
    /// Required only on the very first turn of the conversation.
    FirstTurnOnly,
    /// Required whenever `n` turns have passed without the tool being called.
    /// The counter resets any time the tool is called (on- or off-cycle).
    /// So `n=3` means "never go more than 3 consecutive turns without
    /// calling this tool" — useful for periodic memory-retain / checkpoint
    /// patterns.
    EveryNTurns { n: u32 },
}

/// Where in the turn's tool-call sequence the required call must appear.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum RequirementPosition {
    /// The call may appear anywhere among the turn's tool calls.
    #[default]
    Anywhere,
    /// The call must come before any other non-exempt tool call this turn.
    /// Exempt tools (metadata/read-only) may precede it without violating.
    TurnStart,
    /// The call must come after any other non-exempt tool call this turn
    /// — i.e. be the "last" substantive action.
    TurnEnd,
}

/// How bridge reacts when a tool requirement is violated.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Default)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum RequirementEnforcement {
    /// Emit a `ToolRequirementViolated` event and attach a system reminder
    /// that will be prepended to the next user message. No extra LLM call
    /// this turn. Default — cheapest and typically sufficient.
    #[default]
    NextTurnReminder,
    /// Emit the event AND immediately re-prompt the agent with a synthetic
    /// user message naming the missing requirement. Costs one extra LLM
    /// call per violation per turn. Bounded to 1 retry per turn.
    Reprompt,
    /// Emit the event and log a warning; do not otherwise alter the turn.
    /// Useful for observability-only mode while still surfacing the signal
    /// to clients.
    Warn,
}

/// Configuration for immortal conversations (chain-based context management).
///
/// When the token budget is exceeded, instead of compacting (summarizing in-place),
/// Bridge extracts a structured checkpoint, resets the history with the checkpoint +
/// journal + recent turns, and continues seamlessly. The external conversation_id
/// never changes.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ImmortalConfig {
    /// Token budget. Chain triggers when estimated tokens exceed this.
    #[serde(default = "default_immortal_token_budget")]
    pub token_budget: u32,

    /// Upper bound on the number of recent user turns to carry forward verbatim
    /// into the new chain. The carry-forward is further capped by
    /// `carry_forward_budget_fraction` — whichever binds first wins.
    #[serde(default = "default_carry_forward_turns")]
    pub carry_forward_turns: u32,

    /// Fraction of `token_budget` allowed for the carry-forward tail.
    /// Prevents a single tool-heavy turn from stuffing the new chain's context.
    /// Default 0.3 (30%).
    #[serde(default = "default_carry_forward_budget_fraction")]
    pub carry_forward_budget_fraction: f32,

    /// Provider config for the checkpoint extraction LLM call.
    pub checkpoint_provider: ProviderConfig,

    /// Custom prompt for checkpoint extraction. Uses a built-in default if None.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub checkpoint_prompt: Option<String>,

    /// When true, run a second verification pass after the phase-1 extraction.
    /// Default false — for strong summarizer models phase 2 rarely improves output.
    #[serde(default)]
    pub verify_checkpoint: bool,

    /// Max tokens for each checkpoint LLM call's output. Default 1500.
    #[serde(default = "default_checkpoint_max_tokens")]
    pub checkpoint_max_tokens: u32,

    /// Per-call timeout for checkpoint LLM calls, in seconds. Default 45.
    #[serde(default = "default_checkpoint_timeout_secs")]
    pub checkpoint_timeout_secs: u32,

    /// Max number of prior chain checkpoints to include as context during
    /// checkpoint extraction. Older ones are considered subsumed. Default 2.
    #[serde(default = "default_max_previous_checkpoints")]
    pub max_previous_checkpoints: u32,
}

fn default_immortal_token_budget() -> u32 {
    100_000
}

fn default_carry_forward_turns() -> u32 {
    2
}

fn default_carry_forward_budget_fraction() -> f32 {
    0.3
}

fn default_checkpoint_max_tokens() -> u32 {
    1500
}

fn default_checkpoint_timeout_secs() -> u32 {
    45
}

fn default_max_previous_checkpoints() -> u32 {
    2
}

/// Configuration for stripping tool-result bodies from old messages before
/// they are sent to the LLM. Reduces input tokens while preserving the ability
/// to recover the full content via the on-disk spill file (`RipGrep` on the
/// spill path). Strip is applied at send-time only; persistence is untouched,
/// so the decision is deterministic across turns and the provider prompt
/// cache remains stable after a result is first stripped.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HistoryStripConfig {
    /// Master switch. When false, strip is a no-op.
    #[serde(default = "default_history_strip_enabled")]
    pub enabled: bool,

    /// Number of assistant messages that must follow a tool result before it
    /// becomes eligible for stripping. Mirrors the "old tool output may be
    /// cleared later" contract in Claude Code's system prompt.
    #[serde(default = "default_history_strip_age_threshold")]
    pub age_threshold: usize,

    /// Always keep the most recent N tool results regardless of age. Protects
    /// results the agent is actively reasoning over on the current turn.
    #[serde(default = "default_history_strip_pin_recent")]
    pub pin_recent_count: usize,

    /// When true, tool results with `is_error: true` are never stripped.
    /// Error context is small and high-signal for future turns.
    #[serde(default = "default_history_strip_pin_errors")]
    pub pin_errors: bool,
}

impl Default for HistoryStripConfig {
    fn default() -> Self {
        Self {
            enabled: default_history_strip_enabled(),
            age_threshold: default_history_strip_age_threshold(),
            pin_recent_count: default_history_strip_pin_recent(),
            pin_errors: default_history_strip_pin_errors(),
        }
    }
}

fn default_history_strip_enabled() -> bool {
    true
}

fn default_history_strip_age_threshold() -> usize {
    10
}

fn default_history_strip_pin_recent() -> usize {
    3
}

fn default_history_strip_pin_errors() -> bool {
    true
}

/// Configuration for conversation compaction (history summarization).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CompactionConfig {
    /// Token budget. Compaction triggers when estimated tokens exceed this.
    #[serde(default = "default_token_budget")]
    pub token_budget: u32,

    /// Number of recent messages to preserve verbatim after compaction.
    #[serde(default = "default_tail_messages")]
    pub tail_messages: u32,

    /// Custom system prompt for the summarization call. Uses a default if None.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub summary_prompt: Option<String>,

    /// Provider config for the summarization model (required — defined by control plane).
    pub summary_provider: ProviderConfig,
}

fn default_token_budget() -> u32 {
    100_000
}

fn default_tail_messages() -> u32 {
    10
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
mod tests {
    use super::*;
    use crate::mcp::McpTransport;
    use crate::provider::ProviderType;

    /// Helper to load a fixture file relative to the workspace root.
    fn load_fixture(path: &str) -> String {
        let workspace_root = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .unwrap()
            .parent()
            .unwrap();
        let full_path = workspace_root.join(path);
        std::fs::read_to_string(&full_path)
            .unwrap_or_else(|e| panic!("Failed to read fixture {}: {}", full_path.display(), e))
    }

    #[test]
    fn parse_simple_agent_fixture() {
        let json = load_fixture("fixtures/agents/simple_agent.json");
        let agent: AgentDefinition =
            serde_json::from_str(&json).expect("simple_agent.json should deserialize");

        assert_eq!(agent.id, "agent_simple");
        assert_eq!(agent.name, "Simple Agent");
        assert_eq!(agent.system_prompt, "You are a helpful assistant.");
        assert_eq!(agent.provider.provider_type, ProviderType::OpenAI);
        assert_eq!(agent.provider.model, "gpt-4o");
        assert_eq!(agent.provider.api_key, "test-key");
        assert!(agent.provider.base_url.is_some());
        assert!(agent.tools.is_empty());
        assert!(agent.mcp_servers.is_empty());
        assert!(agent.skills.is_empty());
        assert!(agent.webhook_url.is_none());
        assert!(agent.webhook_secret.is_none());
    }

    #[test]
    fn parse_full_agent_fixture() {
        let json = load_fixture("fixtures/agents/full_agent.json");
        let agent: AgentDefinition =
            serde_json::from_str(&json).expect("full_agent.json should deserialize");

        assert_eq!(agent.id, "agent_full");
        assert_eq!(agent.name, "Full Agent");
        assert_eq!(agent.system_prompt, "You are a coding assistant.");

        // Provider
        assert_eq!(agent.provider.provider_type, ProviderType::OpenAI);
        assert_eq!(agent.provider.model, "gpt-4o");
        assert_eq!(
            agent.provider.base_url.as_deref(),
            Some("https://api.openai.com/v1")
        );

        // Tools
        assert_eq!(agent.tools.len(), 1);
        assert_eq!(agent.tools[0].name, "calculator");
        assert!(agent.tools[0].parameters_schema.is_object());

        // MCP servers
        assert_eq!(agent.mcp_servers.len(), 1);
        assert_eq!(agent.mcp_servers[0].name, "filesystem");
        match &agent.mcp_servers[0].transport {
            McpTransport::Stdio { command, args, env } => {
                assert_eq!(command, "npx");
                assert_eq!(args.len(), 3);
                assert!(env.contains_key("NODE_ENV"));
            }
            other => panic!("Expected Stdio transport, got {:?}", other),
        }

        // Skills
        assert_eq!(agent.skills.len(), 2);
        assert_eq!(agent.skills[0].id, "skill_code_review");
        assert_eq!(agent.skills[1].id, "skill_deploy");
        assert!(!agent.skills[1].files.is_empty());
        assert_eq!(agent.skills[1].files.len(), 2);
        assert!(agent.skills[1].files.contains_key("runbook.md"));
        assert!(agent.skills[1].frontmatter.is_some());

        // Config
        assert_eq!(agent.config.max_tokens, Some(4096));
        assert_eq!(agent.config.max_turns, Some(10));
        assert_eq!(agent.config.temperature, Some(0.7));

        // Webhooks
        assert_eq!(
            agent.webhook_url.as_deref(),
            Some("https://example.com/webhooks/agent")
        );
        assert_eq!(
            agent.webhook_secret.as_deref(),
            Some("<webhook-secret>")
        );
    }

    #[test]
    fn parse_anthropic_agent_fixture() {
        let json = load_fixture("fixtures/agents/anthropic_agent.json");
        let agent: AgentDefinition =
            serde_json::from_str(&json).expect("anthropic_agent.json should deserialize");

        assert_eq!(agent.id, "agent_anthropic");
        assert_eq!(agent.name, "Anthropic Agent");
        assert_eq!(agent.provider.provider_type, ProviderType::Anthropic);
        assert_eq!(agent.provider.model, "claude-haiku-4-5-20251001");
        assert_eq!(agent.config.max_tokens, Some(4096));
        assert_eq!(agent.config.temperature, Some(0.7));
    }

    #[test]
    fn parse_gemini_agent_fixture() {
        let json = load_fixture("fixtures/agents/gemini_agent.json");
        let agent: AgentDefinition =
            serde_json::from_str(&json).expect("gemini_agent.json should deserialize");

        assert_eq!(agent.id, "agent_gemini");
        assert_eq!(agent.provider.provider_type, ProviderType::Google);
        assert_eq!(agent.provider.model, "gemini-2.5-flash");
    }

    #[test]
    fn parse_cohere_agent_fixture() {
        let json = load_fixture("fixtures/agents/cohere_agent.json");
        let agent: AgentDefinition =
            serde_json::from_str(&json).expect("cohere_agent.json should deserialize");

        assert_eq!(agent.id, "agent_cohere");
        assert_eq!(agent.provider.provider_type, ProviderType::Cohere);
        assert_eq!(agent.provider.model, "command-a-03-2025");
    }

    #[test]
    fn simple_agent_roundtrip() {
        let json = load_fixture("fixtures/agents/simple_agent.json");
        let agent: AgentDefinition = serde_json::from_str(&json).unwrap();
        let serialized = serde_json::to_string(&agent).unwrap();
        let roundtripped: AgentDefinition = serde_json::from_str(&serialized).unwrap();
        assert_eq!(agent, roundtripped);
    }

    #[test]
    fn full_agent_roundtrip() {
        let json = load_fixture("fixtures/agents/full_agent.json");
        let agent: AgentDefinition = serde_json::from_str(&json).unwrap();
        let serialized = serde_json::to_string(&agent).unwrap();
        let roundtripped: AgentDefinition = serde_json::from_str(&serialized).unwrap();
        assert_eq!(agent, roundtripped);
    }
}
