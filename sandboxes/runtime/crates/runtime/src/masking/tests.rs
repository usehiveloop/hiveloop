use super::helpers::{extract_spill_path, STRIP_MARKER_PREFIX};
use super::*;
use rig::message::Text;

fn spill_line(uuid: &str) -> String {
    format!(
        "Full output saved to: /tmp/bridge_tool_output/{uuid}.txt\n\
             To find specific content, call the RipGrep tool with path=\"/tmp/bridge_tool_output/{uuid}.txt\" and a regex pattern."
    )
}

fn body_with_spill(bulk_bytes: usize, uuid: &str) -> String {
    let bulk = "x".repeat(bulk_bytes);
    format!(
        "{bulk}\n\n... [50 lines, {bulk_bytes} bytes truncated. {spill}] ...",
        spill = spill_line(uuid),
    )
}

fn make_user_with_tool_result(id: &str, text_content: &str) -> Message {
    Message::User {
        content: OneOrMany::one(UserContent::ToolResult(rig::message::ToolResult {
            id: id.to_string(),
            call_id: None,
            content: OneOrMany::one(ToolResultContent::Text(Text {
                text: text_content.to_string(),
            })),
        })),
    }
}

fn get_tool_result_text(msg: &Message) -> Option<&str> {
    if let Message::User { content } = msg {
        for part in content.iter() {
            if let UserContent::ToolResult(tr) = part {
                for c in tr.content.iter() {
                    if let ToolResultContent::Text(t) = c {
                        return Some(&t.text);
                    }
                }
            }
        }
    }
    None
}

fn small_config(age_threshold: usize) -> HistoryStripConfig {
    HistoryStripConfig {
        enabled: true,
        age_threshold,
        pin_recent_count: 3,
        pin_errors: true,
    }
}

#[test]
fn test_strip_empty_history() {
    let mut history: Vec<Message> = vec![];
    strip_old_tool_outputs(&mut history, &small_config(1));
    assert!(history.is_empty());
}

#[test]
fn test_strip_preserves_recent_outputs() {
    // Three 10KB results with spill markers; age_threshold=10 → budget ≈
    // 20KB after slot math. Oldest should strip, two newest should stay.
    let mut history = vec![
        make_user_with_tool_result("call-1", &body_with_spill(10_000, "uuid-1")),
        Message::assistant("Response 1"),
        make_user_with_tool_result("call-2", &body_with_spill(10_000, "uuid-2")),
        Message::assistant("Response 2"),
        make_user_with_tool_result("call-3", &body_with_spill(10_000, "uuid-3")),
        Message::assistant("Response 3"),
    ];

    strip_old_tool_outputs(&mut history, &small_config(10));

    let text0 = get_tool_result_text(&history[0]).expect("tr");
    assert!(
        text0.starts_with(STRIP_MARKER_PREFIX),
        "oldest should be stripped, got: {text0}"
    );
    assert!(
        text0.contains("/tmp/bridge_tool_output/uuid-1.txt"),
        "marker should embed the spill path"
    );
    assert!(text0.contains("RipGrep"), "marker should steer to RipGrep");
    assert!(
        text0.contains("journal_write"),
        "marker should steer to journal"
    );

    let text2 = get_tool_result_text(&history[4]).expect("tr");
    assert!(
        !text2.starts_with(STRIP_MARKER_PREFIX),
        "most recent should survive"
    );
}

#[test]
fn test_strip_strips_results_even_without_spill_path() {
    // Previously a 5KB result without a "saved to:" marker was kept inline
    // on the theory that we had nothing to point the agent at for recovery.
    // That made strip a no-op for all non-bash tool outputs (Read, RipGrep,
    // Glob, etc.) and let cumulative context bloat to millions of tokens
    // on long runs. Strip now drops the body regardless of spill; the
    // marker tells the agent to re-call the tool if it needs the content.
    let no_spill = "x".repeat(5_000);
    let mut history = vec![
        make_user_with_tool_result("call-1", &no_spill),
        Message::assistant("Response"),
    ];

    strip_old_tool_outputs(&mut history, &small_config(0));

    let text = get_tool_result_text(&history[0]).expect("tr");
    assert!(
        text.starts_with(STRIP_MARKER_PREFIX),
        "expected strip marker, got: {text}"
    );
    assert!(
        text.contains("original body discarded"),
        "expected no-spill marker text, got: {text}"
    );
}

