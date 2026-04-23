//! Tool-name pattern matching and position helpers.

use bridge_core::agent::{RequirementCadence, ToolRequirement};

use super::ToolEnforcementState;

/// Tools considered read-only / metadata — they do not disqualify a
/// `TurnStart` position requirement (lenient matching).
const EXEMPT_FROM_POSITION: &[&str] = &["todoread", "todo_read", "journal_read", "journalread"];

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
pub(super) fn count_matches(pattern: &str, turn_calls: &[String]) -> u32 {
    turn_calls
        .iter()
        .filter(|name| tool_name_matches(pattern, name))
        .count() as u32
}

/// Is this tool considered "substantive" for position checks?
/// Exempt tools (read-only/metadata) don't disqualify TurnStart.
pub(super) fn is_substantive(name: &str) -> bool {
    !EXEMPT_FROM_POSITION.contains(&name)
}

/// Does the cadence qualify this turn given state + requirement?
pub(super) fn cadence_applies(req: &ToolRequirement, state: &ToolEnforcementState) -> bool {
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
pub(super) fn first_match_index(pattern: &str, turn_calls: &[String]) -> Option<usize> {
    turn_calls
        .iter()
        .position(|name| tool_name_matches(pattern, name))
}

/// Find the index (0-based) of the last matching call; or None.
pub(super) fn last_match_index(pattern: &str, turn_calls: &[String]) -> Option<usize> {
    turn_calls
        .iter()
        .rposition(|name| tool_name_matches(pattern, name))
}

/// Is there a substantive (non-exempt) tool call at any index before `idx`?
pub(super) fn any_substantive_before(idx: usize, turn_calls: &[String]) -> bool {
    turn_calls[..idx].iter().any(|name| is_substantive(name))
}

/// Is there a substantive (non-exempt) tool call at any index after `idx`?
pub(super) fn any_substantive_after(idx: usize, turn_calls: &[String]) -> bool {
    turn_calls[idx + 1..]
        .iter()
        .any(|name| is_substantive(name))
}
