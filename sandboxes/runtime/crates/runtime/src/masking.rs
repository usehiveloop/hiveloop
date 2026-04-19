use bridge_core::agent::HistoryStripConfig;
use rig::message::{Message, ToolResultContent, UserContent};
use rig::one_or_many::OneOrMany;

/// Minimum size for a tool result to be considered for stripping.
/// Tiny results aren't worth stripping — the marker would be nearly as large.
const MIN_STRIPPABLE_BYTES: usize = 200;

/// Tools whose output should never be stripped.
/// These are semantic/metadata tools, not data-producing tools.
const EXEMPT_TOOLS: &[&str] = &["journal_read", "journal_write", "todoread", "todowrite"];

/// Per-result byte slot used to translate `age_threshold` (assistant-message
/// count) into a byte budget. Every tool result is capped at ~2KB by the
/// central tool-hook, so `age_threshold * SLOT ≈ "keep the last N results"`.
const PER_RESULT_SLOT_BYTES: usize = 2048;

/// Marker used by the stripping pass. Its presence is how subsequent turns
/// recognise a result as already-stripped (idempotency — critical for
/// provider prompt-cache stability).
const STRIP_MARKER_PREFIX: &str = "[Tool output stripped";

/// Strip old tool-result bodies from `history` in place.
///
/// Walks backward, preserving the most recent window of tool output (sized
/// via `config.age_threshold` * ~2KB-per-result) and replacing older bodies
/// with a short pointer that names the on-disk spill file (from the 2KB-cap
/// pipeline) so the agent can recover specifics via `RipGrep`.
///
/// Determinism guarantee: for a given input history and config, the output
/// is byte-identical across calls. Once a result is stripped, it stays
/// stripped (we detect the marker prefix and skip). This keeps the provider
/// prompt cache hot after the first strip.
///
/// Skips in order of precedence:
///   1. Non-`User::ToolResult` content.
///   2. Already-stripped results (marker prefix present).
///   3. Exempt tools (journal, todo).
///   4. Results with `is_error: true` when `config.pin_errors` is set.
///   5. Results with no detectable spill path — small enough to keep inline.
///   6. Results whose bytes fit within the protection window.
pub fn strip_old_tool_outputs(history: &mut [Message], config: &HistoryStripConfig) {
    if !config.enabled {
        return;
    }

    let protection_budget = config.age_threshold.saturating_mul(PER_RESULT_SLOT_BYTES);

    // Phase 1: Walk backward, identify which message indices to strip.
    let mut protected_bytes: usize = 0;
    let mut should_strip: std::collections::HashSet<usize> = std::collections::HashSet::new();

    for msg_idx in (0..history.len()).rev() {
        let content = match &history[msg_idx] {
            Message::User { content } => content,
            _ => continue,
        };

        for part in content.iter() {
            let tr = match part {
                UserContent::ToolResult(tr) => tr,
                _ => continue,
            };

            let text_bytes = tool_result_byte_count(tr);

            if text_bytes < MIN_STRIPPABLE_BYTES {
                continue;
            }
            if is_exempt(&tr.id) {
                continue;
            }
            if is_already_stripped(tr) {
                // Counts against the protection budget so results further back
                // don't slip into the protected window when one ahead of them
                // is already a pointer-sized marker.
                protected_bytes += text_bytes;
                continue;
            }
            if config.pin_errors && looks_like_error(tr) {
                protected_bytes += text_bytes;
                continue;
            }
            if extract_spill_path(tr).is_none() {
                // Without a spill path we'd lose the content entirely — leave
                // inline. Results that have no spill path are by definition
                // small (<2KB), so keeping them costs little.
                protected_bytes += text_bytes;
                continue;
            }

            if protected_bytes + text_bytes <= protection_budget {
                protected_bytes += text_bytes;
            } else {
                should_strip.insert(msg_idx);
            }
        }
    }

    // Phase 2: Rewrite stripped messages.
    for msg_idx in &should_strip {
        let new_msg = {
            let content = match &history[*msg_idx] {
                Message::User { content } => content,
                _ => continue,
            };

            let new_parts: Vec<UserContent> = content
                .iter()
                .map(|part| match part {
                    UserContent::ToolResult(tr) => {
                        if should_strip_result(tr, config) {
                            let bytes = tool_result_byte_count(tr);
                            let spill_path = extract_spill_path(tr).unwrap_or_default();
                            UserContent::ToolResult(rig::message::ToolResult {
                                id: tr.id.clone(),
                                call_id: tr.call_id.clone(),
                                content: OneOrMany::one(ToolResultContent::Text(
                                    rig::message::Text {
                                        text: build_strip_marker(bytes, &spill_path),
                                    },
                                )),
                            })
                        } else {
                            part.clone()
                        }
                    }
                    other => other.clone(),
                })
                .collect();

            match OneOrMany::many(new_parts) {
                Ok(new_content) => Some(Message::User {
                    content: new_content,
                }),
                Err(_) => None,
            }
        };

        if let Some(msg) = new_msg {
            history[*msg_idx] = msg;
        }
    }
}

