use super::Replacer;

/// BlockAnchorReplacer — Levenshtein-based fuzzy block matching.
pub(super) struct BlockAnchorReplacer;

impl Replacer for BlockAnchorReplacer {
    fn name(&self) -> &str {
        "block_anchor"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let old_lines: Vec<&str> = old_string.lines().collect();
        let content_lines: Vec<&str> = content.lines().collect();

        if old_lines.is_empty() || old_lines.len() > content_lines.len() {
            return None;
        }

        // Find candidate blocks using Levenshtein similarity
        let old_block = old_lines.join("\n");
        let mut candidates: Vec<(usize, f64)> = Vec::new();

        for i in 0..=content_lines.len().saturating_sub(old_lines.len()) {
            let block = content_lines[i..i + old_lines.len()].join("\n");
            let sim = strsim::normalized_levenshtein(&old_block, &block);
            if sim > 0.6 {
                candidates.push((i, sim));
            }
        }

        if candidates.is_empty() {
            return None;
        }

        // Threshold: single candidate = 0.0 (any match), multiple = 0.3 similarity gap
        if candidates.len() > 1 && !replace_all {
            candidates.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
            let gap = candidates[0].1 - candidates[1].1;
            if gap < 0.3 {
                return None; // Too ambiguous
            }
            // Take only the best
            candidates.truncate(1);
        }

        if !replace_all && candidates.len() > 1 {
            return None;
        }

        // Sort by position descending for reverse-order replacement
        candidates.sort_by_key(|c| std::cmp::Reverse(c.0));

        let new_lines_vec: Vec<&str> = new_string.lines().collect();
        let mut result_lines: Vec<&str> = content_lines;
        let count = candidates.len();

        for (start, _) in &candidates {
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

        Some((new_content, count))
    }
}
