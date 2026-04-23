use rig::message::ToolResultContent;

/// Minimum size for a tool result to be considered for stripping.
/// Tiny results aren't worth stripping — the marker would be nearly as large.
pub(super) const MIN_STRIPPABLE_BYTES: usize = 200;

/// Tools whose output should never be stripped.
/// These are semantic/metadata tools, not data-producing tools.
pub(super) const EXEMPT_TOOLS: &[&str] =
    &["journal_read", "journal_write", "todoread", "todowrite"];

/// Per-result byte slot used to translate `age_threshold` (assistant-message
/// count) into a byte budget. Every tool result is capped at ~2KB by the
/// central tool-hook, so `age_threshold * SLOT ≈ "keep the last N results"`.
pub(super) const PER_RESULT_SLOT_BYTES: usize = 2048;

/// Marker used by the stripping pass. Its presence is how subsequent turns
/// recognise a result as already-stripped (idempotency — critical for
/// provider prompt-cache stability).
pub(super) const STRIP_MARKER_PREFIX: &str = "[Tool output stripped";

pub(super) fn build_strip_marker(bytes: usize, spill_path: &str) -> String {
    format!(
        "{STRIP_MARKER_PREFIX} — {bytes} bytes. Full output at {spill_path}. \
        Retrieve: call RipGrep with path=\"{spill_path}\" and a regex pattern. \
        Durable notes: call journal_write — values restated in assistant text \
        can still be stripped on later turns, journal entries survive.]"
    )
}

/// Count the total bytes in a ToolResult's text content.
pub(super) fn tool_result_byte_count(tr: &rig::message::ToolResult) -> usize {
    tr.content
        .iter()
        .map(|c| match c {
            ToolResultContent::Text(t) => t.text.len(),
            other => format!("{:?}", other).len(),
        })
        .sum()
}

pub(super) fn is_exempt(tool_id: &str) -> bool {
    EXEMPT_TOOLS.iter().any(|name| tool_id.contains(name))
}

pub(super) fn is_already_stripped(tr: &rig::message::ToolResult) -> bool {
    tr.content.iter().any(|c| match c {
        ToolResultContent::Text(t) => t.text.starts_with(STRIP_MARKER_PREFIX),
        _ => false,
    })
}

/// Heuristic: the tool_hook / agent_runner serialize errors into the result
/// body as JSON-ish text that includes `"is_error":true` (whitespace-
/// insensitive). Rig's `ToolResult` has no `is_error` field, so this is the
/// only signal available at this layer.
pub(super) fn looks_like_error(tr: &rig::message::ToolResult) -> bool {
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
pub(super) fn extract_spill_path(tr: &rig::message::ToolResult) -> Option<String> {
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
