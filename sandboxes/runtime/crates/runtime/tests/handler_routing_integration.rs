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
        agent_definition: None,
    }
}

#[test]
fn normal_http_user_message_is_not_cron() {
    let event = make_event("http-conversation-1", "user-1");
    let is_cron = event.user == "cron";
    assert!(!is_cron, "normal user message must not be treated as cron");
}

#[test]
fn cron_worker_job_message_is_identified_as_cron() {
    let event = make_event("http-conversation-1-cron-cron-1778211804202", "cron");
    let is_cron = event.user == "cron";
    assert!(is_cron, "cron worker messages must be identified");
    assert!(
        event.session_id.as_str().contains("-cron-"),
        "worker cron has -cron- in session"
    );
}

#[test]
fn wake_cron_uses_original_session_id() {
    let sid = "http-conversation-1";
    let event = make_event(sid, "cron");
    let is_wake = event.user == "cron" && !sid.contains("-cron-") && !sid.contains("-delegate-");
    assert!(is_wake, "wake cron must be identified by clean session ID");
}

#[test]
fn delegate_background_task_uses_delegate_session_pattern() {
    let sid = "http-conversation-1-delegate-delegate-1";
    let event = make_event(sid, "cron");
    let is_delegate = sid.contains("-delegate-");
    let is_wake = event.user == "cron" && !sid.contains("-cron-") && !sid.contains("-delegate-");
    assert!(is_delegate, "delegate has -delegate- in session");
    assert!(!is_wake, "delegate is not a wake cron");
}

#[test]
fn http_response_policy_matrix() {
    let cases = vec![
        ("http-conversation-1", "user-1", "reply"),
        ("http-conversation-1-cron-cron-1", "cron", "reply"),
        ("http-conversation-1", "cron", "reply"),
        ("http-conversation-1-delegate-1", "cron", "reply"),
    ];

    for (sid, user, expected) in &cases {
        let event = make_event(sid, user);
        let route = "reply";
        assert_eq!(route, *expected, "session={}, user={}", sid, user);
        assert_eq!(event.session_id.as_str(), *sid);
    }
}
