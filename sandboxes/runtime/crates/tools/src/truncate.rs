pub const DEFAULT_MAX_LINES: usize = 2000;
pub const DEFAULT_MAX_BYTES: usize = 50 * 1024;

pub struct TruncationResult {
    pub content: String,
    pub truncated: bool,
    pub truncated_by: TruncationReason,
    pub total_lines: usize,
    pub total_bytes: usize,
    pub output_lines: usize,
    pub output_bytes: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TruncationReason {
    NotTruncated,
    Lines,
    Bytes,
}

pub fn truncate_head(content: &str, max_lines: usize, max_bytes: usize) -> TruncationResult {
    let total_lines = count_lines(content);
    let total_bytes = content.len();

    if total_lines <= max_lines && total_bytes <= max_bytes {
        return TruncationResult {
            content: content.to_string(),
            truncated: false,
            truncated_by: TruncationReason::NotTruncated,
            total_lines,
            total_bytes,
            output_lines: total_lines,
            output_bytes: total_bytes,
        };
    }

    let mut output = String::with_capacity(max_bytes.min(total_bytes));
    let mut lines_taken = 0usize;
    let mut bytes_taken = 0usize;
    let mut hit_byte_limit = false;
    for line in content.split_inclusive('\n') {
        if lines_taken >= max_lines {
            break;
        }
        if bytes_taken + line.len() > max_bytes {
            hit_byte_limit = true;
            break;
        }
        output.push_str(line);
        bytes_taken += line.len();
        lines_taken += 1;
    }
    let reason = if hit_byte_limit {
        TruncationReason::Bytes
    } else {
        TruncationReason::Lines
    };
    TruncationResult {
        content: output,
        truncated: true,
        truncated_by: reason,
        total_lines,
        total_bytes,
        output_lines: lines_taken,
        output_bytes: bytes_taken,
    }
}

pub fn truncate_tail(content: &str, max_lines: usize, max_bytes: usize) -> TruncationResult {
    let total_lines = count_lines(content);
    let total_bytes = content.len();

    if total_lines <= max_lines && total_bytes <= max_bytes {
        return TruncationResult {
            content: content.to_string(),
            truncated: false,
            truncated_by: TruncationReason::NotTruncated,
            total_lines,
            total_bytes,
            output_lines: total_lines,
            output_bytes: total_bytes,
        };
    }

    let lines: Vec<&str> = content.split_inclusive('\n').collect();
    let mut start_index = lines.len().saturating_sub(max_lines);
    let mut bytes_taken: usize = lines[start_index..].iter().map(|line| line.len()).sum();
    let mut hit_byte_limit = false;
    while bytes_taken > max_bytes && start_index < lines.len() {
        bytes_taken -= lines[start_index].len();
        start_index += 1;
        hit_byte_limit = true;
    }
    let mut output = String::with_capacity(bytes_taken);
    for line in &lines[start_index..] {
        output.push_str(line);
    }
    let reason = if hit_byte_limit {
        TruncationReason::Bytes
    } else {
        TruncationReason::Lines
    };
    TruncationResult {
        content: output,
        truncated: true,
        truncated_by: reason,
        total_lines,
        total_bytes,
        output_lines: lines.len() - start_index,
        output_bytes: bytes_taken,
    }
}

fn count_lines(content: &str) -> usize {
    if content.is_empty() {
        return 0;
    }
    let mut count = content.matches('\n').count();
    if !content.ends_with('\n') {
        count += 1;
    }
    count
}
