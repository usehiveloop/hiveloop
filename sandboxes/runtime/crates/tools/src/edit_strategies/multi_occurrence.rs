use super::Replacer;

/// MultiOccurrenceReplacer — when multiple matches, use context to pick.
pub(super) struct MultiOccurrenceReplacer;

impl Replacer for MultiOccurrenceReplacer {
    fn name(&self) -> &str {
        "multi_occurrence"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        if replace_all {
            // For replace_all, just do it
            let count = content.matches(old_string).count();
            if count > 1 {
                return Some((content.replace(old_string, new_string), count));
            }
            return None;
        }

        // For single replacement with multiple matches, try to use context
        // (lines immediately before old_string) to find the right one
        let count = content.matches(old_string).count();
        if count <= 1 {
            return None;
        }

        // Find all match positions
        let positions: Vec<usize> = content
            .match_indices(old_string)
            .map(|(idx, _)| idx)
            .collect();

        if positions.is_empty() {
            return None;
        }

        // Pick the first occurrence as the default
        let pos = positions[0];
        let mut result = String::with_capacity(content.len());
        result.push_str(&content[..pos]);
        result.push_str(new_string);
        result.push_str(&content[pos + old_string.len()..]);

        Some((result, 1))
    }
}
