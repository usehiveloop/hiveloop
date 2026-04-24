use super::handoff::build_chain_history;
use super::*;

#[test]
fn test_find_carry_forward_empty() {
    let history: Vec<Message> = vec![];
    let (start, toks) = find_token_bounded_carry_forward(&history, 2, 10_000);
    assert_eq!(start, 0);
    assert_eq!(toks, 0);
}

#[test]
fn test_find_carry_forward_respects_turn_ceiling() {
    let history = vec![
        Message::user("a"),
        Message::assistant("b"),
        Message::user("c"),
        Message::assistant("d"),
        Message::user("e"),
        Message::assistant("f"),
    ];
    // 1 user turn → start at "e"
    let (start, _) = find_token_bounded_carry_forward(&history, 1, 10_000);
    assert_eq!(start, 4);
    // 2 user turns → start at "c"
    let (start, _) = find_token_bounded_carry_forward(&history, 2, 10_000);
    assert_eq!(start, 2);
    // 10 requested but only 3 exist → all of it
    let (start, _) = find_token_bounded_carry_forward(&history, 10, 10_000);
    assert_eq!(start, 0);
}

#[test]
fn test_find_carry_forward_respects_token_cap() {
    // Each message is roughly "x" (tiny). 2 turns would be ~5 messages.
    let history = vec![
        Message::user("a"),
        Message::assistant("b"),
        Message::user("c"),
        Message::assistant("d"),
        Message::user("e"),
        Message::assistant("f"),
    ];
    // Cap far below any turn's tokens → should still take at least 1 turn.
    let (start, _) = find_token_bounded_carry_forward(&history, 5, 1);
    assert!(start >= 4, "token cap must not evict the last user turn");
}

#[test]
fn test_format_journal_entries() {
    let entries = vec![JournalEntry {
        id: "1".to_string(),
        chain_index: 0,
        entry_type: "agent_note".to_string(),
        content: "decision".to_string(),
        category: Some("decision".to_string()),
        timestamp: chrono::Utc::now(),
    }];
    let formatted = format_journal(&entries);
    assert!(formatted.contains("[decision] [chain 0]"));
}

#[test]
fn test_build_chain_history_with_journal_and_checkpoint() {
    let entries = vec![JournalEntry {
        id: "1".to_string(),
        chain_index: 0,
        entry_type: "agent_note".to_string(),
        content: "Important decision".to_string(),
        category: Some("decision".to_string()),
        timestamp: chrono::Utc::now(),
    }];

    let carry_forward = vec![
        Message::user("Continue working on X"),
        Message::assistant("Sure, I'll continue."),
    ];

    let history = build_chain_history(&entries, None, "Checkpoint text here", 0, &carry_forward);

    // journal_user + journal_ack + checkpoint_user + checkpoint_ack + 2 carry-forward
    assert_eq!(history.len(), 6);
}

#[test]
fn test_build_chain_history_no_journal() {
    let entries: Vec<JournalEntry> = vec![];
    let carry_forward = vec![Message::user("Continue"), Message::assistant("OK")];

    let history = build_chain_history(&entries, None, "Checkpoint text", 0, &carry_forward);
    assert_eq!(history.len(), 4);
}

#[test]
fn test_build_chain_history_with_todos_snapshot() {
    let entries: Vec<JournalEntry> = vec![];
    let carry_forward = vec![Message::user("Continue"), Message::assistant("OK")];

    let todos = "1. [in_progress] (high) Implement controller\n2. [pending] (high) Write tests\n";
    let history = build_chain_history(&entries, Some(todos), "Checkpoint text", 0, &carry_forward);

    // No journal block. Todos block (user + assistant). Checkpoint block
    // (user + assistant). 2 carry-forward. Total = 6.
    assert_eq!(history.len(), 6);
    // Second message is the assistant ack for the todos block.
    let first_user = &history[0];
    if let Message::User { content } = first_user {
        let text = match content.first() {
            rig::message::UserContent::Text(t) => t.text.clone(),
            _ => String::new(),
        };
        assert!(text.contains("Todo List Snapshot"), "got: {text}");
        assert!(text.contains("Implement controller"), "got: {text}");
    } else {
        panic!("expected first message to be a user todos snapshot");
    }
}
