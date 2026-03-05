//! Edit matching strategies inspired by OpenCode.
//!
//! When `apply_edit()` tries to find `old_string` in `content`, it uses a
//! chain of increasingly fuzzy strategies. The first match wins.

/// Trait for replacement strategies.
pub(crate) trait Replacer {
    #[allow(dead_code)]
    fn name(&self) -> &str;
    /// Try to replace `old_string` with `new_string` in `content`.
    /// Returns `Some(new_content)` on success, `None` if the strategy cannot match.
    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)>;
}

/// Return the ordered list of all strategies.
pub(crate) fn all_strategies() -> Vec<Box<dyn Replacer>> {
    vec![
        Box::new(SimpleReplacer),
        Box::new(LineTrimmedReplacer),
        Box::new(BlockAnchorReplacer),
        Box::new(WhitespaceNormalizedReplacer),
        Box::new(IndentationFlexibleReplacer),
        Box::new(EscapeNormalizedReplacer),
        Box::new(TrimmedBoundaryReplacer),
        Box::new(ContextAwareReplacer),
        Box::new(MultiOccurrenceReplacer),
    ]
}

// ---------------------------------------------------------------------------
// 1. SimpleReplacer — exact string match
// ---------------------------------------------------------------------------

struct SimpleReplacer;

impl Replacer for SimpleReplacer {
    fn name(&self) -> &str {
        "simple"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let count = content.matches(old_string).count();
        if count == 0 {
            return None;
        }
        if count > 1 && !replace_all {
            return None; // Ambiguous — let caller handle the error
        }
        if replace_all {
            Some((content.replace(old_string, new_string), count))
        } else {
            Some((content.replacen(old_string, new_string, 1), 1))
        }
    }
}

// ---------------------------------------------------------------------------
// 2. LineTrimmedReplacer — trim each line before matching
// ---------------------------------------------------------------------------

struct LineTrimmedReplacer;

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

// ---------------------------------------------------------------------------
// 3. BlockAnchorReplacer — Levenshtein-based fuzzy block matching
// ---------------------------------------------------------------------------

struct BlockAnchorReplacer;

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
        candidates.sort_by(|a, b| b.0.cmp(&a.0));

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

// ---------------------------------------------------------------------------
// 4. WhitespaceNormalizedReplacer — collapse all whitespace to single spaces
// ---------------------------------------------------------------------------

struct WhitespaceNormalizedReplacer;

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

// ---------------------------------------------------------------------------
// 5. IndentationFlexibleReplacer — strip leading whitespace, match, reindent
// ---------------------------------------------------------------------------

struct IndentationFlexibleReplacer;

fn strip_leading_indent(lines: &[&str]) -> (Vec<String>, String) {
    // Find minimum indentation
    let min_indent = lines
        .iter()
        .filter(|l| !l.trim().is_empty())
        .map(|l| l.len() - l.trim_start().len())
        .min()
        .unwrap_or(0);

    let stripped: Vec<String> = lines
        .iter()
        .map(|l| {
            if l.len() >= min_indent {
                l[min_indent..].to_string()
            } else {
                l.trim_start().to_string()
            }
        })
        .collect();

    let indent = if !lines.is_empty() && !lines[0].trim().is_empty() {
        lines[0][..lines[0].len() - lines[0].trim_start().len()].to_string()
    } else {
        " ".repeat(min_indent)
    };

    (stripped, indent)
}

