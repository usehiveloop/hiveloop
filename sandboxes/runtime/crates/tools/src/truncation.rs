use std::path::PathBuf;

/// Maximum number of lines before truncation.
pub const MAX_LINES: usize = 2000;
/// Maximum number of bytes before truncation.
pub const MAX_BYTES: usize = 50 * 1024; // 50KB

/// Direction of truncation.
#[derive(Default, Clone, Copy)]
pub enum TruncationDirection {
    /// Keep the first N lines (default).
    #[default]
    Head,
    /// Keep the last N lines.
    Tail,
}

pub struct TruncationResult {
    pub content: String,
    pub truncated: bool,
    pub original_lines: usize,
    pub original_bytes: usize,
}

/// Directory for persisted tool output files.
fn output_dir() -> PathBuf {
    let dir = std::env::temp_dir().join("bridge_tool_output");
    let _ = std::fs::create_dir_all(&dir);
    dir
}

/// Persist full text to disk and return the file path.
fn persist_full_output(text: &str) -> Option<String> {
    let path = output_dir().join(format!("{}.txt", uuid::Uuid::new_v4()));
    std::fs::write(&path, text).ok()?;
    Some(path.to_string_lossy().to_string())
}

/// Clean up output files older than 7 days.
pub fn cleanup_old_outputs() {
    let retention = std::time::Duration::from_secs(7 * 24 * 60 * 60);
    let cutoff = std::time::SystemTime::now() - retention;
    if let Ok(entries) = std::fs::read_dir(output_dir()) {
        for entry in entries.flatten() {
            if let Ok(metadata) = entry.metadata() {
                if let Ok(modified) = metadata.modified() {
                    if modified < cutoff {
                        let _ = std::fs::remove_file(entry.path());
                    }
                }
            }
        }
    }
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
            "Full output saved to: {}\nUse Grep to search or Read with offset/limit to view specific sections.",
            path
        )
    } else {
        "Use grep/read with offset to see full content".to_string()
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
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_truncate_within_limits() {
        let text = "line1\nline2\nline3\n";
        let result = truncate_output(text, MAX_LINES, MAX_BYTES);
        assert!(!result.truncated);
        assert_eq!(result.content, text);
        assert_eq!(result.original_lines, 3);
    }

    #[test]
    fn test_truncate_exceeds_line_limit() {
        let lines: Vec<String> = (0..3000).map(|i| format!("line {i}")).collect();
        let text = lines.join("\n");
        let result = truncate_output(&text, 2000, MAX_BYTES);
        assert!(result.truncated);
        assert_eq!(result.original_lines, 3000);
        // The truncated content should have at most 2000 actual lines + the notice
        let output_lines: Vec<&str> = result.content.lines().collect();
        // 2000 lines + 1 empty line + 1 notice line
        assert!(output_lines.len() <= 2003);
    }

    #[test]
    fn test_truncate_exceeds_byte_limit() {
        // Create content that exceeds 50KB
        let big_line = "x".repeat(1000);
        let lines: Vec<&str> = std::iter::repeat(big_line.as_str()).take(100).collect();
        let text = lines.join("\n"); // ~100KB
        let result = truncate_output(&text, MAX_LINES, MAX_BYTES);
        assert!(result.truncated);
        assert!(result.original_bytes > MAX_BYTES);
    }

    #[test]
    fn test_truncate_notice_included() {
        let lines: Vec<String> = (0..3000).map(|i| format!("line {i}")).collect();
        let text = lines.join("\n");
        let result = truncate_output(&text, 2000, MAX_BYTES);
        assert!(result.truncated);
        assert!(result.content.contains("truncated."));
    }

    #[test]
    fn test_truncate_persists_to_disk() {
        let lines: Vec<String> = (0..3000).map(|i| format!("line {i}")).collect();
        let text = lines.join("\n");
        let result = truncate_output(&text, 2000, MAX_BYTES);
        assert!(result.truncated);
        // Should contain path to persisted file
        assert!(
            result.content.contains("Full output saved to:"),
            "hint should include file path"
        );
        // Extract the path and verify it exists
        if let Some(start) = result.content.find("Full output saved to: ") {
            let after = &result.content[start + "Full output saved to: ".len()..];
            if let Some(end) = after.find('\n') {
                let path = &after[..end];
                assert!(
                    std::path::Path::new(path).exists(),
                    "persisted file should exist at: {}",
                    path
                );
                // Clean up
                let _ = std::fs::remove_file(path);
            }
        }
    }

    #[test]
    fn test_truncate_tail_direction() {
        let lines: Vec<String> = (0..100).map(|i| format!("line {i}")).collect();
        let text = lines.join("\n");
        let result = truncate_output_directed(&text, 10, MAX_BYTES, TruncationDirection::Tail);
        assert!(result.truncated);
        // Should contain the last lines
        assert!(result.content.contains("line 99"));
        assert!(result.content.contains("line 90"));
        // Should NOT contain early lines
        assert!(!result.content.contains("line 0\n"));
    }

    #[test]
    fn test_truncate_hint_includes_path() {
        let lines: Vec<String> = (0..3000).map(|i| format!("line {i}")).collect();
        let text = lines.join("\n");
        let result = truncate_output(&text, 2000, MAX_BYTES);
        assert!(result.truncated);
        assert!(
            result.content.contains("Full output saved to:"),
            "hint should include file path"
        );
        assert!(
            result.content.contains("Use Grep to search or Read"),
            "hint should suggest tools"
        );
    }

    #[test]
    fn test_cleanup_old_outputs() {
        // Just verify cleanup_old_outputs() doesn't panic.
        // Recent files should survive cleanup.
        let dir = output_dir();
        let recent_file = dir.join("test_cleanup_recent.txt");
        std::fs::write(&recent_file, "recent").expect("write");

        cleanup_old_outputs();

        assert!(recent_file.exists(), "recent file should be kept");

        // Clean up
        let _ = std::fs::remove_file(&recent_file);
    }
}
