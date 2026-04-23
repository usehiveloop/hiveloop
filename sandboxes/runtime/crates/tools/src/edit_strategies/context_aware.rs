use super::Replacer;

/// ContextAwareReplacer — use surrounding context lines to locate block.
pub(super) struct ContextAwareReplacer;

impl Replacer for ContextAwareReplacer {
    fn name(&self) -> &str {
        "context_aware"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        _replace_all: bool,
    ) -> Option<(String, usize)> {
        // This strategy takes the first and last lines of old_string as
        // context anchors and replaces everything between them (inclusive)
        let old_lines: Vec<&str> = old_string.lines().collect();
        if old_lines.len() < 2 {
            return None;
        }

        let first = old_lines[0].trim();
        let last = old_lines[old_lines.len() - 1].trim();
        if first.is_empty() || last.is_empty() {
            return None;
        }

        let content_lines: Vec<&str> = content.lines().collect();

        // Find the first line that matches the first context anchor
        let start = content_lines.iter().position(|l| l.trim() == first)?;

        // Find the last line (after start) that matches the last context anchor
        let end = content_lines[start..]
            .iter()
            .rposition(|l| l.trim() == last)
            .map(|i| i + start)?;

        if end < start || (end - start + 1) != old_lines.len() {
            return None;
        }

        // Verify inner lines match approximately
        let block = &content_lines[start..=end];
        let sim = strsim::normalized_levenshtein(&block.join("\n"), &old_lines.join("\n"));
        if sim < 0.7 {
            return None;
        }

        let new_lines_vec: Vec<&str> = new_string.lines().collect();
        let mut result_lines: Vec<&str> = Vec::new();
        result_lines.extend_from_slice(&content_lines[..start]);
        result_lines.extend_from_slice(&new_lines_vec);
        result_lines.extend_from_slice(&content_lines[end + 1..]);

        let mut new_content = result_lines.join("\n");
        if content.ends_with('\n') && !new_content.ends_with('\n') {
            new_content.push('\n');
        }

        Some((new_content, 1))
    }
}
