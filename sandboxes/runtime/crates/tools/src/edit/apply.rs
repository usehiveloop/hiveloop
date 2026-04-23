/// Normalize CRLF and CR line endings to LF.
pub(super) fn normalize_line_endings(text: &str) -> String {
    text.replace("\r\n", "\n").replace('\r', "\n")
}

/// Shared edit logic used by both Edit and MultiEdit tools.
///
/// Applies a single find-and-replace operation on `content`.
/// Uses a chain of 9 matching strategies (exact → fuzzy) in order.
/// Returns the new content on success.
pub(crate) fn apply_edit(
    content: &str,
    old_string: &str,
    new_string: &str,
    replace_all: bool,
) -> Result<(String, usize), String> {
    if old_string == new_string {
        return Err("oldString and newString are identical".to_string());
    }

    // Try each strategy in order — first match wins
    for strategy in crate::edit_strategies::all_strategies() {
        if let Some((new_content, count)) =
            strategy.try_replace(content, old_string, new_string, replace_all)
        {
            return Ok((new_content, count));
        }
    }

    // No strategy matched — check if there were multiple matches that
    // prevented a non-replace_all edit from succeeding
    let exact_count = content.matches(old_string).count();
    if exact_count > 1 && !replace_all {
        return Err(
            "Found multiple matches for oldString. Provide more surrounding lines in oldString to identify the correct match, or use replaceAll.".to_string()
        );
    }

    Err("oldString not found in file content".to_string())
}

pub(super) fn snippet(s: &str, max_len: usize) -> String {
    if s.len() <= max_len {
        s.to_string()
    } else {
        let end = s.floor_char_boundary(max_len);
        format!("{}...", &s[..end])
    }
}
