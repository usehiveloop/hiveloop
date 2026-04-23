//! Basic read tool tests: simple files, pagination, directories, and lookup errors.

use crate::read::*;
use crate::ToolExecutor;
use std::io::Write;
use tempfile::NamedTempFile;

#[test]
fn test_read_description_is_rich() {
    let tool = ReadTool::new();
    let desc = tool.description();
    assert!(!desc.is_empty());
    assert!(
        desc.contains("absolute path"),
        "should mention absolute path requirement"
    );
    assert!(desc.contains("2000"), "should mention default line limit");
    assert!(desc.contains("image"), "should mention image support");
    assert!(
        desc.contains("RipGrep"),
        "should mention cross-tool guidance"
    );
    assert!(
        desc.contains("self_agent"),
        "should mention self_agent steer for large files"
    );
}

#[tokio::test]
async fn test_read_simple_file() {
    let mut tmp = NamedTempFile::new().expect("create temp file");
    writeln!(tmp, "line one").expect("write");
    writeln!(tmp, "line two").expect("write");
    writeln!(tmp, "line three").expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": tmp.path().to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_lines, 3);
    assert_eq!(parsed.lines_read, 3);
    assert!(!parsed.truncated);
    assert!(parsed.content.contains("line one"));
    assert!(parsed.content.contains("line two"));
    assert!(parsed.content.contains("line three"));
}

#[tokio::test]
async fn test_read_with_offset_and_limit() {
    let mut tmp = NamedTempFile::new().expect("create temp file");
    for i in 1..=10 {
        writeln!(tmp, "line {i}").expect("write");
    }

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": tmp.path().to_str().unwrap(),
        "offset": 3,
        "limit": 2
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_lines, 10);
    assert_eq!(parsed.lines_read, 2);
    assert!(parsed.truncated);
    assert!(parsed.content.contains("line 3"));
    assert!(parsed.content.contains("line 4"));
    assert!(!parsed.content.contains("line 2"));
    assert!(!parsed.content.contains("line 5"));
}

#[tokio::test]
async fn test_read_relative_path_error() {
    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": "relative/path.txt"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("absolute path"));
}

#[tokio::test]
async fn test_read_directory_lists_entries() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let dir_path = dir.path();

    // Create files and a subdirectory
    std::fs::write(dir_path.join("file_a.txt"), "a").expect("write");
    std::fs::write(dir_path.join("file_b.txt"), "b").expect("write");
    std::fs::create_dir(dir_path.join("subdir")).expect("mkdir");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_lines, 3);
    assert!(parsed.content.contains("file_a.txt"));
    assert!(parsed.content.contains("file_b.txt"));
    assert!(parsed.content.contains("subdir/"));
}

#[tokio::test]
async fn test_read_directory_pagination() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let dir_path = dir.path();

    for i in 0..10 {
        std::fs::write(dir_path.join(format!("file_{:02}.txt", i)), "x").expect("write");
    }

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": dir_path.to_str().unwrap(),
        "offset": 3,
        "limit": 2
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_lines, 10);
    assert_eq!(parsed.lines_read, 2);
    assert!(parsed.truncated);
}

#[tokio::test]
async fn test_read_not_found_error() {
    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": "/tmp/nonexistent_file_read_test_xyz.txt"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("not found"));
}

#[tokio::test]
async fn test_read_not_found_suggests_similar_files() {
    let dir = tempfile::tempdir().expect("create temp dir");
    let existing = dir.path().join("foo.rs");
    std::fs::write(&existing, "content").expect("write");

    let tool = ReadTool::new();
    // Try to read "fo.rs" which is similar to "foo.rs"
    let args = serde_json::json!({
        "file_path": dir.path().join("fo.rs").to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(
        err.contains("Did you mean"),
        "should suggest similar files: {err}"
    );
    assert!(err.contains("foo.rs"), "should suggest foo.rs: {err}");
}

#[tokio::test]
async fn test_read_not_found_no_suggestions_when_dir_empty() {
    let dir = tempfile::tempdir().expect("create temp dir");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": dir.path().join("nonexistent.txt").to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("not found"));
    assert!(
        !err.contains("Did you mean"),
        "should not suggest when dir is empty"
    );
}

#[tokio::test]
async fn test_read_not_found_no_suggestions_when_parent_missing() {
    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": "/tmp/nonexistent_parent_dir_xyz/file.txt"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("not found"));
    assert!(
        !err.contains("Did you mean"),
        "should not suggest when parent is missing"
    );
}

#[tokio::test]
async fn test_read_line_truncation() {
    let mut tmp = NamedTempFile::new().expect("create temp file");
    let long_line = "x".repeat(3000);
    writeln!(tmp, "{long_line}").expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": tmp.path().to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

    // The content should have the truncated line ending with "..."
    assert!(parsed.content.contains("..."));
    // The displayed line should be at most MAX_LINE_LENGTH + "..." = 2003 chars (plus line num prefix)
}

#[tokio::test]
async fn test_read_line_number_formatting() {
    let mut tmp = NamedTempFile::new().expect("create temp file");
    writeln!(tmp, "first").expect("write");
    writeln!(tmp, "second").expect("write");

    let tool = ReadTool::new();
    let args = serde_json::json!({
        "file_path": tmp.path().to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

    // Lines should be formatted as "N: content"
    assert!(parsed.content.contains("1: first"));
    assert!(parsed.content.contains("2: second"));
}
