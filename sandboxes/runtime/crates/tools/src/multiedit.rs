use async_trait::async_trait;
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::boundary::ProjectBoundary;
use crate::edit::apply_edit;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

/// A single edit operation within a multiedit.
#[derive(Debug, Clone, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct SingleEdit {
    /// The text to find and replace.
    pub old_string: String,
    /// The replacement text.
    pub new_string: String,
    /// If true, replace all occurrences. Defaults to false.
    pub replace_all: Option<bool>,
}

/// Arguments for the MultiEdit tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct MultiEditArgs {
    /// The absolute path to the file to modify.
    pub file_path: String,
    /// The list of edit operations to apply sequentially.
    pub edits: Vec<SingleEdit>,
}

/// Result returned by the MultiEdit tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct MultiEditResult {
    pub path: String,
    pub edits_applied: usize,
    pub total_replacements: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
}

pub struct MultiEditTool {
    file_tracker: Option<FileTracker>,
    boundary: Option<ProjectBoundary>,
    lsp_manager: Option<Arc<LspManager>>,
}

impl MultiEditTool {
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

impl Default for MultiEditTool {
    fn default() -> Self {
        Self::new()
    }
}

/// Core multiedit logic extracted so it can be called from within with_lock.
async fn do_multiedit(
    file_path: &str,
    edits: &[SingleEdit],
    boundary: &Option<ProjectBoundary>,
    file_tracker: &Option<FileTracker>,
    lsp_manager: &Option<Arc<LspManager>>,
) -> Result<String, String> {
    // Check project boundary
    if let Some(ref boundary) = boundary {
        boundary.check(file_path)?;
    }

    // Enforce staleness check (includes never-read check)
    if let Some(ref tracker) = file_tracker {
        tracker.assert_not_stale(file_path)?;
    }

    let content = tokio::fs::read_to_string(file_path)
        .await
        .map_err(|e| match e.kind() {
            std::io::ErrorKind::NotFound => format!("File not found: {file_path}"),
            std::io::ErrorKind::PermissionDenied => {
                format!("Permission denied: {file_path}")
            }
            _ => format!("Failed to read file: {e}"),
        })?;

    // Apply all edits sequentially — if any fails, no partial writes happen
    let mut current_content = content;
    let mut total_replacements = 0;

    for (i, edit) in edits.iter().enumerate() {
        let replace_all = edit.replace_all.unwrap_or(false);
        let (new_content, count) = apply_edit(
            &current_content,
            &edit.old_string,
            &edit.new_string,
            replace_all,
        )
        .map_err(|e| format!("Edit #{} failed: {e}", i + 1))?;
        current_content = new_content;
        total_replacements += count;
    }

    // Write final content
    tokio::fs::write(file_path, &current_content)
        .await
        .map_err(|e| format!("Failed to write file: {e}"))?;

    // Update tracked timestamp after successful write
    if let Some(ref tracker) = file_tracker {
        tracker.mark_written(file_path);
    }

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

    let result = MultiEditResult {
        path: file_path.to_string(),
        edits_applied: edits.len(),
        total_replacements,
        diagnostics,
    };

    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
}

#[async_trait]
impl ToolExecutor for MultiEditTool {
    fn name(&self) -> &str {
        "multiedit"
    }

    fn description(&self) -> &str {
        include_str!("instructions/multiedit.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(MultiEditArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: MultiEditArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let file_path = args.file_path.clone();

        if args.edits.is_empty() {
            return Err("No edits provided".to_string());
        }

        let edits = args.edits.clone();
        let boundary = self.boundary.clone();
        let file_tracker = self.file_tracker.clone();
        let lsp_manager = self.lsp_manager.clone();

        if let Some(ref tracker) = self.file_tracker {
            let tracker = tracker.clone();
            tracker
                .with_lock(&file_path, || {
                    do_multiedit(&file_path, &edits, &boundary, &file_tracker, &lsp_manager)
                })
                .await
        } else {
            do_multiedit(&file_path, &edits, &boundary, &file_tracker, &lsp_manager).await
        }
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write as IoWrite;
    use tempfile::NamedTempFile;

    #[tokio::test]
    async fn test_multiedit_sequential() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        write!(tmp, "aaa\nbbb\nccc\n").expect("write");

        let tool = MultiEditTool::new();
        let args = serde_json::json!({
            "filePath": tmp.path().to_str().unwrap(),
            "edits": [
                { "oldString": "aaa", "newString": "AAA" },
                { "oldString": "ccc", "newString": "CCC" }
            ]
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: MultiEditResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.edits_applied, 2);
        assert_eq!(parsed.total_replacements, 2);

        let content = std::fs::read_to_string(tmp.path()).expect("read");
        assert!(content.contains("AAA"));
        assert!(content.contains("CCC"));
        assert!(!content.contains("aaa"));
        assert!(!content.contains("ccc"));
    }

    #[tokio::test]
    async fn test_multiedit_atomic_failure() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        write!(tmp, "aaa\nbbb\nccc\n").expect("write");

        let tool = MultiEditTool::new();
        let args = serde_json::json!({
            "filePath": tmp.path().to_str().unwrap(),
            "edits": [
                { "oldString": "aaa", "newString": "AAA" },
                { "oldString": "zzz_not_found", "newString": "ZZZ" }
            ]
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Edit #2 failed"));

        // File should remain unchanged
        let content = std::fs::read_to_string(tmp.path()).expect("read");
        assert!(content.contains("aaa"));
    }

    #[tokio::test]
    async fn test_multiedit_empty_edits() {
        let tool = MultiEditTool::new();
        let args = serde_json::json!({
            "filePath": "/tmp/whatever.txt",
            "edits": []
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("No edits"));
    }
}
