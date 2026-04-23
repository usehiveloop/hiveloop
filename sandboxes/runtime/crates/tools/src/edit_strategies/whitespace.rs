use super::Replacer;

/// WhitespaceNormalizedReplacer — collapse all whitespace to single spaces.
pub(super) struct WhitespaceNormalizedReplacer;

fn normalize_whitespace(s: &str) -> String {
    s.split_whitespace().collect::<Vec<_>>().join(" ")
}

impl Replacer for WhitespaceNormalizedReplacer {
    fn name(&self) -> &str {
        "whitespace_normalized"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let norm_old = normalize_whitespace(old_string);
        let content_lines: Vec<&str> = content.lines().collect();
        let old_lines: Vec<&str> = old_string.lines().collect();

        if old_lines.is_empty() {
            return None;
        }

        // Find blocks where whitespace-normalized content matches
        let mut matches: Vec<usize> = Vec::new();
        for i in 0..=content_lines.len().saturating_sub(old_lines.len()) {
            let block = content_lines[i..i + old_lines.len()].join("\n");
            if normalize_whitespace(&block) == norm_old {
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
        let mut sorted = matches_to_apply.clone();
        sorted.sort_unstable_by(|a, b| b.cmp(a));

        for start in &sorted {
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
