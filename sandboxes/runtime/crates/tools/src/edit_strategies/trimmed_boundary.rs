use super::Replacer;

/// TrimmedBoundaryReplacer — trim first/last lines of old_string.
pub(super) struct TrimmedBoundaryReplacer;

impl Replacer for TrimmedBoundaryReplacer {
    fn name(&self) -> &str {
        "trimmed_boundary"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let old_lines: Vec<&str> = old_string.lines().collect();
        if old_lines.len() < 3 {
            // Need at least 3 lines: boundary + content + boundary
            return None;
        }

        // Trim first and last lines and try to match the inner content
        let inner = &old_lines[1..old_lines.len() - 1];
        let inner_str = inner.join("\n");

        let count = content.matches(&inner_str).count();
        if count == 0 {
            return None;
        }
        if count > 1 && !replace_all {
            return None;
        }

        // The new string replaces just like SimpleReplacer but using the inner match
        if replace_all {
            Some((content.replace(&inner_str, new_string), count))
        } else {
            Some((content.replacen(&inner_str, new_string, 1), 1))
        }
    }
}
