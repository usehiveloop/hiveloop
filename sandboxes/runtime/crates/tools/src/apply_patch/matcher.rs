//! Sequence matching (4-pass with fallback) used by patch application.

fn normalize_unicode(s: &str) -> String {
    use unicode_normalization::UnicodeNormalization;
    s.nfkc().collect()
}

fn try_match<F>(
    lines: &[String],
    pattern: &[String],
    start_index: usize,
    compare: F,
    eof: bool,
) -> Option<usize>
where
    F: Fn(&str, &str) -> bool,
{
    if pattern.is_empty() || pattern.len() > lines.len() {
        return None;
    }

    // If EOF anchor, try matching from end of file first
    if eof {
        let from_end = lines.len().saturating_sub(pattern.len());
        if from_end >= start_index {
            let matches = pattern
                .iter()
                .enumerate()
                .all(|(j, p)| compare(&lines[from_end + j], p));
            if matches {
                return Some(from_end);
            }
        }
    }

    // Forward search from start_index
    let max_start = lines.len().saturating_sub(pattern.len());
    for i in start_index..=max_start {
        let matches = pattern
            .iter()
            .enumerate()
            .all(|(j, p)| compare(&lines[i + j], p));
        if matches {
            return Some(i);
        }
    }

    None
}

pub(super) fn seek_sequence(
    lines: &[String],
    pattern: &[String],
    start_index: usize,
    eof: bool,
) -> Option<usize> {
    if pattern.is_empty() {
        return None;
    }

    // Pass 1: exact match
    if let Some(idx) = try_match(lines, pattern, start_index, |a, b| a == b, eof) {
        return Some(idx);
    }

    // Pass 2: rstrip (trim trailing whitespace)
    if let Some(idx) = try_match(
        lines,
        pattern,
        start_index,
        |a, b| a.trim_end() == b.trim_end(),
        eof,
    ) {
        return Some(idx);
    }

    // Pass 3: trim (both ends)
    if let Some(idx) = try_match(
        lines,
        pattern,
        start_index,
        |a, b| a.trim() == b.trim(),
        eof,
    ) {
        return Some(idx);
    }

    // Pass 4: normalized Unicode
    try_match(
        lines,
        pattern,
        start_index,
        |a, b| normalize_unicode(a.trim()) == normalize_unicode(b.trim()),
        eof,
    )
}
