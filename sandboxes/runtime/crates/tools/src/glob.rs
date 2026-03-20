use async_trait::async_trait;
use globset::GlobBuilder;
use ignore::WalkBuilder;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::time::SystemTime;

use crate::boundary::ProjectBoundary;
use crate::ToolExecutor;

/// Arguments for the Glob tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct GlobArgs {
    /// Glob pattern to match files. Example: '**/*.rs', 'src/**/*.ts'
    #[schemars(description = "Glob pattern to match files. Example: '**/*.rs', 'src/**/*.ts'")]
    pub pattern: String,
    /// The directory to search in. Defaults to the current working directory.
    #[schemars(description = "Directory to search in. Defaults to the current working directory")]
    pub path: Option<String>,
}

/// A single matched file entry.
#[derive(Debug, Serialize, Deserialize)]
pub struct GlobFileEntry {
    pub path: String,
    pub modified: Option<String>,
}

/// Result returned by the Glob tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct GlobResult {
    pub files: Vec<GlobFileEntry>,
    pub total_matches: usize,
    pub truncated: bool,
}

/// Maximum number of results to return.
const MAX_RESULTS: usize = 1000;

pub struct GlobTool {
    boundary: Option<ProjectBoundary>,
}

impl GlobTool {
    pub fn new() -> Self {
        Self { boundary: None }
    }

    pub fn with_boundary(mut self, boundary: ProjectBoundary) -> Self {
        self.boundary = Some(boundary);
        self
    }
}

impl Default for GlobTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for GlobTool {
    fn name(&self) -> &str {
        "Glob"
    }

    fn description(&self) -> &str {
        include_str!("instructions/glob.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(GlobArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: GlobArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let pattern = args.pattern.clone();
        let search_path = args.path.clone().unwrap_or_else(|| ".".to_string());

        // Check project boundary for the search path
        if let Some(ref boundary) = self.boundary {
            if search_path != "." {
                boundary.check(&search_path)?;
            }
        }

        let result = tokio::task::spawn_blocking(move || execute_glob(&pattern, &search_path))
            .await
            .map_err(|e| format!("Task join error: {e}"))??;

        let serialized = serde_json::to_string(&result)
            .map_err(|e| format!("Failed to serialize result: {e}"))?;

        // Apply shared truncation for large results
        let truncated = crate::truncation::truncate_output(
            &serialized,
            crate::truncation::MAX_LINES,
            crate::truncation::MAX_BYTES,
        );
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

fn execute_glob(pattern: &str, search_path: &str) -> Result<GlobResult, String> {
    let glob = GlobBuilder::new(pattern)
        .literal_separator(false)
        .build()
        .map_err(|e| format!("Invalid glob pattern: {e}"))?;
    let matcher = glob.compile_matcher();

    let root = PathBuf::from(search_path);
    if !root.exists() {
        return Err(format!("Path does not exist: {search_path}"));
    }

    let walker = WalkBuilder::new(&root)
        .hidden(false)
        .git_ignore(true)
        .build();

    let mut matched_files: Vec<(String, Option<SystemTime>)> = Vec::new();
    let mut truncated = false;

    for entry in walker {
        let entry = match entry {
            Ok(e) => e,
            Err(_) => continue,
        };

        // Only match files, not directories
        let path = entry.path();
        if !path.is_file() {
            continue;
        }

        // Match against the relative path from the root
        let relative = path.strip_prefix(&root).unwrap_or(path);

        if matcher.is_match(relative) {
            // Early termination once we have enough results
            if matched_files.len() >= MAX_RESULTS {
                truncated = true;
                break;
            }

            let abs_path = if path.is_absolute() {
                path.to_string_lossy().to_string()
            } else {
                path.canonicalize()
                    .map(|p| p.to_string_lossy().to_string())
                    .unwrap_or_else(|_| path.to_string_lossy().to_string())
            };

            let modified = path.metadata().ok().and_then(|m| m.modified().ok());

            matched_files.push((abs_path, modified));
        }
    }

    // Sort by modification time, newest first
    matched_files.sort_by(|a, b| {
        let time_a = a.1.unwrap_or(SystemTime::UNIX_EPOCH);
        let time_b = b.1.unwrap_or(SystemTime::UNIX_EPOCH);
        time_b.cmp(&time_a)
    });

    let total_matches = matched_files.len();

    let files: Vec<GlobFileEntry> = matched_files
        .into_iter()
        .take(MAX_RESULTS)
        .map(|(path, modified)| {
            let modified_str = modified
                .and_then(|t| t.duration_since(SystemTime::UNIX_EPOCH).ok())
                .map(|d| {
                    chrono::DateTime::from_timestamp(d.as_secs() as i64, d.subsec_nanos())
                        .map(|dt| dt.to_rfc3339())
                        .unwrap_or_default()
                });
            GlobFileEntry {
                path,
                modified: modified_str,
            }
        })
        .collect();

    Ok(GlobResult {
        files,
        total_matches,
        truncated,
    })
}

#[cfg(test)]
mod tests {
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
    }
}
