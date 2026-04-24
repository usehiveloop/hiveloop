use async_trait::async_trait;
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::sync::Arc;

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

#[cfg(test)]
mod tests;

/// Arguments for the Write tool.
#[derive(Debug, Deserialize, JsonSchema)]
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
///
/// `path` is intentionally omitted from the serialized output — the model
/// just sent the path in `file_path` and re-echoing it on every write
/// added ~120 bytes per call × N later turns of carried context. Kept on
/// the struct (skipped in serde) so tests / internal callers that build a
/// `WriteResult` aren't broken.
#[derive(Debug, Serialize, Deserialize)]
pub struct WriteResult {
    #[serde(skip)]
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
        return Err("file_path must be an absolute path".to_string());
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
        include_str!("../instructions/write.txt")
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

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
