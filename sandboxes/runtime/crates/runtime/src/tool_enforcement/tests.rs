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
