use super::*;
use std::fs;
use std::time::Duration;
use tempfile::tempdir;

#[test]
fn test_mark_and_check() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("test.txt");
    fs::write(&file_path, "hello").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    assert!(!tracker.was_read(path_str));
    tracker.mark_read(path_str);
    assert!(tracker.was_read(path_str));
}

#[test]
fn test_require_read_fails_when_not_read() {
    let tracker = FileTracker::new();
    let result = tracker.require_read("/some/path/file.txt");
    assert!(result.is_err());
    assert!(result.unwrap_err().contains("must be read before"));
}

#[test]
fn test_require_read_succeeds_after_read() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("test.txt");
    fs::write(&file_path, "hello").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    tracker.mark_read(path_str);
    assert!(tracker.require_read(path_str).is_ok());
}

#[test]
fn test_tracker_is_shared_across_clones() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("shared.txt");
    fs::write(&file_path, "hello").expect("write");

    let tracker1 = FileTracker::new();
    let tracker2 = tracker1.clone();
    let path_str = file_path.to_str().unwrap();

    tracker1.mark_read(path_str);
    assert!(tracker2.was_read(path_str));
}

#[test]
fn test_staleness_detects_external_modification() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("stale.txt");
    fs::write(&file_path, "original").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    tracker.mark_read(path_str);

    // Wait to ensure mtime changes, then modify the file externally
    std::thread::sleep(Duration::from_millis(100));
    fs::write(&file_path, "modified externally").expect("write");

    let result = tracker.assert_not_stale(path_str);
    assert!(result.is_err(), "should detect external modification");
    assert!(result.unwrap_err().contains("has been modified"));
}

#[test]
fn test_staleness_passes_when_unmodified() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("stable.txt");
    fs::write(&file_path, "content").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    tracker.mark_read(path_str);
    let result = tracker.assert_not_stale(path_str);
    assert!(
        result.is_ok(),
        "unmodified file should pass staleness check"
    );
}

#[test]
fn test_mark_written_updates_timestamp() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("written.txt");
    fs::write(&file_path, "original").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    tracker.mark_read(path_str);

    // Simulate writing via tool (modifies file, then calls mark_written)
    std::thread::sleep(Duration::from_millis(100));
    fs::write(&file_path, "updated via tool").expect("write");
    tracker.mark_written(path_str);

    // Should pass because mark_written updated the recorded mtime
    let result = tracker.assert_not_stale(path_str);
    assert!(result.is_ok(), "should pass after mark_written");
}

#[test]
fn test_assert_not_stale_on_unread_file() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("never_read.txt");
    fs::write(&file_path, "content").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap();

    let result = tracker.assert_not_stale(path_str);
    assert!(result.is_err(), "never-read file should be rejected");
    assert!(result.unwrap_err().contains("must be read"));
}

#[tokio::test]
async fn test_file_lock_serializes_concurrent_edits() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("locked.txt");
    fs::write(&file_path, "line1\nline2\nline3\n").expect("write");

    let tracker = FileTracker::new();
    let path_str = file_path.to_str().unwrap().to_string();

    // Spawn two concurrent edit tasks that both modify the same file
    let tracker1 = tracker.clone();
    let path1 = path_str.clone();
    let file1 = file_path.clone();

    let tracker2 = tracker.clone();
    let path2 = path_str.clone();
    let file2 = file_path.clone();

    let (r1, r2) = tokio::join!(
        async move {
            tracker1
                .with_lock(&path1, || async {
                    let content = fs::read_to_string(&file1).unwrap();
                    // Small delay to ensure overlap would happen without locks
                    tokio::time::sleep(Duration::from_millis(50)).await;
                    let new_content = content.replace("line1", "LINE1");
                    fs::write(&file1, &new_content).unwrap();
                    "edit1 done"
                })
                .await
        },
        async move {
            tracker2
                .with_lock(&path2, || async {
                    let content = fs::read_to_string(&file2).unwrap();
                    tokio::time::sleep(Duration::from_millis(50)).await;
                    let new_content = content.replace("line3", "LINE3");
                    fs::write(&file2, &new_content).unwrap();
                    "edit2 done"
                })
                .await
        }
    );

    assert_eq!(r1, "edit1 done");
    assert_eq!(r2, "edit2 done");

    // Both edits should have been applied (serialized, not clobbered)
    let final_content = fs::read_to_string(&file_path).expect("read");
    assert!(
        final_content.contains("LINE1"),
        "first edit should be applied"
    );
    assert!(
        final_content.contains("LINE3"),
        "second edit should be applied"
    );
}

#[tokio::test]
async fn test_file_lock_different_files_run_concurrently() {
    let dir = tempdir().expect("create temp dir");
    let file_a = dir.path().join("a.txt");
    let file_b = dir.path().join("b.txt");
    fs::write(&file_a, "aaa").expect("write");
    fs::write(&file_b, "bbb").expect("write");

    let tracker = FileTracker::new();

    let tracker1 = tracker.clone();
    let path_a = file_a.to_str().unwrap().to_string();

    let tracker2 = tracker.clone();
    let path_b = file_b.to_str().unwrap().to_string();

    let start = std::time::Instant::now();

    let (_, _) = tokio::join!(
        async move {
            tracker1
                .with_lock(&path_a, || async {
                    tokio::time::sleep(Duration::from_millis(50)).await;
                })
                .await
        },
        async move {
            tracker2
                .with_lock(&path_b, || async {
                    tokio::time::sleep(Duration::from_millis(50)).await;
                })
                .await
        }
    );

    let elapsed = start.elapsed();
    // If they ran sequentially, it would take ~100ms. In parallel, ~50ms.
    assert!(
        elapsed < Duration::from_millis(90),
        "different files should not block each other (took {:?})",
        elapsed
    );
}
