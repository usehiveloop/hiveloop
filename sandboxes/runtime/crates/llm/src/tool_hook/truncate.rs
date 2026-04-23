//! Tool-result truncation, argument validation, and the `Truncated` Display wrapper.

/// Maximum bytes for a single tool result entering conversation history (~2KB).
/// Any tool result exceeding this cap is truncated to a head+tail window with
/// the full output persisted to disk; the agent is pointed at the spill file
/// via the `RipGrep` tool for any deeper inspection. Applies uniformly to every
/// tool: builtin, MCP, integration, skill, and subagent results.
pub(super) const TOOL_RESULT_MAX_BYTES: usize = 2048;

/// Validate tool arguments against a JSON schema.
/// Returns Ok(()) if valid, Err(message) with human-readable validation errors if not.
pub(super) fn validate_tool_args(
    args: &serde_json::Value,
    schema: &serde_json::Value,
) -> Result<(), String> {
    // Skip validation for empty/trivial schemas
    if schema.is_null() || schema == &serde_json::json!({}) {
        return Ok(());
    }

    let validator = match jsonschema::validator_for(schema) {
        Ok(v) => v,
        Err(_) => return Ok(()), // If schema itself is invalid, skip validation
    };

    let errors: Vec<String> = validator
        .iter_errors(args)
        .map(|e| {
            let path = e.instance_path().to_string();
            if path.is_empty() {
                e.to_string()
            } else {
                format!("{}: {}", path, e)
            }
        })
        .collect();

    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors.join("; "))
    }
}

/// Truncate a tool result string if it exceeds the safety net threshold.
/// Returns the original string if within limits. When over the threshold, the
/// full output is spilled to disk and the in-context payload keeps a head+tail
/// window plus a pointer to the spill file.
pub(super) fn truncate_if_needed(result: String) -> String {
    if result.len() <= TOOL_RESULT_MAX_BYTES {
        return result;
    }
    let truncated = tools::truncation::truncate_output_directed(
        &result,
        tools::truncation::MAX_LINES,
        TOOL_RESULT_MAX_BYTES,
        tools::truncation::TruncationDirection::HeadTail,
    );
    truncated.content
}

/// A zero-allocation Display wrapper that truncates a string slice on output.
/// Only performs formatting work when actually rendered (i.e., when the log
/// event is enabled), making it truly zero-cost when filtered out.
pub(super) struct Truncated<'a> {
    s: &'a str,
    max_len: usize,
}

impl<'a> Truncated<'a> {
    pub(super) fn new(s: &'a str, max_len: usize) -> Self {
        Self { s, max_len }
    }
}

impl std::fmt::Display for Truncated<'_> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        if self.s.len() <= self.max_len {
            f.write_str(self.s)
        } else {
            // Find a char-safe boundary at or before max_len
            let boundary = self.s.floor_char_boundary(self.max_len);
            write!(
                f,
                "{}...[truncated, {} bytes total]",
                &self.s[..boundary],
                self.s.len()
            )
        }
    }
}
