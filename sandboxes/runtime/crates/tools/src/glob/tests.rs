use super::*;
use std::fs;
use tempfile::tempdir;

#[test]
fn test_glob_description_is_rich() {
    let tool = GlobTool::new();
    let desc = tool.description();
    assert!(!desc.is_empty());
    assert!(
        desc.contains("glob pattern"),
        "should mention glob patterns"
    );
    assert!(
        desc.contains("modification time"),
        "should mention sort order"
    );
    assert!(desc.contains("Agent"), "should mention cross-tool guidance");
}

#[tokio::test]
async fn test_glob_matches_files() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::write(dir_path.join("foo.rs"), "fn main() {}").expect("write");
    fs::write(dir_path.join("bar.rs"), "fn bar() {}").expect("write");
    fs::write(dir_path.join("baz.txt"), "hello").expect("write");

    let tool = GlobTool::new();
    let args = serde_json::json!({
        "pattern": "**/*.rs",
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: GlobResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_matches, 2);
    assert!(!parsed.truncated);

    let paths: Vec<&str> = parsed.files.iter().map(|f| f.path.as_str()).collect();
    assert!(paths.iter().any(|p| p.ends_with("foo.rs")));
    assert!(paths.iter().any(|p| p.ends_with("bar.rs")));
}

#[tokio::test]
async fn test_glob_no_matches() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::write(dir_path.join("test.txt"), "hello").expect("write");

    let tool = GlobTool::new();
    let args = serde_json::json!({
        "pattern": "**/*.rs",
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: GlobResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_matches, 0);
    assert!(parsed.files.is_empty());
}

#[tokio::test]
async fn test_glob_nested_directories() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    let sub = dir_path.join("src").join("lib");
    fs::create_dir_all(&sub).expect("mkdir");
    fs::write(sub.join("mod.rs"), "mod test;").expect("write");
    fs::write(dir_path.join("main.rs"), "fn main() {}").expect("write");

    let tool = GlobTool::new();
    let args = serde_json::json!({
        "pattern": "**/*.rs",
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: GlobResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_matches, 2);
}

#[tokio::test]
async fn test_glob_invalid_pattern() {
    let tool = GlobTool::new();
    let args = serde_json::json!({
        "pattern": "[invalid",
        "path": "/tmp"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("Invalid glob pattern"));
}

#[tokio::test]
async fn test_glob_nonexistent_path() {
    let tool = GlobTool::new();
    let args = serde_json::json!({
        "pattern": "**/*.rs",
        "path": "/tmp/nonexistent_glob_test_xyz"
    });

    let err = tool.execute(args).await.unwrap_err();
    assert!(err.contains("does not exist"));
}

#[tokio::test]
async fn test_glob_results_have_modification_time() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();

    fs::write(dir_path.join("test.rs"), "fn test() {}").expect("write");

    let tool = GlobTool::new();
    let args = serde_json::json!({
        "pattern": "**/*.rs",
        "path": dir_path.to_str().unwrap()
    });

    let result = tool.execute(args).await.expect("execute");
    let parsed: GlobResult = serde_json::from_str(&result).expect("parse");

    assert_eq!(parsed.total_matches, 1);
    assert!(parsed.files[0].modified.is_some());
    assert!(
        parsed.hint.is_none(),
        "hint should only appear when truncated"
    );
}

#[test]
fn test_glob_execute_populates_hint_when_truncated() {
    let dir = tempdir().expect("create temp dir");
    let dir_path = dir.path();
    for i in 0..(MAX_RESULTS + 5) {
        fs::write(dir_path.join(format!("f{i:05}.rs")), "x").expect("write");
    }

    let result = execute_glob("**/*.rs", dir_path.to_str().unwrap()).expect("glob should succeed");

    assert!(result.truncated, "should be truncated at MAX_RESULTS");
    let hint = result.hint.expect("hint should be present when truncated");
    assert!(
        hint.contains("narrow the pattern"),
        "hint should offer narrowing: {hint}"
    );
    assert!(
        hint.contains("RipGrep"),
        "hint should steer toward RipGrep: {hint}"
    );
    assert!(
        hint.contains("self_agent"),
        "hint should steer toward self_agent: {hint}"
    );
}
