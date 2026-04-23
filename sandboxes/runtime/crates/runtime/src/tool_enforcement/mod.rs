//! Tool-call requirement enforcement.
//!
//! Agents can declare `tool_requirements` in their config — bridge evaluates
//! them at the end of each turn and, on violation, either emits a signal
//! event, attaches a system reminder to the next user message, or re-prompts
//! the agent this turn.
//!
//! This module is PURE logic. The conversation loop wires it in by:
//!   1. Constructing a [`ToolEnforcementState`] per conversation.
//!   2. Collecting the ordered list of tool names called in each turn.
//!   3. Calling [`evaluate_requirements`] to get any violations.
//!   4. Dispatching each violation by its [`RequirementEnforcement`] variant.

use std::collections::{HashMap, HashSet};

use bridge_core::agent::{RequirementEnforcement, RequirementPosition, ToolRequirement};

mod matching;
#[cfg(test)]
mod tests;

pub use matching::tool_name_matches;

use matching::{
    any_substantive_after, any_substantive_before, cadence_applies, count_matches,
    first_match_index, last_match_index,
};

/// Running state kept across turns for a conversation's requirement checks.
#[derive(Debug, Clone, Default)]
pub struct ToolEnforcementState {
    /// 1-based turn counter incremented on each successful agent turn.
    pub turn_count: u32,
    /// Turn at which each requirement pattern was most recently satisfied.
    /// Keyed by `ToolRequirement.tool` (the pattern, not the resolved name)
    /// so cadence bookkeeping survives model-driven tool-name variation.
    pub last_satisfied_turn: HashMap<String, u32>,
}

impl ToolEnforcementState {
    pub fn new() -> Self {
        Self::default()
    }

    /// Bump the turn counter. Call at the start of evaluation, once per turn.
    pub fn advance_turn(&mut self) {
        self.turn_count = self.turn_count.saturating_add(1);
    }
}

/// Why a particular requirement was not satisfied this turn.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ViolationReason {
    /// The tool wasn't called enough times this turn.
    /// `observed < required`.
    InsufficientCalls { observed: u32, required: u32 },
    /// Another non-exempt tool was called before the required tool,
    /// violating a `TurnStart` position constraint.
    NotAtTurnStart,
    /// Another non-exempt tool was called after the required tool,
    /// violating a `TurnEnd` position constraint.
    NotAtTurnEnd,
}

/// A materialized violation — the requirement, the reason, and the default
/// reminder text. The enforcement variant is copied from the requirement.
#[derive(Debug, Clone)]
pub struct Violation {
    pub requirement: ToolRequirement,
    pub reason: ViolationReason,
    /// The reminder text to inject / attach. Uses the requirement's
    /// `reminder_message` if set, otherwise a generated default.
    pub reminder_text: String,
}

impl Violation {
    /// Convenience accessor for the enforcement dispatch variant.
    pub fn enforcement(&self) -> RequirementEnforcement {
        self.requirement.enforcement
    }
}

/// Evaluate all requirements against this turn's tool calls.
///
/// Call [`ToolEnforcementState::advance_turn`] BEFORE this function so the
/// state reflects the current turn number.
///
/// Updates `state.last_satisfied_turn` for every requirement whose tool was
/// actually called this turn (regardless of whether other constraints were
/// violated — a call still "resets" the cadence counter).
pub fn evaluate_requirements(
    state: &mut ToolEnforcementState,
    requirements: &[ToolRequirement],
    turn_calls: &[String],
) -> Vec<Violation> {
    let mut violations = Vec::new();

    for req in requirements {
        // Update cadence bookkeeping first — if the tool was called this turn,
        // the cadence counter resets, regardless of position/min_calls state.
        let observed = count_matches(&req.tool, turn_calls);
        if observed > 0 {
            state
                .last_satisfied_turn
                .insert(req.tool.clone(), state.turn_count);
        }

        // Does the cadence qualify this turn?
        if !cadence_applies(req, state) {
            continue;
        }

        // min_calls check.
        if observed < req.min_calls {
            let reminder_text = build_reminder_text(
                req,
                &ViolationReason::InsufficientCalls {
                    observed,
                    required: req.min_calls,
                },
            );
            violations.push(Violation {
                requirement: req.clone(),
                reason: ViolationReason::InsufficientCalls {
                    observed,
                    required: req.min_calls,
                },
                reminder_text,
            });
            continue;
        }

        // position check (lenient — EXEMPT_FROM_POSITION tools don't count).
        match req.position {
            RequirementPosition::Anywhere => { /* already satisfied */ }
            RequirementPosition::TurnStart => {
                if let Some(idx) = first_match_index(&req.tool, turn_calls) {
                    if any_substantive_before(idx, turn_calls) {
                        let reason = ViolationReason::NotAtTurnStart;
                        let reminder_text = build_reminder_text(req, &reason);
                        violations.push(Violation {
                            requirement: req.clone(),
                            reason,
                            reminder_text,
                        });
                    }
                }
            }
            RequirementPosition::TurnEnd => {
                if let Some(idx) = last_match_index(&req.tool, turn_calls) {
                    if any_substantive_after(idx, turn_calls) {
                        let reason = ViolationReason::NotAtTurnEnd;
                        let reminder_text = build_reminder_text(req, &reason);
                        violations.push(Violation {
                            requirement: req.clone(),
                            reason,
                            reminder_text,
                        });
                    }
                }
            }
        }
    }

    violations
}

/// Default reminder text when `ToolRequirement.reminder_message` is unset.
fn build_reminder_text(req: &ToolRequirement, reason: &ViolationReason) -> String {
    if let Some(custom) = &req.reminder_message {
        return custom.clone();
    }
    match reason {
        ViolationReason::InsufficientCalls { observed, required } => {
            if *observed == 0 {
                format!(
                    "You must call the `{}` tool this turn. It is required by the conversation's enforcement policy — please call it now before finishing your response.",
                    req.tool
                )
            } else {
                format!(
                    "You called `{}` {} time(s) this turn but {} call(s) are required. Please call it again before finishing your response.",
                    req.tool, observed, required
                )
            }
        }
        ViolationReason::NotAtTurnStart => format!(
            "The `{}` tool must be the FIRST substantive action of each qualifying turn — please call it before any other work in the next turn.",
            req.tool
        ),
        ViolationReason::NotAtTurnEnd => format!(
            "The `{}` tool must be the LAST substantive action of each qualifying turn — please make sure it is the final tool call in the next turn.",
            req.tool
        ),
    }
}

/// Convert a list of violations into a single user-facing reminder block
/// suitable for the `<system-reminder>` wrapping used by the
/// `NextTurnReminder` enforcement path. De-duplicates on tool name in case
/// multiple reasons fired for the same tool.
pub fn render_reminder_block(violations: &[Violation]) -> String {
    let mut seen = HashSet::new();
    let mut lines = Vec::new();
    for v in violations {
        if seen.insert(v.requirement.tool.clone()) {
            lines.push(format!("- {}", v.reminder_text));
        }
    }
    if lines.is_empty() {
        String::new()
    } else {
        format!(
            "Tool-call requirement(s) were missed last turn:\n{}",
            lines.join("\n")
        )
    }
}
