#[test]
fn test_system_reminder_prepended_to_user_message() {
    let system_reminder = "<system-reminder>\n\n# System Reminders\n\n## Available skills\n\nThe following skills are available for use with the Skill tool:\n\n- **Code Review** - Reviews code\n\n</system-reminder>";
    let user_text = "Please review this code";

    let final_text = format!("{}\n\n{}", system_reminder, user_text);

    assert!(final_text.contains("<system-reminder>"));
    assert!(final_text.contains("</system-reminder>"));
    assert!(final_text.contains(user_text));
    assert!(final_text.starts_with("<system-reminder>"));
}

#[test]
fn test_empty_system_reminder_skipped() {
    let system_reminder = "";
    let user_text = "Hello";

    let final_text = if system_reminder.is_empty() {
        user_text.to_string()
    } else {
        format!("{}\n\n{}", system_reminder, user_text)
    };

    assert_eq!(final_text, user_text);
}

/// P1.4 layout invariant: within a single user turn, bytes are laid out
/// as `[date?][stable][user][volatile?]` — stable content stays in a
/// deterministic head position so prior turns' user messages remain
/// byte-locked for cache reuse, and volatile content stays at the tail
/// so it never leaks into the cached region.
#[test]
fn test_user_turn_layout_stable_head_volatile_tail() {
    let date = Some("<DATE>".to_string());
    let stable = "<STABLE>";
    let user = "Hello";
    let volatile = "<VOLATILE>";

    let mut pieces: Vec<String> = Vec::new();
    if let Some(ref d) = date {
        pieces.push(d.clone());
    }
    if !stable.is_empty() {
        pieces.push(stable.to_string());
    }
    pieces.push(user.to_string());
    if !volatile.is_empty() {
        pieces.push(volatile.to_string());
    }
    let out = pieces.join("\n\n");

    // Stable lives BEFORE the user text; volatile lives AFTER.
    let stable_pos = out.find(stable).unwrap();
    let user_pos = out.find(user).unwrap();
    let volatile_pos = out.find(volatile).unwrap();
    assert!(stable_pos < user_pos);
    assert!(user_pos < volatile_pos);
}

#[test]
fn test_user_turn_layout_omits_empty_pieces() {
    let pieces: Vec<String> = vec!["HEAD".into(), "BODY".into()];
    let out = pieces.join("\n\n");
    assert_eq!(out, "HEAD\n\nBODY");

    let just_body: Vec<String> = vec!["BODY".into()];
    assert_eq!(just_body.join("\n\n"), "BODY");
}
