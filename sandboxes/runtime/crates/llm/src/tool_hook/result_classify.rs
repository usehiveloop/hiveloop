//! Heuristic classifier for "tool returned successfully but the result content
//! describes a failure".
//!
//! Distinct from `is_error`, which means the executor itself raised
//! (`Result::Err` from `ToolExecutor::execute`). Several built-in tools (Read,
//! Write, Edit, bash) return error messages inside `Ok(String)` rather than
//! raising — so without this classifier, bridge counts those as successes
//! even when the model received a "File not found" or non-zero exit code.
//!
//! The classifier is conservative on purpose. False positives are worse than
//! false negatives here: silently flagging a normal call as a failure would
//! mislead operators reading metrics. So we only return `true` when the
//! result string matches a small, well-known failure shape.

/// Decide whether a tool result string represents a failure.
///
/// Recognised failure shapes:
/// - Plain text result starts with `"Toolset error"` or `"ToolCallError"`
///   (the prefix bridge's `tool_adapter` and rig's `ToolSetError` produce).
/// - JSON object whose top-level shape is `{"error": ...}` (with or without
///   sibling fields). Built-in tools use this for validation rejections.
/// - JSON object containing `"is_error": true` at the top level.
/// - JSON object containing `"exit_code": N` where N is a non-zero integer
///   (the bash tool's normal output shape on a failed command).
pub(crate) fn looks_like_failure(result: &str) -> bool {
    let trimmed = result.trim_start();
    if trimmed.starts_with("Toolset error") || trimmed.starts_with("ToolCallError") {
        return true;
    }
    // JSON-shape checks. Parse only when the first non-space char is `{`
    // to avoid an allocation on every plain-text result.
    if !trimmed.starts_with('{') {
        return false;
    }
    let Ok(v) = serde_json::from_str::<serde_json::Value>(trimmed) else {
        return false;
    };
    let Some(obj) = v.as_object() else {
        return false;
    };
    if obj.contains_key("error") {
        return true;
    }
    if obj.get("is_error").and_then(|v| v.as_bool()) == Some(true) {
        return true;
    }
    if let Some(code) = obj.get("exit_code").and_then(|v| v.as_i64()) {
        if code != 0 {
            return true;
        }
    }
    false
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn flags_toolset_error_prefix() {
        assert!(looks_like_failure(
            "Toolset error: ToolCallError: ToolCallError: File not found: /x"
        ));
    }

    #[test]
    fn flags_toolcallerror_prefix() {
        assert!(looks_like_failure("ToolCallError: bad args"));
    }

    #[test]
    fn flags_leading_whitespace_then_toolset_error() {
        assert!(looks_like_failure("  \nToolset error: foo"));
    }

    #[test]
    fn flags_json_with_error_field() {
        assert!(looks_like_failure(
            r#"{"error":"Invalid arguments for tool 'edit': /replace_all"}"#
        ));
    }

    #[test]
    fn flags_json_with_is_error_true() {
        assert!(looks_like_failure(r#"{"output":"...","is_error":true}"#));
    }

    #[test]
    fn flags_bash_nonzero_exit() {
        assert!(looks_like_failure(
            r#"{"output":"command not found","exit_code":127,"timed_out":false}"#
        ));
    }

    #[test]
    fn does_not_flag_bash_zero_exit() {
        assert!(!looks_like_failure(
            r#"{"output":"hello","exit_code":0,"timed_out":false}"#
        ));
    }

    #[test]
    fn does_not_flag_normal_read_result() {
        assert!(!looks_like_failure(
            r#"{"content":"1: foo\n2: bar","truncated":false}"#
        ));
    }

    #[test]
    fn does_not_flag_arbitrary_text_with_error_word() {
        // The string contains the word "error" but isn't shaped as a failure.
        assert!(!looks_like_failure(
            "Configured error handler installed at /etc/x"
        ));
    }

    #[test]
    fn does_not_flag_plain_text_starting_with_error_in_text() {
        // Prose starts with "Error" but no JSON, no Toolset/ToolCall prefix.
        // Conservative: don't flag — would be too eager.
        assert!(!looks_like_failure("Error formatting output: try again"));
    }

    #[test]
    fn does_not_flag_empty_string() {
        assert!(!looks_like_failure(""));
    }

    #[test]
    fn does_not_flag_malformed_json() {
        // Starts with `{` but isn't valid JSON — don't crash, don't flag.
        assert!(!looks_like_failure(r#"{"foo": this isn't json"#));
    }
}