#[test]
fn test_strip_skips_small_outputs() {
    let mut history = vec![
        make_user_with_tool_result("call-1", "small output"),
        Message::assistant("Response"),
    ];

    strip_old_tool_outputs(&mut history, &small_config(0));

    let text = get_tool_result_text(&history[0]).expect("tr");
    assert_eq!(text, "small output");
}

#[test]
fn test_strip_skips_exempt_tools() {
    let mut history = vec![
        make_user_with_tool_result(
            "journal_read-call-1",
            &body_with_spill(10_000, "uuid-journal"),
        ),
        Message::assistant("Response"),
    ];

    strip_old_tool_outputs(&mut history, &small_config(0));

    let text = get_tool_result_text(&history[0]).expect("tr");
    assert!(!text.starts_with(STRIP_MARKER_PREFIX));
}

#[test]
fn test_strip_pins_errors_when_configured() {
    let error_body = format!(
        "{{ \"output\": \"...\", \"is_error\": true, \"details\": \"{}\" }}\n{}",
        "e".repeat(5_000),
        spill_line("uuid-err"),
    );
    let mut history = vec![
        make_user_with_tool_result("call-1", &error_body),
        Message::assistant("Response"),
    ];

    strip_old_tool_outputs(&mut history, &small_config(0));

    let text = get_tool_result_text(&history[0]).expect("tr");
    assert!(
        !text.starts_with(STRIP_MARKER_PREFIX),
        "pinned error should survive stripping"
    );
}

#[test]
fn test_strip_is_idempotent() {
    // Stripping the same history twice should yield byte-identical
    // output — this is what makes the provider prompt cache stable.
    let mut history = vec![
        make_user_with_tool_result("call-1", &body_with_spill(10_000, "uuid-1")),
        Message::assistant("R1"),
        make_user_with_tool_result("call-2", &body_with_spill(10_000, "uuid-2")),
        Message::assistant("R2"),
    ];

    strip_old_tool_outputs(&mut history, &small_config(1));
    let after_first: Vec<String> = history
        .iter()
        .filter_map(|m| get_tool_result_text(m).map(String::from))
        .collect();

    strip_old_tool_outputs(&mut history, &small_config(1));
    let after_second: Vec<String> = history
        .iter()
        .filter_map(|m| get_tool_result_text(m).map(String::from))
        .collect();

    assert_eq!(
        after_first, after_second,
        "strip should be idempotent for prompt-cache stability"
    );
}

#[test]
fn test_strip_disabled_is_noop() {
    let mut history = vec![
        make_user_with_tool_result("call-1", &body_with_spill(10_000, "uuid-1")),
        Message::assistant("R1"),
    ];

    let config = HistoryStripConfig {
        enabled: false,
        ..HistoryStripConfig::default()
    };
    strip_old_tool_outputs(&mut history, &config);

    let text = get_tool_result_text(&history[0]).expect("tr");
    assert!(
        !text.starts_with(STRIP_MARKER_PREFIX),
        "disabled strip should leave history untouched"
    );
}

#[test]
fn test_strip_no_tool_results() {
    let mut history = vec![
        Message::user("Hello"),
        Message::assistant("Hi there"),
        Message::user("How are you?"),
        Message::assistant("Good!"),
    ];

    let original_len = history.len();
    strip_old_tool_outputs(&mut history, &small_config(1));
    assert_eq!(history.len(), original_len);
}

#[test]
fn test_extract_spill_path_bash_variant() {
    // The bash tool writes its spill marker differently (one line, no
    // "To find specific content" suffix before the ]). Confirm extraction
    // handles both shapes.
    let text = "head chunk\n\n... [Output truncated. Full output (5000 bytes) saved to: /tmp/bridge_bash_abc.txt. To find specific content, call the RipGrep tool with path=\"/tmp/bridge_bash_abc.txt\" and a regex pattern.] ...\n\ntail chunk";
    let tr = rig::message::ToolResult {
        id: "bash".into(),
        call_id: None,
        content: OneOrMany::one(ToolResultContent::Text(Text { text: text.into() })),
    };
    assert_eq!(
        extract_spill_path(&tr).as_deref(),
        Some("/tmp/bridge_bash_abc.txt"),
    );
}
