use super::apply::apply_replacements;
use super::matcher::seek_sequence;
use super::parser::{parse_patch, strip_heredoc, Hunk};
use super::{ApplyPatchResult, ApplyPatchTool};
use crate::ToolExecutor;
use tempfile::tempdir;

#[test]
fn test_parse_patch_add_file() {
    let patch = r#"*** Begin Patch
*** Add File: hello.txt
+Hello world
+Second line
*** End Patch"#;

    let hunks = parse_patch(patch).expect("parse");
    assert_eq!(hunks.len(), 1);
    match &hunks[0] {
        Hunk::Add { path, contents } => {
            assert_eq!(path, "hello.txt");
            assert_eq!(contents, "Hello world\nSecond line");
        }
        _ => panic!("expected Add hunk"),
    }
}

#[test]
fn test_parse_patch_delete_file() {
    let patch = r#"*** Begin Patch
*** Delete File: old.txt
*** End Patch"#;

    let hunks = parse_patch(patch).expect("parse");
    assert_eq!(hunks.len(), 1);
    match &hunks[0] {
        Hunk::Delete { path } => assert_eq!(path, "old.txt"),
        _ => panic!("expected Delete hunk"),
    }
}

#[test]
fn test_parse_patch_update_file() {
    let patch = r#"*** Begin Patch
*** Update File: src/main.rs
@@ fn main() {
-    println!("old");
+    println!("new");
*** End Patch"#;

    let hunks = parse_patch(patch).expect("parse");
    assert_eq!(hunks.len(), 1);
    match &hunks[0] {
        Hunk::Update { path, chunks, .. } => {
            assert_eq!(path, "src/main.rs");
            assert_eq!(chunks.len(), 1);
            assert_eq!(chunks[0].old_lines, vec!["    println!(\"old\");"]);
            assert_eq!(chunks[0].new_lines, vec!["    println!(\"new\");"]);
        }
        _ => panic!("expected Update hunk"),
    }
}

#[test]
fn test_parse_patch_with_move() {
    let patch = r#"*** Begin Patch
*** Update File: old/path.rs
*** Move to: new/path.rs
@@ fn hello() {
-    println!("hi");
+    println!("hello");
*** End Patch"#;

    let hunks = parse_patch(patch).expect("parse");
    assert_eq!(hunks.len(), 1);
    match &hunks[0] {
        Hunk::Update {
            path, move_path, ..
        } => {
            assert_eq!(path, "old/path.rs");
            assert_eq!(move_path.as_deref(), Some("new/path.rs"));
        }
        _ => panic!("expected Update hunk"),
    }
}

#[test]
fn test_parse_patch_missing_markers() {
    let patch = "just some text";
    assert!(parse_patch(patch).is_err());
}

#[test]
fn test_seek_sequence_exact() {
    let lines: Vec<String> = vec![
        "line 1".to_string(),
        "line 2".to_string(),
        "line 3".to_string(),
    ];
    let pattern = vec!["line 2".to_string()];
    assert_eq!(seek_sequence(&lines, &pattern, 0, false), Some(1));
}

#[test]
fn test_seek_sequence_trimmed() {
    let lines: Vec<String> = vec!["  line 1  ".to_string(), "  line 2  ".to_string()];
    let pattern = vec!["line 2".to_string()];
    assert_eq!(seek_sequence(&lines, &pattern, 0, false), Some(1));
}

#[test]
fn test_seek_sequence_not_found() {
    let lines: Vec<String> = vec!["line 1".to_string()];
    let pattern = vec!["line 99".to_string()];
    assert_eq!(seek_sequence(&lines, &pattern, 0, false), None);
}

#[tokio::test]
async fn test_apply_patch_add_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("new_file.txt");

    let patch = format!(
        "*** Begin Patch\n*** Add File: {}\n+Hello\n+World\n*** End Patch",
        file_path.display()
    );

    let tool = ApplyPatchTool::new();
    let args = serde_json::json!({ "patchText": patch });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.files_changed, 1);
    assert!(parsed.summary[0].starts_with("A "));

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "Hello\nWorld");
}

#[tokio::test]
async fn test_apply_patch_delete_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("to_delete.txt");
    std::fs::write(&file_path, "content").expect("write");

    let patch = format!(
        "*** Begin Patch\n*** Delete File: {}\n*** End Patch",
        file_path.display()
    );

    let tool = ApplyPatchTool::new();
    let args = serde_json::json!({ "patchText": patch });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.files_changed, 1);
    assert!(!file_path.exists());
}

#[tokio::test]
async fn test_apply_patch_update_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("update_me.txt");
    std::fs::write(&file_path, "fn main() {\n    println!(\"old\");\n}\n").expect("write");

    let patch = format!(
        "*** Begin Patch\n*** Update File: {}\n@@ fn main() {{\n-    println!(\"old\");\n+    println!(\"new\");\n*** End Patch",
        file_path.display()
    );

    let tool = ApplyPatchTool::new();
    let args = serde_json::json!({ "patchText": patch });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.files_changed, 1);

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert!(content.contains("println!(\"new\")"));
    assert!(!content.contains("println!(\"old\")"));
}

#[test]
fn test_strip_heredoc() {
    let input = "cat <<'EOF'\nhello\nworld\nEOF";
    assert_eq!(strip_heredoc(input), "hello\nworld");
}

#[test]
fn test_strip_heredoc_no_match() {
    let input = "just regular text";
    assert_eq!(strip_heredoc(input), "just regular text");
}

#[test]
fn test_parse_patch_crlf_normalized() {
    // Patch with CRLF line endings should parse correctly
    let patch = "*** Begin Patch\r\n*** Add File: hello.txt\r\n+Hello world\r\n+Second line\r\n*** End Patch\r\n";

    let hunks = parse_patch(patch).expect("parse");
    assert_eq!(hunks.len(), 1);
    match &hunks[0] {
        Hunk::Add { path, contents } => {
            assert_eq!(path, "hello.txt");
            assert_eq!(contents, "Hello world\nSecond line");
        }
        _ => panic!("expected Add hunk"),
    }
}

#[tokio::test]
async fn test_apply_patch_crlf_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("crlf_file.txt");
    // Write file with CRLF line endings
    std::fs::write(&file_path, "fn main() {\r\n    println!(\"old\");\r\n}\r\n").expect("write");

    let patch = format!(
        "*** Begin Patch\n*** Update File: {}\n@@ fn main() {{\n-    println!(\"old\");\n+    println!(\"new\");\n*** End Patch",
        file_path.display()
    );

    let tool = ApplyPatchTool::new();
    let args = serde_json::json!({ "patchText": patch });

    let result = tool.execute(args).await.expect("execute");
    let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.files_changed, 1);

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert!(content.contains("println!(\"new\")"));
    assert!(!content.contains("println!(\"old\")"));
}

#[test]
fn test_apply_replacements_reverse_order() {
    let lines: Vec<String> = vec![
        "a".to_string(),
        "b".to_string(),
        "c".to_string(),
        "d".to_string(),
    ];
    let replacements = vec![
        (1, 1, vec!["B1".to_string(), "B2".to_string()]),
        (3, 1, vec!["D1".to_string()]),
    ];
    let result = apply_replacements(&lines, &replacements);
    assert_eq!(result, vec!["a", "B1", "B2", "c", "D1"]);
}
