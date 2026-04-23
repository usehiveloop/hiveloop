use super::Replacer;

/// LineTrimmedReplacer — trim each line before matching.
pub(super) struct LineTrimmedReplacer;

impl Replacer for LineTrimmedReplacer {
    fn name(&self) -> &str {
        "line_trimmed"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let old_lines: Vec<&str> = old_string.lines().map(|l| l.trim()).collect();
        let content_lines: Vec<&str> = content.lines().collect();
        let content_trimmed: Vec<&str> = content_lines.iter().map(|l| l.trim()).collect();

        if old_lines.is_empty() {
            return None;
        }

        let mut matches: Vec<usize> = Vec::new();
        for i in 0..=content_trimmed.len().saturating_sub(old_lines.len()) {
            if content_trimmed[i..i + old_lines.len()] == old_lines[..] {
                matches.push(i);
            }
        }

        if matches.is_empty() {
            return None;
        }
        if matches.len() > 1 && !replace_all {
            return None;
        }

        let new_lines_vec: Vec<&str> = new_string.lines().collect();
        let matches_to_apply = if replace_all {
            matches.clone()
        } else {
            vec![matches[0]]
        };

        let mut result_lines: Vec<&str> = content_lines;
        let mut sorted_matches = matches_to_apply.clone();
        sorted_matches.sort_unstable_by(|a, b| b.cmp(a));

        for start in &sorted_matches {
            let end = start + old_lines.len();
            let mut new_result: Vec<&str> = Vec::new();
            new_result.extend_from_slice(&result_lines[..*start]);
            new_result.extend_from_slice(&new_lines_vec);
            new_result.extend_from_slice(&result_lines[end..]);
            result_lines = new_result;
        }

        let mut new_content = result_lines.join("\n");
        if content.ends_with('\n') && !new_content.ends_with('\n') {
            new_content.push('\n');
        }

        Some((new_content, matches_to_apply.len()))
    }
}
