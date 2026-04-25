use serde::{Deserialize, Serialize};

/// Configuration for immortal conversations — forgecode-style in-place
/// compaction.
///
/// When the conversation grows past `token_budget`, the eligible head of
/// the history is replaced in place with a single user message containing
/// a structured summary. Pure code — no LLM call, no scaffolding.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ImmortalConfig {
    /// Token budget. Compaction triggers when estimated history tokens
    /// exceed this. Used by both the top-of-turn check and the
    /// mid-rig-loop hook.
    #[serde(default = "default_immortal_token_budget")]
    pub token_budget: u32,

    /// Number of most-recent messages preserved verbatim — never
    /// compacted. Mirrors forgecode's `retention_window`. Default 0
    /// (compaction may take everything except the system prompt and
    /// initial user message). Increase to keep more recent context
    /// pristine; the trade-off is the eligible compaction range shrinks.
    #[serde(default = "default_retention_window")]
    pub retention_window: u32,

    /// Maximum fraction of total tokens eligible for compaction in any
    /// single pass (0.0–1.0). Mirrors forgecode's `eviction_window`.
    /// Default 1.0 — compaction may take everything from the first
    /// assistant message up to the retention boundary. Reduce to keep
    /// the head of the conversation more stable across compactions
    /// (each pass takes a smaller slice).
    #[serde(default = "default_eviction_window")]
    pub eviction_window: f64,

    /// When true (default), bridge registers `journal_read` /
    /// `journal_write` tools so the agent can record durable notes. The
    /// journal is no longer read or written by the compaction engine
    /// itself — it's only available as a tool for the agent's own use.
    #[serde(default = "default_expose_journal_tools")]
    pub expose_journal_tools: bool,
}

fn default_expose_journal_tools() -> bool {
    true
}

fn default_immortal_token_budget() -> u32 {
    100_000
}

fn default_retention_window() -> u32 {
    0
}

fn default_eviction_window() -> f64 {
    1.0
}

/// Configuration for stripping tool-result bodies from old messages before
/// they are sent to the LLM. Reduces input tokens while preserving the
/// ability to recover the full content via the on-disk spill file.
/// Independent of immortal mode — applied at every send.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct HistoryStripConfig {
    /// Master switch. When false, strip is a no-op.
    #[serde(default = "default_history_strip_enabled")]
    pub enabled: bool,

    /// Number of assistant messages that must follow a tool result before
    /// it becomes eligible for stripping.
    #[serde(default = "default_history_strip_age_threshold")]
    pub age_threshold: usize,

    /// Always keep the most recent N tool results regardless of age.
    #[serde(default = "default_history_strip_pin_recent")]
    pub pin_recent_count: usize,

    /// When true, tool results with `is_error: true` are never stripped.
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
