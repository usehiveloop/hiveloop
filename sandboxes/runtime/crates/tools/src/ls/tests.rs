use super::*;
use std::fs;
use tempfile::tempdir;

#[test]
fn test_ls_description_is_rich() {
    let tool = LsTool::new();
    let desc = tool.description();
    assert!(!desc.is_empty());
    assert!(
        desc.contains("absolute"),
        "should mention absolute path requirement"
    );
    assert!(desc.contains("Glob"), "should mention cross-tool guidance");
    assert!(
        desc.contains("RipGrep"),
        "should mention cross-tool guidance"
    );
}

#[tokio::test]
async fn test_ls_basic_listing() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::write(dir_path.join("file_a.txt"), "a").expect("write");
    fs::write(dir_path.join("file_b.txt"), "b").expect("write");
    fs::create_dir(dir_path.join("subdir")).expect("mkdir");

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.total_entries >= 3);
    assert!(parsed.output.contains("file_a.txt"));
    assert!(parsed.output.contains("file_b.txt"));
    assert!(parsed.output.contains("subdir/"));
}

#[tokio::test]
async fn test_ls_empty_directory() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_entries, 0);
    assert!(parsed.output.is_empty());
}

#[tokio::test]
async fn test_ls_nonexistent_path() {
    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": "/tmp/nonexistent_ls_test_xyz"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("does not exist"));
}

#[tokio::test]
async fn test_ls_not_a_directory() {
    let dir = tempdir().expect("create temp dir");
    let file_path = dir.path().join("file.txt");
    fs::write(&file_path, "hello").expect("write");

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": file_path.to_str().unwrap()
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("Not a directory"));
}

#[tokio::test]
async fn test_ls_ignores_node_modules() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::write(dir_path.join("index.js"), "x").expect("write");
    fs::create_dir(dir_path.join("node_modules")).expect("mkdir");
    fs::write(dir_path.join("node_modules/dep.js"), "x").expect("write");

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.output.contains("index.js"));
    assert!(
        !parsed.output.contains("node_modules"),
        "node_modules should be excluded"
    );
}

#[tokio::test]
async fn test_ls_ignores_git_dir() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::write(dir_path.join("readme.txt"), "x").expect("write");
    fs::create_dir(dir_path.join(".git")).expect("mkdir");
    fs::write(dir_path.join(".git/config"), "x").expect("write");

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.output.contains("readme.txt"));
    assert!(!parsed.output.contains(".git"), ".git should be excluded");
}

#[tokio::test]
async fn test_ls_tree_format() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::create_dir(dir_path.join("src")).expect("mkdir");
    fs::write(dir_path.join("src/main.rs"), "fn main() {}").expect("write");
    fs::write(dir_path.join("Cargo.toml"), "[package]").expect("write");

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    // Tree format should have indented entries
    assert!(parsed.output.contains("src/"), "should show src directory");
    assert!(parsed.output.contains("main.rs"), "should show nested file");
}

#[tokio::test]
async fn test_ls_limit_100() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    // Create 110 files
    for i in 0..110 {
        fs::write(dir_path.join(format!("f{i:03}.txt")), "x").expect("write");
    }

    let tool = LsTool::new();
    let args = serde_json::json!({
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.truncated, "should be truncated at 100");
    assert!(parsed.total_entries <= MAX_ENTRIES);
}

#[tokio::test]
async fn test_ls_truncation_appends_steer() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();
    for i in 0..(MAX_ENTRIES + 5) {
        fs::write(dir_path.join(format!("f{i:04}.txt")), "x").expect("write");
    }

    let tool = LsTool::new();
    let args = serde_json::json!({ "path": dir_path.to_str().unwrap() });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert!(parsed.truncated);
    assert!(
        parsed.output.contains("Listing truncated"),
        "should include steer marker"
    );
    assert!(parsed.output.contains("Glob"), "should point at Glob");
    assert!(parsed.output.contains("RipGrep"), "should point at RipGrep");
    assert!(
        parsed.output.contains("self_agent"),
        "should point at self_agent"
    );
}

#[tokio::test]
async fn test_ls_no_steer_when_not_truncated() {
    let dir = tempdir().expect("create temp dir");
    fs::write(dir.path().join("one.txt"), "x").expect("write");

    let tool = LsTool::new();
    let args = serde_json::json!({ "path": dir.path().to_str().unwrap() });

    let result = tool.execute(args).await.expect("execute");
    let parsed: LsResult = serde_json::from_str(&result).expect("parse");

    assert!(!parsed.truncated);
    assert!(
        !parsed.output.contains("Listing truncated"),
        "steer should not be present for small dirs"
    );
}
