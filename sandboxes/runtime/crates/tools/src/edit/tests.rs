use super::apply::{apply_edit, snippet};
use super::args::EditResult;
use super::tool::EditTool;
use std::io::Write as IoWrite;
use tempfile::NamedTempFile;

use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

#[test]
fn test_apply_edit_exact_match() {
    let content = "hello world\nfoo bar\nbaz qux\n";
    let (result, count) = apply_edit(content, "foo bar", "foo replaced", false).unwrap();
    assert!(result.contains("foo replaced"));
    assert!(!result.contains("foo bar"));
    assert_eq!(count, 1);
}

#[test]
fn test_apply_edit_not_found() {
    let content = "hello world\n";
    let err = apply_edit(content, "not here", "replacement", false).unwrap_err();
    assert!(err.contains("not found"));
}

#[test]
fn test_apply_edit_multiple_matches_no_replace_all() {
    // With the new strategy chain, multiple exact matches with
    // replace_all=false now picks the first occurrence via MultiOccurrenceReplacer
    let content = "aaa\nbbb\naaa\n";
    let (result, count) = apply_edit(content, "aaa", "ccc", false).unwrap();
    assert_eq!(count, 1);
    // First occurrence should be replaced
    assert!(result.starts_with("ccc\n"));
    // Second occurrence should remain
    assert!(result.contains("\naaa\n"));
}

#[test]
fn test_apply_edit_replace_all() {
    let content = "aaa\nbbb\naaa\n";
    let (result, count) = apply_edit(content, "aaa", "ccc", true).unwrap();
    assert_eq!(result.matches("ccc").count(), 2);
    assert_eq!(count, 2);
}

#[test]
fn test_apply_edit_identical_strings() {
    let content = "hello\n";
    let err = apply_edit(content, "hello", "hello", false).unwrap_err();
    assert!(err.contains("identical"));
}

#[tokio::test]
async fn test_edit_tool_execute() {
    let mut tmp = NamedTempFile::new().expect("create temp file");
    write!(tmp, "line one\nline two\nline three\n").expect("write");

    let tool = EditTool::new();
    let args = serde_json::json!({
        "filePath": tmp.path().to_str().unwrap(),
        "oldString": "line two",
        "newString": "line TWO"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: EditResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.replacements_made, 1);

    // Verify file was actually written
    let content = std::fs::read_to_string(tmp.path()).expect("read");
    assert!(content.contains("line TWO"));
    assert!(!content.contains("line two"));
}

#[tokio::test]
async fn test_edit_tool_not_found_file() {
    let tool = EditTool::new();
    let args = serde_json::json!({
        "filePath": "/tmp/nonexistent_edit_test_xyz.txt",
        "oldString": "foo",
        "newString": "bar"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("not found") || err.contains("Not found"));
}

#[tokio::test]
async fn test_edit_marks_written_after_success() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("tracked.txt");
    std::fs::write(&file_path, "aaa\nbbb\nccc\n").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    // Mark as read first
    tracker.mark_read(path_str);

    let tool = EditTool::new().with_file_tracker(tracker.clone());

    // First edit
    let args = serde_json::json!({
        "filePath": path_str,
        "oldString": "aaa",
        "newString": "AAA"
    });
    tool.execute(args).await.expect("first edit should succeed");

    // Second edit should work without re-reading (because mark_written was called)
    let args2 = serde_json::json!({
        "filePath": path_str,
        "oldString": "bbb",
        "newString": "BBB"
    });
    tool.execute(args2)
        .await
        .expect("second edit should succeed without re-reading");

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert!(content.contains("AAA"));
    assert!(content.contains("BBB"));
}

#[tokio::test]
async fn test_edit_rejects_stale_file() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("stale_edit.txt");
    std::fs::write(&file_path, "original content\n").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    tracker.mark_read(path_str);

    // Modify externally
    std::thread::sleep(std::time::Duration::from_millis(100));
    std::fs::write(&file_path, "modified externally\n").expect("write");

    let tool = EditTool::new().with_file_tracker(tracker);
    let args = serde_json::json!({
        "filePath": path_str,
        "oldString": "original content",
        "newString": "new content"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("has been modified"));
}

#[tokio::test]
async fn test_edit_empty_old_string_creates_file() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("new_created.txt");

    let tool = EditTool::new();
    let args = serde_json::json!({
        "filePath": file_path.to_str().unwrap(),
        "oldString": "",
        "newString": "new file content\nline two\n"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: EditResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.replacements_made, 1);
    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "new file content\nline two\n");
}

#[tokio::test]
async fn test_edit_empty_old_string_appends_existing() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("existing.txt");
    std::fs::write(&file_path, "original\n").expect("write");

    let tracker = FileTracker::new();
    tracker.mark_read(file_path.to_str().unwrap());

    let tool = EditTool::new().with_file_tracker(tracker);
    let args = serde_json::json!({
        "filePath": file_path.to_str().unwrap(),
        "oldString": "",
        "newString": "appended\n"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: EditResult = serde_json::from_str(&result).expect("parse");
    assert_eq!(parsed.replacements_made, 1);

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "original\nappended\n");
}

#[tokio::test]
async fn test_edit_crlf_normalization() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let file_path = dir.path().join("crlf.txt");
    // Write file with CRLF line endings
    std::fs::write(&file_path, "line one\r\nline two\r\nline three\r\n").expect("write");

    let tool = EditTool::new();
    let args = serde_json::json!({
        "filePath": file_path.to_str().unwrap(),
        "oldString": "line two",
        "newString": "line TWO"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: EditResult = serde_json::from_str(&result).expect("parse");
    assert_eq!(parsed.replacements_made, 1);

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert!(content.contains("line TWO"));
}

#[tokio::test]
async fn test_edit_line_counts_in_output() {
    let mut tmp = NamedTempFile::new().expect("create temp file");
    write!(tmp, "line1\nline2\nline3\n").expect("write");

    let tool = EditTool::new();
    let args = serde_json::json!({
        "filePath": tmp.path().to_str().unwrap(),
        "oldString": "line2",
        "newString": "line2a\nline2b\nline2c"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: EditResult = serde_json::from_str(&result).expect("parse");

    // Replaced 1 line with 3 lines = +2 lines added, 0 removed
    assert_eq!(parsed.lines_added, 2);
    assert_eq!(parsed.lines_removed, 0);
}

#[test]
fn test_snippet_multibyte_truncation() {
    // Each 'あ' is 3 bytes; 1000 of them = 3000 bytes.
    // Truncating at a non-char-boundary byte index would panic without floor_char_boundary.
    let s = "あ".repeat(1000);
    let result = snippet(&s, 100);
    assert!(result.ends_with("..."), "should be truncated with ...");
    // Should not panic — that's the main assertion
}
