use super::persist::output_dir;
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
    // Create content well above MAX_BYTES so the byte cap kicks in.
    let big_line = "x".repeat(1000);
    let lines: Vec<&str> = std::iter::repeat_n(big_line.as_str(), 100).collect();
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
        result.content.contains("call the RipGrep tool"),
        "hint should point the agent at the RipGrep tool"
    );
}

#[test]
fn test_truncate_headtail_keeps_both_ends() {
    // Need input well above MAX_BYTES (2KB) for truncation to trigger.
    let lines: Vec<String> = (0..500)
        .map(|i| format!("line {i:04} padding_padding_padding"))
        .collect();
    let text = lines.join("\n");
    assert!(text.len() > MAX_BYTES, "test input must exceed cap");
    let result =
        truncate_output_directed(&text, MAX_LINES, MAX_BYTES, TruncationDirection::HeadTail);
    assert!(result.truncated, "expected truncation");
    // Output stays under the cap.
    assert!(
        result.content.len() <= MAX_BYTES + 512,
        "head+tail output ({} bytes) should be near the byte cap",
        result.content.len()
    );
    // Keeps an early line AND a late line with the marker in the middle.
    assert!(
        result.content.contains("line 0000"),
        "head section should survive"
    );
    assert!(
        result.content.contains("line 0499"),
        "tail section should survive"
    );
    let marker_pos = result
        .content
        .find("truncated.")
        .expect("marker should be present");
    let head_pos = result.content.find("line 0000").unwrap();
    let tail_pos = result.content.find("line 0499").unwrap();
    assert!(head_pos < marker_pos && marker_pos < tail_pos);
    // Spill pointer survives so the agent can recover the full content.
    assert!(result.content.contains("Full output saved to:"));
    assert!(result.content.contains("call the RipGrep tool"));
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
