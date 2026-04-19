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

use bridge_core::agent::{
    RequirementCadence, RequirementEnforcement, RequirementPosition, ToolRequirement,
};

/// Tools considered read-only / metadata — they do not disqualify a
/// `TurnStart` position requirement (lenient matching).
const EXEMPT_FROM_POSITION: &[&str] = &["todoread", "todo_read", "journal_read", "journalread"];

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

/// Flexible tool-name match. Reduces MCP verbosity by allowing
/// `"post_message"` to match `"slack__post_message"` when the user didn't
/// bother to write the server prefix.
///
/// Rules:
/// - If `pattern` contains `"__"` → exact match only (user opted into the
///   full MCP name).
/// - Else → exact match OR `actual` ends with `__{pattern}`.
///
/// Matching is case-sensitive.
pub fn tool_name_matches(pattern: &str, actual: &str) -> bool {
    if pattern.contains("__") {
        pattern == actual
    } else if pattern == actual {
        true
    } else {
        let suffix = format!("__{pattern}");
        actual.ends_with(&suffix)
    }
}

/// Count how many times any call in `turn_calls` matches `pattern`.
fn count_matches(pattern: &str, turn_calls: &[String]) -> u32 {
    turn_calls
        .iter()
        .filter(|name| tool_name_matches(pattern, name))
        .count() as u32
}

/// Is this tool considered "substantive" for position checks?
/// Exempt tools (read-only/metadata) don't disqualify TurnStart.
fn is_substantive(name: &str) -> bool {
    !EXEMPT_FROM_POSITION.contains(&name)
}

/// Does the cadence qualify this turn given state + requirement?
fn cadence_applies(req: &ToolRequirement, state: &ToolEnforcementState) -> bool {
    let current_turn = state.turn_count;
    let last = state.last_satisfied_turn.get(&req.tool).copied();
    match req.cadence {
        RequirementCadence::EveryTurn => true,
        RequirementCadence::FirstTurnOnly => current_turn == 1,
        RequirementCadence::EveryNTurns { n } => {
            if n == 0 {
                return true; // treat n=0 as every turn
            }
            match last {
                None => true, // never called — require now
                Some(prev) => current_turn.saturating_sub(prev) >= n,
            }
        }
    }
}

/// Find the index (0-based) of the first matching call; or None.
fn first_match_index(pattern: &str, turn_calls: &[String]) -> Option<usize> {
    turn_calls
        .iter()
        .position(|name| tool_name_matches(pattern, name))
}

/// Find the index (0-based) of the last matching call; or None.
fn last_match_index(pattern: &str, turn_calls: &[String]) -> Option<usize> {
    turn_calls
        .iter()
        .rposition(|name| tool_name_matches(pattern, name))
}

/// Is there a substantive (non-exempt) tool call at any index before `idx`?
fn any_substantive_before(idx: usize, turn_calls: &[String]) -> bool {
    turn_calls[..idx].iter().any(|name| is_substantive(name))
}

