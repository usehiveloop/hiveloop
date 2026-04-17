use async_trait::async_trait;
use ignore::WalkBuilder;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;

use crate::ToolExecutor;

/// Arguments for the LS tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct LsArgs {
    /// The absolute path of the directory to list.
    pub path: String,
}

/// Result returned by the LS tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct LsResult {
    pub output: String,
    pub total_entries: usize,
    pub truncated: bool,
}

/// Maximum number of entries to return.
const MAX_ENTRIES: usize = 100;

/// Default directories/files to ignore.
const DEFAULT_IGNORE_PATTERNS: &[&str] = &[
    "node_modules",
    "__pycache__",
    ".git",
    "dist",
    "build",
    "target",
    "vendor",
    "bin",
    "obj",
    ".idea",
    ".vscode",
    ".zig-cache",
    "zig-out",
    ".coverage",
    "coverage",
    "tmp",
    "temp",
    ".cache",
    "cache",
    "logs",
    ".venv",
    "venv",
    "env",
];

pub struct LsTool;

impl LsTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for LsTool {
    fn default() -> Self {
        Self::new()
    }
}

/// Render directory tree using ignore::WalkBuilder (respects .gitignore).
fn render_tree(
    root: &Path,
    ignore_patterns: &[&str],
    limit: usize,
) -> Result<(String, usize, bool), String> {
    let walker = WalkBuilder::new(root)
        .hidden(false)
        .git_ignore(true)
        .build();

    let mut files: Vec<String> = Vec::new();
    for entry in walker {
        let entry = match entry {
            Ok(e) => e,
            Err(_) => continue,
        };
        let path = entry.path();
        let relative = path.strip_prefix(root).unwrap_or(path);
        let name = relative.to_string_lossy().to_string();
        if name.is_empty() {
            continue;
        }

        // Check ignore patterns on any path component
        let should_ignore = relative.components().any(|c| {
            let s = c.as_os_str().to_string_lossy();
            ignore_patterns.contains(&s.as_ref())
        });
        if should_ignore {
            continue;
        }

        if path.is_dir() {
            files.push(format!("{}/", name));
        } else {
            files.push(name);
        }

        if files.len() >= limit {
            break;
        }
    }

    let truncated = files.len() >= limit;
    let total = files.len();

    // Build tree-like output with 2-space indentation
    let mut output = String::new();
    for f in &files {
        let depth = f
            .matches('/')
            .count()
            .saturating_sub(if f.ends_with('/') { 1 } else { 0 });
        let indent = "  ".repeat(depth);
        let basename = f.rsplit('/').find(|s| !s.is_empty()).unwrap_or(f);
        let suffix = if f.ends_with('/') { "/" } else { "" };
        output.push_str(&format!("{}{}{}\n", indent, basename, suffix));
    }

    Ok((output, total, truncated))
}

#[async_trait]
impl ToolExecutor for LsTool {
    fn name(&self) -> &str {
        "LS"
    }

    fn description(&self) -> &str {
        include_str!("instructions/ls.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(LsArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: LsArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let dir_path = &args.path;
        let path = Path::new(dir_path);

        if !path.exists() {
            return Err(format!("Path does not exist: {dir_path}"));
        }

        if !path.is_dir() {
            return Err(format!("Not a directory: {dir_path}"));
        }

        let root = path.to_path_buf();
        let (output, total_entries, truncated) = tokio::task::spawn_blocking(move || {
            render_tree(&root, DEFAULT_IGNORE_PATTERNS, MAX_ENTRIES)
        })
        .await
        .map_err(|e| format!("Task join error: {e}"))??;

        let result = LsResult {
            output,
            total_entries,
            truncated,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[cfg(test)]
mod tests {
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
        assert!(desc.contains("RipGrep"), "should mention cross-tool guidance");
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
}
