use domain::{InboundEvent, MessageHandle, SessionId};

fn make_event(session_id: &str, user: &str) -> InboundEvent {
    InboundEvent {
        envelope_id: "test-1".into(),
        session_id: SessionId::from(session_id),
        user: user.into(),
        user_display_name: None,
        text: "test".into(),
        attachments: vec![],
        raw: serde_json::json!({}),
        inbound_handle: MessageHandle {
            channel: "C123".into(),
            ts: "".into(),
        },
        is_direct_message: false,
        is_directly_addressed: true,
        link_previews: vec![],
    }
}

#[test]
fn normal_user_message_in_slack_thread_is_not_cron() {
    let event = make_event("C123-1778247607.836569", "U08P1G9EDNG");
    let is_cron = event.user == "cron";
    assert!(!is_cron, "normal user message must not be treated as cron");
}

#[test]
fn cron_worker_job_message_is_identified_as_cron() {
    let event = make_event("C123-cron-cron-1778211804202", "cron");
    let is_cron = event.user == "cron";
    assert!(is_cron, "cron worker messages must be identified");
    assert!(
        event.session_id.as_str().contains("-cron-"),
        "worker cron has -cron- in session"
    );
}

#[test]
fn wake_cron_uses_original_session_id() {
    let sid = "C123-1778247607.836569";
    let event = make_event(sid, "cron");
    let is_wake = event.user == "cron" && !sid.contains("-cron-") && !sid.contains("-delegate-");
    assert!(is_wake, "wake cron must be identified by clean session ID");
}

#[test]
fn delegate_background_task_uses_delegate_session_pattern() {
    let sid = "C123-delegate-delegate-1";
    let event = make_event(sid, "cron");
    let is_delegate = sid.contains("-delegate-");
    let is_wake = event.user == "cron" && !sid.contains("-cron-") && !sid.contains("-delegate-");
    assert!(is_delegate, "delegate has -delegate- in session");
    assert!(!is_wake, "delegate is not a wake cron");
}

#[test]
fn channel_is_derived_from_session_id() {
    let sid = SessionId::from("C0AS791RGLW-1778247607.836569");
    let channel = sid.as_str().split_once('-').map(|(c, _)| c).unwrap();
    assert_eq!(channel, "C0AS791RGLW");
}

#[test]
fn channel_extraction_handles_worker_cron() {
    let sid = SessionId::from("C0AS791RGLW-cron-cron-1778211804202");
    let channel = sid.as_str().split_once('-').map(|(c, _)| c).unwrap();
    assert_eq!(
        channel, "C0AS791RGLW",
        "channel extraction must work for worker cron IDs"
    );
}

#[test]
fn channel_extraction_handles_delegate_session() {
    let sid = SessionId::from("C0AS791RGLW-delegate-delegate-1");
    let channel = sid.as_str().split_once('-').map(|(c, _)| c).unwrap();
    assert_eq!(
        channel, "C0AS791RGLW",
        "channel extraction must work for delegate IDs"
    );
}

#[test]
fn routing_decision_matrix() {
    let cases = vec![
        ("C123-1778247607.836569", "U08P1G9EDNG", "reply"), // normal user → thread
        ("C123-cron-cron-1", "cron", "post_to_channel"),    // worker cron → channel
        ("C123-1778247607.836569", "cron", "reply"),        // wake cron → thread
        ("C123-delegate-1", "cron", "post_to_channel"),     // delegate cron → channel
    ];

    for (sid, user, expected) in &cases {
        let event = make_event(sid, user);
        let is_cron = event.user == "cron";
        let is_wake = is_cron && !sid.contains("-cron-") && !sid.contains("-delegate-");
        let route = if is_cron && !is_wake {
            "post_to_channel"
        } else {
            "reply"
        };
        assert_eq!(route, *expected, "session={}, user={}", sid, user);
    }
}