impl Replacer for IndentationFlexibleReplacer {
    fn name(&self) -> &str {
        "indentation_flexible"
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

        if old_lines.is_empty() {
            return None;
        }

        let (stripped_old, _) = strip_leading_indent(&old_lines);

        let mut matches: Vec<(usize, String)> = Vec::new();
        for i in 0..=content_lines.len().saturating_sub(old_lines.len()) {
            let block: Vec<&str> = content_lines[i..i + old_lines.len()].to_vec();
            let (stripped_block, actual_indent) = strip_leading_indent(&block);
            if stripped_old == stripped_block {
                matches.push((i, actual_indent));
            }
        }

        if matches.is_empty() {
            return None;
        }
        if matches.len() > 1 && !replace_all {
            return None;
        }

        let matches_to_apply = if replace_all {
            matches.clone()
        } else {
            vec![matches[0].clone()]
        };

        let mut result_lines: Vec<String> = content_lines.iter().map(|l| l.to_string()).collect();
        let mut sorted = matches_to_apply;
        sorted.sort_unstable_by(|a, b| b.0.cmp(&a.0));
        let count = sorted.len();

        for (start, actual_indent) in &sorted {
            let end = start + old_lines.len();
            // Reindent new_string with the actual indentation from the file
            let new_reindented: Vec<String> = new_string
                .lines()
                .enumerate()
                .map(|(j, l)| {
                    if j == 0 || l.trim().is_empty() {
                        if l.trim().is_empty() {
                            String::new()
                        } else {
                            format!("{}{}", actual_indent, l.trim_start())
                        }
                    } else {
                        format!("{}{}", actual_indent, l.trim_start())
                    }
                })
                .collect();

            let mut new_result: Vec<String> = Vec::new();
            new_result.extend_from_slice(&result_lines[..*start]);
            new_result.extend(new_reindented);
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

// ---------------------------------------------------------------------------
// 6. EscapeNormalizedReplacer — normalize escape sequences
// ---------------------------------------------------------------------------

struct EscapeNormalizedReplacer;

fn normalize_escapes(s: &str) -> String {
    s.replace("\\n", "\n")
        .replace("\\t", "\t")
        .replace("\\\"", "\"")
        .replace("\\'", "'")
        .replace("\\\\", "\\")
}

impl Replacer for EscapeNormalizedReplacer {
    fn name(&self) -> &str {
        "escape_normalized"
    }

    fn try_replace(
        &self,
        content: &str,
        old_string: &str,
        new_string: &str,
        replace_all: bool,
    ) -> Option<(String, usize)> {
        let norm_old = normalize_escapes(old_string);
        if norm_old == old_string {
            // No escape normalization possible
            return None;
        }

        let count = content.matches(&norm_old).count();
        if count == 0 {
            return None;
        }
        if count > 1 && !replace_all {
            return None;
        }

        let norm_new = normalize_escapes(new_string);
        if replace_all {
            Some((content.replace(&norm_old, &norm_new), count))
        } else {
            Some((content.replacen(&norm_old, &norm_new, 1), 1))
        }
    }
}

// ---------------------------------------------------------------------------
// 7. TrimmedBoundaryReplacer — trim first/last lines of old_string
// ---------------------------------------------------------------------------

struct TrimmedBoundaryReplacer;

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

// ---------------------------------------------------------------------------
// 8. ContextAwareReplacer — use surrounding context lines to locate block
// ---------------------------------------------------------------------------

struct ContextAwareReplacer;

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

// ---------------------------------------------------------------------------
// 9. MultiOccurrenceReplacer — when multiple matches, use context to pick
// ---------------------------------------------------------------------------

struct MultiOccurrenceReplacer;

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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_simple_exact_match() {
        let r = SimpleReplacer;
        let result = r.try_replace("hello world", "world", "rust", false);
        assert_eq!(result, Some(("hello rust".to_string(), 1)));
    }

    #[test]
    fn test_simple_no_match() {
        let r = SimpleReplacer;
        assert!(r.try_replace("hello", "world", "rust", false).is_none());
    }

    #[test]
    fn test_line_trimmed_match() {
        let r = LineTrimmedReplacer;
        let content = "  hello\n  world\n";
        let result = r.try_replace(content, "hello\nworld", "foo\nbar", false);
        assert!(result.is_some());
        let (new_content, count) = result.unwrap();
        assert_eq!(count, 1);
        assert!(new_content.contains("foo"));
    }

    #[test]
    fn test_whitespace_normalized() {
        let r = WhitespaceNormalizedReplacer;
        let content = "hello   world\n";
        let result = r.try_replace(content, "hello world", "hello rust", false);
        assert!(result.is_some());
    }

    #[test]
    fn test_indentation_flexible() {
        let r = IndentationFlexibleReplacer;
        let content = "    fn main() {\n        println!(\"old\");\n    }\n";
        let old = "fn main() {\n    println!(\"old\");\n}";
        let new = "fn main() {\n    println!(\"new\");\n}";
        let result = r.try_replace(content, old, new, false);
        assert!(result.is_some());
        let (new_content, _) = result.unwrap();
        assert!(new_content.contains("new"));
    }

    #[test]
    fn test_escape_normalized() {
        let r = EscapeNormalizedReplacer;
        let content = "line one\nline two\n";
        let result = r.try_replace(content, "line one\\nline two", "replaced", false);
        assert!(result.is_some());
    }

    #[test]
    fn test_block_anchor_fuzzy() {
        let r = BlockAnchorReplacer;
        let content = "fn main() {\n    println!(\"hello\");\n}\n";
        // Slightly different: extra space
        let old = "fn main()  {\n    println!(\"hello\");\n}";
        let new = "fn main() {\n    println!(\"world\");\n}";
        let result = r.try_replace(content, old, new, false);
        assert!(result.is_some());
    }

    #[test]
    fn test_multi_occurrence_replace_all() {
        let r = MultiOccurrenceReplacer;
        let content = "aaa\nbbb\naaa\n";
        let result = r.try_replace(content, "aaa", "ccc", true);
        assert!(result.is_some());
        let (new_content, count) = result.unwrap();
        assert_eq!(count, 2);
        assert!(!new_content.contains("aaa"));
    }

    #[test]
    fn test_all_strategies_chain() {
        let strategies = all_strategies();
        assert_eq!(strategies.len(), 9);

        // Test that the chain finds an exact match
        let content = "hello world";
        for strategy in &strategies {
            if let Some((result, count)) = strategy.try_replace(content, "world", "rust", false) {
                assert_eq!(result, "hello rust");
                assert_eq!(count, 1);
                assert_eq!(strategy.name(), "simple"); // Should be found by first strategy
                return;
            }
        }
        panic!("No strategy matched");
    }
}
