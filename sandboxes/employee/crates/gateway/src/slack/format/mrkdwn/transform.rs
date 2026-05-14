pub fn translate_headers(text: &str) -> String {
    let mut output = String::with_capacity(text.len());
    for (line_index, line) in text.lines().enumerate() {
        if line_index > 0 {
            output.push('\n');
        }
        if let Some(stripped) = strip_header_marker(line) {
            let cleaned = strip_inner_bold(stripped);
            output.push('*');
            output.push_str(cleaned.trim_end());
            output.push('*');
        } else {
            output.push_str(line);
        }
    }
    output
}

fn strip_header_marker(line: &str) -> Option<&str> {
    let trimmed = line.trim_start();
    let mut hash_count = 0;
    for ch in trimmed.chars() {
        if ch == '#' {
            hash_count += 1;
        } else {
            break;
        }
    }
    if hash_count == 0 || hash_count > 6 {
        return None;
    }
    let after = trimmed[hash_count..].strip_prefix(' ')?;
    Some(after)
}

fn strip_inner_bold(text: &str) -> String {
    text.replace("**", "")
}

pub fn translate_bold_italic_runs(text: &str) -> String {
    swap_paired_marker(text, "***", "*_", "_*")
}

pub fn translate_bold_runs(text: &str) -> String {
    swap_paired_marker(text, "**", "*", "*")
}

pub fn translate_strikethrough(text: &str) -> String {
    swap_paired_marker(text, "~~", "~", "~")
}

pub fn translate_italic_runs(text: &str) -> String {
    let mut output = String::with_capacity(text.len());
    let bytes = text.as_bytes();
    let mut cursor = 0usize;
    let mut scan = 0usize;
    while scan < bytes.len() {
        if bytes[scan] == b'*'
            && !is_double_marker(bytes, scan)
            && is_word_boundary_before(bytes, scan)
        {
            if let Some(close) = find_single_close(bytes, scan + 1, b'*') {
                output.push_str(&text[cursor..scan]);
                output.push('_');
                output.push_str(&text[scan + 1..close]);
                output.push('_');
                cursor = close + 1;
                scan = close + 1;
                continue;
            }
        }
        scan += 1;
    }
    output.push_str(&text[cursor..]);
    output
}

fn swap_paired_marker(text: &str, open: &str, close_open: &str, close_close: &str) -> String {
    let bytes = text.as_bytes();
    let marker_bytes = open.as_bytes();
    let marker_len = marker_bytes.len();
    let mut output = String::with_capacity(text.len());
    let mut cursor = 0usize;
    let mut scan = 0usize;
    while scan + marker_len <= bytes.len() {
        if bytes[scan..].starts_with(marker_bytes) {
            if let Some(close) = find_marker(bytes, scan + marker_len, marker_bytes) {
                output.push_str(&text[cursor..scan]);
                output.push_str(close_open);
                output.push_str(&text[scan + marker_len..close]);
                output.push_str(close_close);
                cursor = close + marker_len;
                scan = close + marker_len;
                continue;
            }
        }
        scan += 1;
    }
    output.push_str(&text[cursor..]);
    output
}

fn find_marker(bytes: &[u8], start: usize, marker: &[u8]) -> Option<usize> {
    let mut probe = start;
    while probe + marker.len() <= bytes.len() {
        if bytes[probe..].starts_with(marker) {
            return Some(probe);
        }
        probe += 1;
    }
    None
}

fn find_single_close(bytes: &[u8], start: usize, marker: u8) -> Option<usize> {
    let mut probe = start;
    while probe < bytes.len() {
        if bytes[probe] == marker && !is_double_marker(bytes, probe) {
            return Some(probe);
        }
        probe += 1;
    }
    None
}

fn is_double_marker(bytes: &[u8], position: usize) -> bool {
    (position + 1 < bytes.len() && bytes[position + 1] == bytes[position])
        || (position > 0 && bytes[position - 1] == bytes[position])
}

fn is_word_boundary_before(bytes: &[u8], position: usize) -> bool {
    if position == 0 {
        return true;
    }
    let prev = bytes[position - 1];
    !prev.is_ascii_alphanumeric()
}

pub fn translate_link_runs(text: &str) -> String {
    let bytes = text.as_bytes();
    let mut output = String::with_capacity(text.len());
    let mut cursor = 0usize;
    let mut scan = 0usize;
    while scan < bytes.len() {
        if bytes[scan] == b'[' && !preceded_by_image_marker(bytes, scan) {
            if let Some((label_end, url_start, url_end)) = parse_markdown_link(bytes, scan) {
                output.push_str(&text[cursor..scan]);
                output.push('<');
                output.push_str(strip_outer_brackets(&text[url_start..url_end]));
                output.push('|');
                output.push_str(&text[scan + 1..label_end]);
                output.push('>');
                cursor = url_end + 1;
                scan = url_end + 1;
                continue;
            }
        }
        scan += 1;
    }
    output.push_str(&text[cursor..]);
    output
}

fn preceded_by_image_marker(bytes: &[u8], position: usize) -> bool {
    position > 0 && bytes[position - 1] == b'!'
}

fn strip_outer_brackets(url: &str) -> &str {
    if url.starts_with('<') && url.ends_with('>') && url.len() >= 2 {
        &url[1..url.len() - 1]
    } else {
        url
    }
}

fn parse_markdown_link(bytes: &[u8], start: usize) -> Option<(usize, usize, usize)> {
    let mut probe = start + 1;
    while probe < bytes.len() && bytes[probe] != b']' {
        probe += 1;
    }
    if probe >= bytes.len() || probe + 1 >= bytes.len() || bytes[probe + 1] != b'(' {
        return None;
    }
    let label_end = probe;
    let url_start = probe + 2;
    let mut url_probe = url_start;
    let mut paren_depth = 0u32;
    while url_probe < bytes.len() {
        match bytes[url_probe] {
            b'(' => paren_depth += 1,
            b')' => {
                if paren_depth == 0 {
                    return Some((label_end, url_start, url_probe));
                }
                paren_depth -= 1;
            }
            _ => {}
        }
        url_probe += 1;
    }
    None
}
