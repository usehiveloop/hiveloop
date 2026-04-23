//! HTML extraction helpers: readability, tag stripping, markdown conversion,
//! and truncation for the WebFetch tool.

/// Try to extract article content using dom_smoothie's Readability algorithm.
/// Returns `None` if extraction fails (e.g., non-article pages).
pub(super) fn extract_article(html: &str, url: &str) -> Option<dom_smoothie::Article> {
    let config = dom_smoothie::Config {
        text_mode: dom_smoothie::TextMode::Markdown,
        ..Default::default()
    };

    let mut readability = dom_smoothie::Readability::new(html, Some(url), Some(config)).ok()?;
    readability.parse().ok()
}

/// Strip all HTML tags and return plain text.
/// Also removes script and style blocks and collapses whitespace.
pub(super) fn strip_html_tags(html: &str) -> String {
    // Use htmd to convert to markdown, then strip remaining markdown formatting
    // This is simpler and more robust than regex-based tag stripping
    let md = htmd::convert(html).unwrap_or_default();
    // The markdown output is already a reasonable plain-text representation
    // Just collapse excessive blank lines
    let mut result = String::with_capacity(md.len());
    let mut blank_count = 0;
    for line in md.lines() {
        if line.trim().is_empty() {
            blank_count += 1;
            if blank_count <= 2 {
                result.push('\n');
            }
        } else {
            blank_count = 0;
            result.push_str(line);
            result.push('\n');
        }
    }
    result.trim().to_string()
}

/// Fallback conversion: convert raw HTML to Markdown using htmd.
/// htmd handles script/style removal internally.
pub(super) fn fallback_convert(html: &str) -> String {
    htmd::convert(html).unwrap_or_default()
}

/// Truncate fetched web content via the shared `truncate_output` pipeline so
/// oversized pages spill to disk with a RipGrep pointer instead of a silent
/// char-based cut. `max_length` is the caller's requested ceiling; it's
/// clamped down to the shared `MAX_BYTES` so a single tool can never exceed
/// the uniform 2KB conversation-history budget.
pub(super) fn truncate_content(s: &str, max_length: usize) -> String {
    let cap = max_length.min(crate::truncation::MAX_BYTES);
    crate::truncation::truncate_output(s, crate::truncation::MAX_LINES, cap).content
}