/// Is there a substantive (non-exempt) tool call at any index after `idx`?
fn any_substantive_after(idx: usize, turn_calls: &[String]) -> bool {
    turn_calls[idx + 1..]
        .iter()
        .any(|name| is_substantive(name))
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

// ── Tests ────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::agent::{
        RequirementCadence, RequirementEnforcement, RequirementPosition, ToolRequirement,
    };

    fn req(tool: &str) -> ToolRequirement {
        ToolRequirement {
            tool: tool.to_string(),
            cadence: RequirementCadence::default(),
            position: RequirementPosition::default(),
            min_calls: 1,
            enforcement: RequirementEnforcement::default(),
            reminder_message: None,
        }
    }

    fn turn(names: &[&str]) -> Vec<String> {
        names.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn test_tool_name_matches_exact() {
        assert!(tool_name_matches("journal_write", "journal_write"));
        assert!(!tool_name_matches("journal_write", "journal_read"));
    }

    #[test]
    fn test_tool_name_matches_mcp_suffix() {
        // Pattern without "__" matches both exact and suffix.
        assert!(tool_name_matches("post_message", "post_message"));
        assert!(tool_name_matches("post_message", "slack__post_message"));
        assert!(tool_name_matches("post_message", "discord__post_message"));
        // Should NOT match random suffixes.
        assert!(!tool_name_matches("post_message", "post_messages"));
        assert!(!tool_name_matches("post_message", "slack_post_message")); // single _
    }

    #[test]
    fn test_tool_name_matches_mcp_explicit_wins() {
        // Pattern with "__" requires exact match.
        assert!(tool_name_matches(
            "slack__post_message",
            "slack__post_message"
        ));
        assert!(!tool_name_matches(
            "slack__post_message",
            "discord__post_message"
        ));
        assert!(!tool_name_matches("slack__post_message", "post_message"));
    }

    #[test]
    fn test_every_turn_requires_every_turn() {
        let mut state = ToolEnforcementState::new();
        let reqs = vec![req("journal_write")];

        // Turn 1, tool called → satisfied.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["journal_write"]));
        assert!(v.is_empty());
        assert_eq!(state.last_satisfied_turn.get("journal_write"), Some(&1));

        // Turn 2, tool NOT called → violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["read"]));
        assert_eq!(v.len(), 1);
        assert!(matches!(
            v[0].reason,
            ViolationReason::InsufficientCalls {
                observed: 0,
                required: 1
            }
        ));
    }

    #[test]
    fn test_every_n_turns_resets_on_call() {
        let mut r = req("memory_retain");
        r.cadence = RequirementCadence::EveryNTurns { n: 3 };
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        // Never called → every turn violates until first satisfaction.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert_eq!(v.len(), 1);
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert_eq!(v.len(), 1);

        // Turn 3: agent finally calls it → cadence resets, no violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["memory_retain"]));
        assert!(v.is_empty());
        assert_eq!(state.last_satisfied_turn.get("memory_retain"), Some(&3));

        // Turn 4: gap=1 → within window, no violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert!(v.is_empty());

        // Turn 5: gap=2 → within window, no violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert!(v.is_empty());

        // Turn 6: gap=3 → at threshold, violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert_eq!(v.len(), 1);

        // Turn 7: agent calls it again off-cycle → resets.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["memory_retain"]));
        assert!(v.is_empty());
        assert_eq!(state.last_satisfied_turn.get("memory_retain"), Some(&7));
    }

    #[test]
    fn test_first_turn_only() {
        let mut r = req("workspace_scan");
        r.cadence = RequirementCadence::FirstTurnOnly;
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        // Turn 1 without the tool → violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert_eq!(v.len(), 1);

        // Turn 2 without it → no violation (cadence doesn't apply).
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert!(v.is_empty());
    }

    #[test]
    fn test_position_turn_start_lenient() {
        let mut r = req("memory_recall");
        r.position = RequirementPosition::TurnStart;
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        // Exempt tools (todoread) before memory_recall are OK.
        state.advance_turn();
        let v = evaluate_requirements(
            &mut state,
            &reqs,
            &turn(&["todoread", "memory_recall", "bash"]),
        );
        assert!(v.is_empty(), "lenient: todoread before is exempt");

        // Substantive tool (bash) before memory_recall → violation.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["bash", "memory_recall"]));
        assert_eq!(v.len(), 1);
        assert!(matches!(v[0].reason, ViolationReason::NotAtTurnStart));
    }

    #[test]
    fn test_position_turn_end() {
        let mut r = req("memory_retain");
        r.position = RequirementPosition::TurnEnd;
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["bash", "memory_retain", "bash"]));
        assert_eq!(v.len(), 1);
        assert!(matches!(v[0].reason, ViolationReason::NotAtTurnEnd));

        // Journal_read after is exempt.
        state.advance_turn();
        let v = evaluate_requirements(
            &mut state,
            &reqs,
            &turn(&["bash", "memory_retain", "journal_read"]),
        );
        assert!(v.is_empty());
    }

    #[test]
    fn test_min_calls() {
        let mut r = req("slack__post_message");
        r.min_calls = 2;
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["slack__post_message"]));
        assert_eq!(v.len(), 1);
        assert!(matches!(
            v[0].reason,
            ViolationReason::InsufficientCalls {
                observed: 1,
                required: 2
            }
        ));

        state.advance_turn();
        let v = evaluate_requirements(
            &mut state,
            &reqs,
            &turn(&["slack__post_message", "slack__post_message"]),
        );
        assert!(v.is_empty());
    }

    #[test]
    fn test_mcp_suffix_matches_in_turn_calls() {
        let r = req("post_message"); // pattern without "__"
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        // Registered tool is "slack__post_message" — should match.
        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&["slack__post_message"]));
        assert!(v.is_empty());
    }

    #[test]
    fn test_render_reminder_dedupes_by_tool() {
        let reqs = [req("journal_write"), req("memory_retain")];
        let violations = vec![
            Violation {
                requirement: reqs[0].clone(),
                reason: ViolationReason::InsufficientCalls {
                    observed: 0,
                    required: 1,
                },
                reminder_text: "journal reminder a".into(),
            },
            Violation {
                requirement: reqs[0].clone(),
                reason: ViolationReason::NotAtTurnStart,
                reminder_text: "journal reminder b (duplicate tool)".into(),
            },
            Violation {
                requirement: reqs[1].clone(),
                reason: ViolationReason::InsufficientCalls {
                    observed: 0,
                    required: 1,
                },
                reminder_text: "memory reminder".into(),
            },
        ];
        let block = render_reminder_block(&violations);
        assert!(block.contains("journal reminder a"));
        assert!(!block.contains("journal reminder b"));
        assert!(block.contains("memory reminder"));
    }

    #[test]
    fn test_custom_reminder_message_wins() {
        let mut r = req("memory_recall");
        r.reminder_message = Some("Call recall first, no exceptions.".to_string());
        let reqs = vec![r];
        let mut state = ToolEnforcementState::new();

        state.advance_turn();
        let v = evaluate_requirements(&mut state, &reqs, &turn(&[]));
        assert_eq!(v.len(), 1);
        assert_eq!(
            v[0].reminder_text,
            "Call recall first, no exceptions.".to_string()
        );
    }
}
