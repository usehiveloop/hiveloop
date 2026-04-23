use super::Replacer;

/// IndentationFlexibleReplacer — strip leading whitespace, match, reindent.
pub(super) struct IndentationFlexibleReplacer;

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
        sorted.sort_unstable_by_key(|s| std::cmp::Reverse(s.0));
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
