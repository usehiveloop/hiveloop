use super::persist::persist_full_output;

/// Maximum number of lines before truncation.
pub const MAX_LINES: usize = 2000;
/// Maximum number of bytes before truncation (~2KB).
/// Aggressive cap to minimize input tokens; full output is persisted to disk
/// and reachable via the `RipGrep` tool.
pub const MAX_BYTES: usize = 2048;

/// Direction of truncation.
#[derive(Default, Clone, Copy)]
pub enum TruncationDirection {
    /// Keep the first N lines (default).
    #[default]
    Head,
    /// Keep the last N lines.
    Tail,
    /// Keep roughly half head and half tail with a truncation marker in the middle.
    HeadTail,
}

pub struct TruncationResult {
    pub content: String,
    pub truncated: bool,
    pub original_lines: usize,
    pub original_bytes: usize,
}

/// Truncate tool output to fit within line and byte limits.
pub fn truncate_output(text: &str, max_lines: usize, max_bytes: usize) -> TruncationResult {
    truncate_output_directed(text, max_lines, max_bytes, TruncationDirection::Head)
}

/// Truncate tool output with configurable direction.
pub fn truncate_output_directed(
    text: &str,
    max_lines: usize,
    max_bytes: usize,
    direction: TruncationDirection,
) -> TruncationResult {
    let lines: Vec<&str> = text.lines().collect();
    let total_bytes = text.len();
    let total_lines = lines.len();

    if total_lines <= max_lines && total_bytes <= max_bytes {
        return TruncationResult {
            content: text.to_string(),
            truncated: false,
            original_lines: total_lines,
            original_bytes: total_bytes,
        };
    }

    // Persist full output to disk
    let hint = if let Some(path) = persist_full_output(text) {
        format!(
            "Full output saved to: {}\nTo find specific content, call the RipGrep tool with path=\"{}\" and a regex pattern.",
            path, path
        )
    } else {
        "Full output was too large and could not be persisted to disk.".to_string()
    };

    match direction {
        TruncationDirection::Head => {
            let mut out = Vec::new();
            let mut bytes = 0;
            for line in &lines {
                let line_bytes = line.len() + 1;
                if out.len() >= max_lines || bytes + line_bytes > max_bytes {
                    break;
                }
                out.push(*line);
                bytes += line_bytes;
            }

            let removed_lines = total_lines - out.len();
            let removed_bytes = total_bytes - bytes;
            let mut result = out.join("\n");
            result.push_str(&format!(
                "\n\n... [{} lines, {} bytes truncated. {}] ...",
                removed_lines, removed_bytes, hint
            ));

            TruncationResult {
                content: result,
                truncated: true,
                original_lines: total_lines,
                original_bytes: total_bytes,
            }
        }
        TruncationDirection::Tail => {
            let mut out = Vec::new();
            let mut bytes = 0;
            for line in lines.iter().rev() {
                let line_bytes = line.len() + 1;
                if out.len() >= max_lines || bytes + line_bytes > max_bytes {
                    break;
                }
                out.push(*line);
                bytes += line_bytes;
            }
            out.reverse();

            let removed_lines = total_lines - out.len();
            let removed_bytes = total_bytes - bytes;
            let mut result = format!(
                "... [{} lines, {} bytes truncated. {}] ...\n\n",
                removed_lines, removed_bytes, hint
            );
            result.push_str(&out.join("\n"));

            TruncationResult {
                content: result,
                truncated: true,
                original_lines: total_lines,
                original_bytes: total_bytes,
            }
        }
        TruncationDirection::HeadTail => {
            // Reserve budget for the marker; split remainder ~50/50 between head and tail.
            let marker_overhead = 120usize.saturating_add(hint.len());
            let body_budget = max_bytes.saturating_sub(marker_overhead);
            let head_budget = body_budget / 2;
            let tail_budget = body_budget - head_budget;
            let head_lines_budget = max_lines.saturating_sub(1) / 2;
            let tail_lines_budget = max_lines.saturating_sub(1) - head_lines_budget;

            let mut head_out: Vec<&str> = Vec::new();
            let mut head_bytes = 0usize;
            for line in &lines {
                let line_bytes = line.len() + 1;
                if head_out.len() >= head_lines_budget || head_bytes + line_bytes > head_budget {
                    break;
                }
                head_out.push(*line);
                head_bytes += line_bytes;
            }

            let mut tail_out: Vec<&str> = Vec::new();
            let mut tail_bytes = 0usize;
            // Walk from the end, but never overlap the lines we already took for head.
            let tail_limit_idx = head_out.len();
            for (idx, line) in lines.iter().enumerate().rev() {
                if idx < tail_limit_idx {
                    break;
                }
                let line_bytes = line.len() + 1;
                if tail_out.len() >= tail_lines_budget || tail_bytes + line_bytes > tail_budget {
                    break;
                }
                tail_out.push(*line);
                tail_bytes += line_bytes;
            }
            tail_out.reverse();

            let kept_lines = head_out.len() + tail_out.len();
            let kept_bytes = head_bytes + tail_bytes;
            let removed_lines = total_lines.saturating_sub(kept_lines);
            let removed_bytes = total_bytes.saturating_sub(kept_bytes);

            let mut result = head_out.join("\n");
            if !result.is_empty() {
                result.push('\n');
            }
            result.push_str(&format!(
                "\n... [{} lines, {} bytes truncated. {}] ...\n\n",
                removed_lines, removed_bytes, hint
            ));
            result.push_str(&tail_out.join("\n"));

            TruncationResult {
                content: result,
                truncated: true,
                original_lines: total_lines,
                original_bytes: total_bytes,
            }
        }
    }
}
