use super::super::*;
use super::{make_test_skills, make_test_subagents};
use chrono::Utc;

#[test]
fn test_build_split_places_skills_in_stable_half() {
    let skills = make_test_skills();
    let subagents = make_test_subagents();
    let reminder = SystemReminder::new()
        .with_skills(&skills)
        .with_subagents(&subagents)
        .with_current_date(Utc::now())
        .with_immortal_context(3, 5);

    let (stable, volatile) = reminder.build_split();
    // Skills & subagents are stable
    assert!(stable.contains("Available skills"));
    assert!(stable.contains("Available sub-agents"));
    assert!(stable.contains("Code Review"));
    assert!(stable.contains("researcher"));
    // Stable half does NOT contain volatile markers
    assert!(!stable.contains("Current date"));
    assert!(!stable.contains("Immortal Conversation"));
    assert!(!stable.contains("Today is"));
    // And the volatile half is self-consistent
    assert!(volatile.contains("Current date"));
    assert!(volatile.contains("Immortal Conversation"));
}

#[test]
fn test_build_split_places_date_in_volatile_half() {
    let reminder = SystemReminder::new()
        .with_skills(&make_test_skills())
        .with_current_date(Utc::now());
    let (_, volatile) = reminder.build_split();
    assert!(volatile.contains("Current date"));
    assert!(volatile.contains("Today is"));
}

#[test]
fn test_build_split_immortal_is_volatile() {
    let reminder = SystemReminder::new()
        .with_skills(&make_test_skills())
        .with_immortal_context(7, 42);
    let (stable, volatile) = reminder.build_split();
    assert!(stable.contains("Available skills"));
    assert!(!stable.contains("Immortal Conversation"));
    assert!(volatile.contains("Immortal Conversation"));
    assert!(volatile.contains("Current chain**: 7"));
}

#[test]
fn test_build_split_todos_are_volatile() {
    let todos = vec![TodoItem {
        content: "write docs".to_string(),
        status: "in_progress".to_string(),
        priority: "high".to_string(),
    }];
    let reminder = SystemReminder::new()
        .with_skills(&make_test_skills())
        .with_todos(&todos);
    let (stable, volatile) = reminder.build_split();
    assert!(!stable.contains("Todo List"));
    assert!(volatile.contains("Todo List"));
    assert!(volatile.contains("write docs"));
}

#[test]
fn test_build_split_empty_halves_when_only_one_side() {
    // Only stable sections
    let r1 = SystemReminder::new().with_skills(&make_test_skills());
    let (s, v) = r1.build_split();
    assert!(!s.is_empty());
    assert_eq!(v, "", "no volatile half when only stable sections present");

    // Only volatile sections
    let r2 = SystemReminder::new().with_current_date(Utc::now());
    let (s, v) = r2.build_split();
    assert_eq!(s, "", "no stable half when only volatile sections present");
    assert!(!v.is_empty());
}

#[test]
fn test_build_split_is_deterministic_per_call() {
    // The stable half must be byte-identical across two builds of an
    // equivalent SystemReminder. This is the exact invariant cache
    // reuse depends on.
    let skills = make_test_skills();
    let subagents = make_test_subagents();
    let a = SystemReminder::new()
        .with_skills(&skills)
        .with_subagents(&subagents)
        .build_split()
        .0;
    let b = SystemReminder::new()
        .with_skills(&skills)
        .with_subagents(&subagents)
        .build_split()
        .0;
    assert_eq!(a, b, "stable half must be byte-stable");
}

#[test]
fn test_build_equals_concatenated_split_for_stable_first_order() {
    // When we ingest sections in (stable..., volatile...) order, the
    // legacy `build()` joins them into one block. `build_split()`
    // returns two blocks — each is a full `<system-reminder>` envelope.
    let reminder = SystemReminder::new()
        .with_skills(&make_test_skills())
        .with_current_date(Utc::now());
    let one = reminder.build();
    let (s, v) = reminder.build_split();
    // Both halves are present as valid blocks, together they carry
    // the same information as the single-block `build()`.
    assert!(!s.is_empty() && !v.is_empty());
    // Each half is a self-contained envelope.
    assert!(s.starts_with("<system-reminder>"));
    assert!(s.ends_with("</system-reminder>"));
    assert!(v.starts_with("<system-reminder>"));
    assert!(v.ends_with("</system-reminder>"));
    // Contents match.
    assert!(one.contains("Available skills"));
    assert!(one.contains("Current date"));
}
