use serde::{Deserialize, Serialize};

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
