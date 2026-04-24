use super::*;
use tempfile::tempdir;

#[tokio::test]
async fn test_write_new_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("new_file.txt");

    let tool = WriteTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap(),
        "content": "hello world"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.created);
    assert_eq!(parsed.bytes_written, 11);
    assert!(parsed.diff.is_none()); // New file, no diff

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "hello world");
}

#[tokio::test]
async fn test_write_overwrite_existing() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("existing.txt");
    std::fs::write(&file_path, "old content").expect("write");

    let tool = WriteTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap(),
        "content": "new content"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

    assert!(!parsed.created);
    assert_eq!(parsed.bytes_written, 11);

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "new content");
}

#[tokio::test]
async fn test_write_creates_parent_dirs() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("a").join("b").join("c").join("deep.txt");

    let tool = WriteTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap(),
        "content": "deep content"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.created);
    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "deep content");
}

#[tokio::test]
async fn test_write_requires_absolute_path() {
    let tool = WriteTool::new();
    let args = serde_json::json!({
        "file_path": "relative/path.txt",
        "content": "content"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("absolute path"));
}

#[tokio::test]
async fn test_write_marks_written_after_success() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("write_tracked.txt");
    std::fs::write(&file_path, "original").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    // Mark as read first
    tracker.mark_read(path_str);

    let tool = WriteTool::new().with_file_tracker(tracker.clone());

    // First write
    let args = serde_json::json!({
        "file_path": path_str,
        "content": "first write"
    });
    tool.execute(args)
        .await
        .expect("first write should succeed");

    // Second write should work without re-reading (because mark_written was called)
    let args2 = serde_json::json!({
        "file_path": path_str,
        "content": "second write"
    });
    tool.execute(args2)
        .await
        .expect("second write should succeed without re-reading");

    let content = std::fs::read_to_string(&file_path).expect("read");
    assert_eq!(content, "second write");
}

#[tokio::test]
async fn test_write_rejects_stale_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("stale_write.txt");
    std::fs::write(&file_path, "original").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    tracker.mark_read(path_str);

    // Modify externally
    std::thread::sleep(std::time::Duration::from_millis(100));
    std::fs::write(&file_path, "modified externally").expect("write");

    let tool = WriteTool::new().with_file_tracker(tracker);
    let args = serde_json::json!({
        "file_path": path_str,
        "content": "new content"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("has been modified"));
}

#[tokio::test]
async fn test_write_includes_diff() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("diff_test.txt");
    std::fs::write(&file_path, "line one\nline two\n").expect("write");

    let tool = WriteTool::new();
    let args = serde_json::json!({
        "file_path": file_path.to_str().unwrap(),
        "content": "line one\nline TWO\n"
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.diff.is_some());
    let diff = parsed.diff.unwrap();
    assert!(diff.contains("-line two"));
    assert!(diff.contains("+line TWO"));
}

#[tokio::test]
async fn test_write_file_locking() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("locked_write.txt");
    std::fs::write(&file_path, "original").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap().to_string();

    tracker.mark_read(&path_str);

    let tool = Arc::new(WriteTool::new().with_file_tracker(tracker.clone()));

    // Run two concurrent writes — they should serialize via lock
    let tool1 = tool.clone();
    let path1 = path_str.clone();
    let h1 = tokio::spawn(async move {
        tool1
            .execute(serde_json::json!({
                "file_path": path1,
                "content": "write A"
            }))
            .await
    });

    let tool2 = tool.clone();
    let path2 = path_str.clone();
    let h2 = tokio::spawn(async move {
        tool2
            .execute(serde_json::json!({
                "file_path": path2,
                "content": "write B"
            }))
            .await
    });

    let (r1, r2) = tokio::join!(h1, h2);
    // Both should succeed (serialized by lock)
    assert!(r1.unwrap().is_ok());
    assert!(r2.unwrap().is_ok());
}
