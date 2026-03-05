use async_trait::async_trait;
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::sync::Arc;

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

/// Arguments for the Write tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct WriteArgs {
    /// Absolute path to the file to write. Parent directories are created automatically.
    #[schemars(
        description = "Absolute path to the file to write. Parent directories are created automatically"
    )]
    pub file_path: String,
    /// The full content to write to the file. Overwrites existing content.
    #[schemars(description = "The full content to write to the file. Overwrites existing content")]
    pub content: String,
}

/// Result returned by the Write tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct WriteResult {
    pub path: String,
    pub bytes_written: usize,
    pub created: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diff: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
}

pub struct WriteTool {
    file_tracker: Option<FileTracker>,
    boundary: Option<ProjectBoundary>,
    lsp_manager: Option<Arc<LspManager>>,
}

impl WriteTool {
    pub fn new() -> Self {
        Self {
            file_tracker: None,
            boundary: None,
            lsp_manager: None,
        }
    }

    pub fn with_file_tracker(mut self, tracker: FileTracker) -> Self {
        self.file_tracker = Some(tracker);
        self
    }

    pub fn with_boundary(mut self, boundary: ProjectBoundary) -> Self {
        self.boundary = Some(boundary);
        self
    }

    pub fn with_lsp_manager(mut self, m: Arc<LspManager>) -> Self {
        self.lsp_manager = Some(m);
        self
    }

    pub fn with_lsp_manager_opt(mut self, m: Option<Arc<LspManager>>) -> Self {
        self.lsp_manager = m;
        self
    }
}

impl Default for WriteTool {
    fn default() -> Self {
        Self::new()
    }
}

/// Core write logic extracted so it can be called from within with_lock.
async fn do_write(
    file_path: &str,
    content: &str,
    boundary: &Option<ProjectBoundary>,
    file_tracker: &Option<FileTracker>,
    lsp_manager: &Option<Arc<LspManager>>,
) -> Result<String, String> {
    let path = Path::new(file_path);

    // Require absolute paths
    if !path.is_absolute() {
        return Err("filePath must be an absolute path".to_string());
    }

    // Check project boundary
    if let Some(ref boundary) = boundary {
        boundary.check(file_path)?;
    }

    // Check if file already exists
    let created = !path.exists();

    // Read old content for diff generation (before staleness check modifies anything)
    let old_content = if !created {
        tokio::fs::read_to_string(file_path).await.ok()
    } else {
        None
    };

    // Enforce staleness check for existing files (includes never-read check)
    if !created {
        if let Some(ref tracker) = file_tracker {
            tracker.assert_not_stale(file_path)?;
        }
    }

    // Create parent directories if needed
    if let Some(parent) = path.parent() {
        if !parent.exists() {
            tokio::fs::create_dir_all(parent)
                .await
                .map_err(|e| format!("Failed to create parent directories: {e}"))?;
        }
    }

    let bytes_written = content.len();

    tokio::fs::write(file_path, content)
        .await
        .map_err(|e| format!("Failed to write file: {e}"))?;

    // Update tracked timestamp after successful write
    if let Some(ref tracker) = file_tracker {
        tracker.mark_written(file_path);
    }

    // Generate diff
    let diff = old_content.map(|old| crate::diff_helper::generate_diff(file_path, &old, content));
    let diff = diff.filter(|d| !d.is_empty());

    // Fetch LSP diagnostics
    let diagnostics = if let Some(ref lsp) = lsp_manager {
        let output = crate::diagnostics_helper::fetch_diagnostics_output(lsp, file_path).await;
        if output.is_empty() {
            None
        } else {
            Some(output)
        }
    } else {
        None
    };

    let result = WriteResult {
        path: file_path.to_string(),
        bytes_written,
        created,
        diff,
        diagnostics,
    };

    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
}

#[async_trait]
impl ToolExecutor for WriteTool {
    fn name(&self) -> &str {
        "write"
    }

    fn description(&self) -> &str {
        include_str!("instructions/write.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WriteArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WriteArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let file_path = args.file_path.clone();
        let content = args.content.clone();
        let boundary = self.boundary.clone();
        let file_tracker = self.file_tracker.clone();
        let lsp_manager = self.lsp_manager.clone();

        if let Some(ref tracker) = self.file_tracker {
            let tracker = tracker.clone();
            tracker
                .with_lock(&file_path, || {
                    do_write(&file_path, &content, &boundary, &file_tracker, &lsp_manager)
                })
                .await
        } else {
            do_write(&file_path, &content, &boundary, &file_tracker, &lsp_manager).await
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_write_new_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("new_file.txt");

        let tool = WriteTool::new();
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
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
            "filePath": file_path.to_str().unwrap(),
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
            "filePath": file_path.to_str().unwrap(),
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
            "filePath": "relative/path.txt",
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
            "filePath": path_str,
            "content": "first write"
        });
        tool.execute(args)
            .await
            .expect("first write should succeed");

        // Second write should work without re-reading (because mark_written was called)
        let args2 = serde_json::json!({
            "filePath": path_str,
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
            "filePath": path_str,
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
            "filePath": file_path.to_str().unwrap(),
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
                    "filePath": path1,
                    "content": "write A"
                }))
                .await
        });

        let tool2 = tool.clone();
        let path2 = path_str.clone();
        let h2 = tokio::spawn(async move {
            tool2
                .execute(serde_json::json!({
                    "filePath": path2,
                    "content": "write B"
                }))
                .await
        });

        let (r1, r2) = tokio::join!(h1, h2);
        // Both should succeed (serialized by lock)
        assert!(r1.unwrap().is_ok());
        assert!(r2.unwrap().is_ok());
    }
}
