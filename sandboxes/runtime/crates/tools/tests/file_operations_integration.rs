use std::path::PathBuf;

use tools::{LocalFsOperations, ReadOperations, WriteOperations};

fn fs() -> LocalFsOperations {
    LocalFsOperations
}

fn tmp(path: &str) -> PathBuf {
    std::env::temp_dir().join(path)
}

#[tokio::test]
async fn developer_reads_existing_file() {
    let content = "Hello, this is a test file.\nLine 2.";
    let path = tmp("read-test-1.txt");
    fs().write_file(&path, content.as_bytes()).await.unwrap();
    let result = fs().read_file(&path).await.unwrap();
    assert_eq!(String::from_utf8_lossy(&result), content);
    std::fs::remove_file(&path).ok();
}

#[tokio::test]
async fn developer_writes_and_then_reads_a_config_file() {
    let path = tmp("config-test.json");
    let json = "{\"key\": \"value\", \"enabled\": true}";
    fs().write_file(&path, json.as_bytes()).await.unwrap();
    let read_back = fs().read_file(&path).await.unwrap();
    assert_eq!(String::from_utf8_lossy(&read_back), json);
    std::fs::remove_file(&path).ok();
}

#[tokio::test]
async fn reading_nonexistent_file_returns_error() {
    let path = tmp("does-not-exist-xyz.txt");
    let result = fs().read_file(&path).await;
    assert!(result.is_err());
}

#[tokio::test]
async fn overwriting_existing_file_replaces_content() {
    let path = tmp("overwrite-test.txt");
    fs().write_file(&path, b"original content").await.unwrap();
    fs().write_file(&path, b"updated content").await.unwrap();
    let result = fs().read_file(&path).await.unwrap();
    assert_eq!(String::from_utf8_lossy(&result), "updated content");
    std::fs::remove_file(&path).ok();
}

#[tokio::test]
async fn writing_to_existing_directory_succeeds() {
    let dir = tmp("writable-dir");
    std::fs::create_dir_all(&dir).unwrap();
    let path = dir.join("file.txt");
    fs().write_file(&path, b"content in subdirectory")
        .await
        .unwrap();
    let result = fs().read_file(&path).await.unwrap();
    assert_eq!(String::from_utf8_lossy(&result), "content in subdirectory");
    let _ = std::fs::remove_dir_all(&dir);
}

#[tokio::test]
async fn reading_large_file_returns_full_content() {
    let path = tmp("large-file.txt");
    let content = "x".repeat(10000);
    fs().write_file(&path, content.as_bytes()).await.unwrap();
    let result = fs().read_file(&path).await.unwrap();
    assert_eq!(result.len(), 10000);
    std::fs::remove_file(&path).ok();
}