/// Strip with default config (enabled, standard thresholds). Convenience
/// wrapper used in tests and by call sites that don't carry a per-agent
/// config yet.
pub fn strip_old_tool_outputs_default(history: &mut [Message]) {
    strip_old_tool_outputs(history, &HistoryStripConfig::default());
}

fn should_strip_result(tr: &rig::message::ToolResult, config: &HistoryStripConfig) -> bool {
    if is_exempt(&tr.id) {
        return false;
    }
    if is_already_stripped(tr) {
        return false;
    }
    if config.pin_errors && looks_like_error(tr) {
        return false;
    }
    if tool_result_byte_count(tr) < MIN_STRIPPABLE_BYTES {
        return false;
    }
    extract_spill_path(tr).is_some()
}

fn build_strip_marker(bytes: usize, spill_path: &str) -> String {
    format!(
        "{STRIP_MARKER_PREFIX} — {bytes} bytes. Full output at {spill_path}. \
        Retrieve: call RipGrep with path=\"{spill_path}\" and a regex pattern. \
        Durable notes: call journal_write — values restated in assistant text \
        can still be stripped on later turns, journal entries survive.]"
    )
}

/// Count the total bytes in a ToolResult's text content.
fn tool_result_byte_count(tr: &rig::message::ToolResult) -> usize {
    tr.content
        .iter()
        .map(|c| match c {
            ToolResultContent::Text(t) => t.text.len(),
            other => format!("{:?}", other).len(),
        })
        .sum()
}

fn is_exempt(tool_id: &str) -> bool {
    EXEMPT_TOOLS.iter().any(|name| tool_id.contains(name))
}

fn is_already_stripped(tr: &rig::message::ToolResult) -> bool {
    tr.content.iter().any(|c| match c {
        ToolResultContent::Text(t) => t.text.starts_with(STRIP_MARKER_PREFIX),
        _ => false,
    })
}

/// Heuristic: the tool_hook / agent_runner serialize errors into the result
/// body as JSON-ish text that includes `"is_error":true` (whitespace-
/// insensitive). Rig's `ToolResult` has no `is_error` field, so this is the
/// only signal available at this layer.
fn looks_like_error(tr: &rig::message::ToolResult) -> bool {
    tr.content.iter().any(|c| match c {
        ToolResultContent::Text(t) => {
            let compact: String = t.text.chars().filter(|ch| !ch.is_whitespace()).collect();
            compact.contains("\"is_error\":true")
        }
        _ => false,
    })
}

/// Extract the spill-file path embedded by the 2KB-cap pipeline. Handles two
/// marker shapes:
///
///   - `tools::truncation`:           "Full output saved to: <path>"
///   - `tools::bash::truncate_output`: "Full output (N bytes) saved to: <path>"
///
/// Both contain the substring "saved to: " immediately before the path, and
/// the path itself is terminated by whitespace or the "." that precedes the
/// "To find..." sentence. Returns `None` if no marker is present — the
/// result is small enough to keep inline.
fn extract_spill_path(tr: &rig::message::ToolResult) -> Option<String> {
    const NEEDLE: &str = "saved to: ";
    for c in tr.content.iter() {
        if let ToolResultContent::Text(t) = c {
            if let Some(start) = t.text.find(NEEDLE) {
                let after = &t.text[start + NEEDLE.len()..];
                let end = after
                    .find([' ', '\n', '\t', '\r'])
                    .or_else(|| after.find(". To find"))
                    .unwrap_or(after.len());
                let path = after[..end].trim_end_matches(|c: char| ".,;)]".contains(c));
                if !path.is_empty() {
                    return Some(path.to_string());
                }
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
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
    fn test_strip_skips_results_without_spill_path() {
        // A 5KB result without a spill marker is small enough to keep inline
        // (no file exists to point at).
        let no_spill = "x".repeat(5_000);
        let mut history = vec![
            make_user_with_tool_result("call-1", &no_spill),
            Message::assistant("Response"),
        ];

        strip_old_tool_outputs(&mut history, &small_config(0));

        let text = get_tool_result_text(&history[0]).expect("tr");
        assert!(!text.starts_with(STRIP_MARKER_PREFIX));
        assert_eq!(text.len(), 5_000);
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
}
