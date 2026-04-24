use serde::{Deserialize, Serialize};

use crate::provider::ProviderConfig;

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

    /// When true (default), bridge registers `journal_read` and
    /// `journal_write` tools with the agent so the model can record durable
    /// notes that survive chain rotations. When false, journal tools are
    /// NOT exposed to the model — the immortal engine falls back to using
    /// the current `todowrite` list as the cross-chain carry-forward state.
    /// Useful when the agent's task doesn't benefit from free-form notes
    /// and the todo list is sufficient as persistent context.
    #[serde(default = "default_expose_journal_tools")]
    pub expose_journal_tools: bool,
}

fn default_expose_journal_tools() -> bool {
    true
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
