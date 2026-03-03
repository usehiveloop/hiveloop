use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::boundary::ProjectBoundary;
use crate::edit::apply_edit;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

/// A single edit operation within a multiedit.
#[derive(Debug, Deserialize, JsonSchema)]
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
}

pub struct MultiEditTool {
    file_tracker: Option<FileTracker>,
    boundary: Option<ProjectBoundary>,
}

impl MultiEditTool {
    pub fn new() -> Self {
        Self {
            file_tracker: None,
            boundary: None,
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
}

impl Default for MultiEditTool {
    fn default() -> Self {
        Self::new()
    }
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

        let file_path = &args.file_path;

        if args.edits.is_empty() {
            return Err("No edits provided".to_string());
        }

        // Check project boundary
        if let Some(ref boundary) = self.boundary {
            boundary.check(file_path)?;
        }

        // Enforce read-before-edit
        if let Some(ref tracker) = self.file_tracker {
            tracker.require_read(file_path)?;
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

        for (i, edit) in args.edits.iter().enumerate() {
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

        let result = MultiEditResult {
            path: file_path.clone(),
            edits_applied: args.edits.len(),
            total_replacements,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
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
