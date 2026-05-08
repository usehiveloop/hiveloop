use similar::TextDiff;
use unicode_normalization::UnicodeNormalization;

#[derive(Debug, thiserror::Error)]
#[allow(dead_code)]
pub enum EditMatchError {
    #[error("could not find the exact text in the file")]
    NotFound,
    #[error("multiple matches found for the text; provide more context to disambiguate")]
    Ambiguous,
    #[error("edit at index {0} overlaps with another edit in the same call")]
    Overlap(usize),
}

pub struct PendingEdit {
    pub old_text: String,
    pub new_text: String,
}

struct ResolvedEdit {
    start: usize,
    end: usize,
    new_text: String,
}

pub fn apply_edits(content: &str, edits: &[PendingEdit]) -> Result<String, EditMatchError> {
    if edits.is_empty() {
        return Ok(content.to_string());
    }
    let mut resolved: Vec<(usize, ResolvedEdit)> = Vec::with_capacity(edits.len());
    for (index, edit) in edits.iter().enumerate() {
        let (start, end) = locate_match(content, &edit.old_text).ok_or(EditMatchError::NotFound)?;
        resolved.push((
            index,
            ResolvedEdit {
                start,
                end,
                new_text: edit.new_text.clone(),
            },
        ));
    }
    resolved.sort_by_key(|(_, edit)| edit.start);
    for window in resolved.windows(2) {
        if window[0].1.end > window[1].1.start {
            return Err(EditMatchError::Overlap(window[1].0));
        }
    }
    resolved.sort_by(|left, right| right.1.start.cmp(&left.1.start));
    let mut result = content.to_string();
    for (_, edit) in resolved {
        result.replace_range(edit.start..edit.end, &edit.new_text);
    }
    Ok(result)
}

fn locate_match(haystack: &str, needle: &str) -> Option<(usize, usize)> {
    if needle.is_empty() {
        return None;
    }
    if let Some(start) = haystack.find(needle) {
        let mut occurrences = haystack.match_indices(needle);
        let _first = occurrences.next();
        if occurrences.next().is_some() {
            return None;
        }
        return Some((start, start + needle.len()));
    }
    locate_fuzzy_match(haystack, needle)
}

fn locate_fuzzy_match(haystack: &str, needle: &str) -> Option<(usize, usize)> {
    let normalized_needle = normalize_for_match(needle);
    let mut current_normalized = String::new();
    let chars: Vec<(usize, char)> = haystack.char_indices().collect();
    let mut left = 0usize;
    while left < chars.len() {
        let mut right = left;
        current_normalized.clear();
        while right < chars.len() {
            let (start_index, ch) = chars[right];
            let normalized_char: String = std::iter::once(ch).nfkc().collect();
            for normalized in normalized_char.chars() {
                current_normalized.push(map_char_for_match(normalized));
            }
            let last_end = start_index + ch.len_utf8();
            if current_normalized.len() >= normalized_needle.len()
                && current_normalized == normalized_needle
            {
                return Some((chars[left].0, last_end));
            }
            if current_normalized.len() > normalized_needle.len() {
                break;
            }
            right += 1;
        }
        left += 1;
    }
    None
}

pub fn normalize_for_match(input: &str) -> String {
    input
        .nfkc()
        .map(map_char_for_match)
        .collect()
}

fn map_char_for_match(ch: char) -> char {
    match ch {
        '\u{2018}' | '\u{2019}' | '\u{201A}' | '\u{201B}' => '\'',
        '\u{201C}' | '\u{201D}' | '\u{201E}' | '\u{201F}' => '"',
        '\u{2013}' | '\u{2014}' | '\u{2212}' => '-',
        '\u{00A0}' | '\u{2009}' | '\u{200A}' | '\u{2002}' | '\u{2003}' => ' ',
        other => other,
    }
}

pub fn unified_diff(before: &str, after: &str, path: &str) -> String {
    let diff = TextDiff::from_lines(before, after);
    let mut formatter = diff.unified_diff();
    formatter.context_radius(3);
    formatter.header(&format!("a/{path}"), &format!("b/{path}"));
    format!("{}", formatter)
}
