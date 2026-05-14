pub const CODE_BLOCK_PLACEHOLDER_PREFIX: &str = "\x00CB";
pub const INLINE_CODE_PLACEHOLDER_PREFIX: &str = "\x00IC";
pub const SLACK_ENTITY_PLACEHOLDER_PREFIX: &str = "\x00SE";
pub const BLOCKQUOTE_PLACEHOLDER_PREFIX: &str = "\x00BQ";

pub fn extract_code_blocks(text: &str) -> (String, Vec<String>) {
    extract_balanced(text, "```", CODE_BLOCK_PLACEHOLDER_PREFIX)
}

pub fn extract_inline_code(text: &str) -> (String, Vec<String>) {
    extract_balanced(text, "`", INLINE_CODE_PLACEHOLDER_PREFIX)
}

fn extract_balanced(text: &str, fence: &str, placeholder_prefix: &str) -> (String, Vec<String>) {
    let mut output = String::with_capacity(text.len());
    let mut blocks = Vec::new();
    let mut remaining = text;
    while let Some(start) = remaining.find(fence) {
        output.push_str(&remaining[..start]);
        let after_open = &remaining[start + fence.len()..];
        if let Some(end) = after_open.find(fence) {
            let block_end = start + fence.len() + end + fence.len();
            blocks.push(remaining[start..block_end].to_string());
            output.push_str(&format!("{}{}\x00", placeholder_prefix, blocks.len() - 1));
            remaining = &remaining[block_end..];
        } else {
            output.push_str(remaining);
            return (output, blocks);
        }
    }
    output.push_str(remaining);
    (output, blocks)
}

pub fn extract_slack_entities(text: &str) -> (String, Vec<String>) {
    let bytes = text.as_bytes();
    let mut output = String::with_capacity(text.len());
    let mut entities = Vec::new();
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'<' {
            if let Some(end_inclusive) = find_slack_entity_end(bytes, i) {
                let candidate = &text[i..=end_inclusive];
                if looks_like_slack_entity(candidate) {
                    entities.push(candidate.to_string());
                    output.push_str(&format!(
                        "{}{}\x00",
                        SLACK_ENTITY_PLACEHOLDER_PREFIX,
                        entities.len() - 1
                    ));
                    i = end_inclusive + 1;
                    continue;
                }
            }
        }
        let codepoint_end = next_codepoint_end(bytes, i);
        output.push_str(&text[i..codepoint_end]);
        i = codepoint_end;
    }
    (output, entities)
}

fn find_slack_entity_end(bytes: &[u8], open_index: usize) -> Option<usize> {
    let mut probe = open_index + 1;
    while probe < bytes.len() {
        match bytes[probe] {
            b'>' => return Some(probe),
            b'\n' => return None,
            _ => probe += 1,
        }
    }
    None
}

fn looks_like_slack_entity(entity: &str) -> bool {
    let inner = entity.trim_start_matches('<').trim_end_matches('>');
    inner.starts_with('@')
        || inner.starts_with('#')
        || inner.starts_with('!')
        || inner.starts_with("https://")
        || inner.starts_with("http://")
        || inner.starts_with("mailto:")
        || inner.starts_with("tel:")
}

fn next_codepoint_end(bytes: &[u8], start: usize) -> usize {
    let mut probe = start + 1;
    while probe < bytes.len() && (bytes[probe] & 0xC0) == 0x80 {
        probe += 1;
    }
    probe
}

pub fn extract_blockquote_prefixes(text: &str) -> (String, Vec<String>) {
    let mut output = String::with_capacity(text.len());
    let mut prefixes = Vec::new();
    for (line_index, line) in text.split('\n').enumerate() {
        if line_index > 0 {
            output.push('\n');
        }
        if let Some(prefix_end) = blockquote_prefix_end(line) {
            prefixes.push(line[..prefix_end].to_string());
            output.push_str(&format!(
                "{}{}\x00",
                BLOCKQUOTE_PLACEHOLDER_PREFIX,
                prefixes.len() - 1
            ));
            output.push_str(&line[prefix_end..]);
        } else {
            output.push_str(line);
        }
    }
    (output, prefixes)
}

fn blockquote_prefix_end(line: &str) -> Option<usize> {
    let trimmed_left = line.trim_start();
    let indent_size = line.len() - trimmed_left.len();
    let bytes = trimmed_left.as_bytes();
    let mut quote_count = 0;
    while quote_count < bytes.len() && bytes[quote_count] == b'>' {
        quote_count += 1;
    }
    if quote_count == 0 || quote_count >= bytes.len() {
        return None;
    }
    if bytes[quote_count] == b' ' || bytes[quote_count] == b'\t' {
        return Some(indent_size + quote_count + 1);
    }
    None
}

pub fn html_entity_escape(text: &str) -> String {
    let unescaped = text
        .replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">");
    unescaped
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
}
