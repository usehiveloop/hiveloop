use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

/// Arguments for the Edit tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct EditArgs {
    /// Absolute path to the file to modify.
    #[schemars(description = "Absolute path to the file to modify")]
    pub file_path: String,
    /// The exact text to find and replace. Must match uniquely in the file unless replaceAll is true.
    #[schemars(description = "The exact text to find and replace. Must match uniquely in the file unless replaceAll is true")]
    pub old_string: String,
    /// The replacement text. Must differ from oldString.
    #[schemars(description = "The replacement text. Must differ from oldString")]
    pub new_string: String,
    /// If true, replace all occurrences of oldString. Defaults to false.
    #[schemars(description = "If true, replace all occurrences of oldString. Defaults to false")]
    pub replace_all: Option<bool>,
}

/// Result returned by the Edit tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct EditResult {
    pub path: String,
    pub old_content_snippet: String,
    pub new_content_snippet: String,
    pub replacements_made: usize,
}

/// Shared edit logic used by both Edit and MultiEdit tools.
///
/// Applies a single find-and-replace operation on `content`.
/// Uses a chain of 9 matching strategies (exact → fuzzy) in order.
/// Returns the new content on success.
pub(crate) fn apply_edit(
    content: &str,
    old_string: &str,
    new_string: &str,
    replace_all: bool,
) -> Result<(String, usize), String> {
    if old_string == new_string {
        return Err("oldString and newString are identical".to_string());
    }

    // Try each strategy in order — first match wins
    for strategy in crate::edit_strategies::all_strategies() {
        if let Some((new_content, count)) =
            strategy.try_replace(content, old_string, new_string, replace_all)
        {
            return Ok((new_content, count));
        }
    }

    // No strategy matched — check if there were multiple matches that
    // prevented a non-replace_all edit from succeeding
    let exact_count = content.matches(old_string).count();
    if exact_count > 1 && !replace_all {
        return Err(
            "Found multiple matches for oldString. Provide more surrounding lines in oldString to identify the correct match, or use replaceAll.".to_string()
        );
    }

    Err("oldString not found in file content".to_string())
}

fn snippet(s: &str, max_len: usize) -> String {
    if s.len() <= max_len {
        s.to_string()
    } else {
        format!("{}...", &s[..max_len])
    }
}

pub struct EditTool {
    file_tracker: Option<FileTracker>,
    boundary: Option<ProjectBoundary>,
}

impl EditTool {
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

impl Default for EditTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for EditTool {
    fn name(&self) -> &str {
        "edit"
    }

    fn description(&self) -> &str {
        include_str!("instructions/edit.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(EditArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: EditArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let file_path = &args.file_path;
        let replace_all = args.replace_all.unwrap_or(false);

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

        let (new_content, replacements_made) =
            apply_edit(&content, &args.old_string, &args.new_string, replace_all)?;

        tokio::fs::write(file_path, &new_content)
            .await
            .map_err(|e| format!("Failed to write file: {e}"))?;

        let result = EditResult {
            path: file_path.clone(),
            old_content_snippet: snippet(&args.old_string, 200),
            new_content_snippet: snippet(&args.new_string, 200),
            replacements_made,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write as IoWrite;
    use tempfile::NamedTempFile;

    #[test]
    fn test_apply_edit_exact_match() {
        let content = "hello world\nfoo bar\nbaz qux\n";
        let (result, count) = apply_edit(content, "foo bar", "foo replaced", false).unwrap();
        assert!(result.contains("foo replaced"));
        assert!(!result.contains("foo bar"));
        assert_eq!(count, 1);
    }

    #[test]
    fn test_apply_edit_not_found() {
        let content = "hello world\n";
        let err = apply_edit(content, "not here", "replacement", false).unwrap_err();
        assert!(err.contains("not found"));
    }

    #[test]
    fn test_apply_edit_multiple_matches_no_replace_all() {
        // With the new strategy chain, multiple exact matches with
        // replace_all=false now picks the first occurrence via MultiOccurrenceReplacer
        let content = "aaa\nbbb\naaa\n";
        let (result, count) = apply_edit(content, "aaa", "ccc", false).unwrap();
        assert_eq!(count, 1);
        // First occurrence should be replaced
        assert!(result.starts_with("ccc\n"));
        // Second occurrence should remain
        assert!(result.contains("\naaa\n"));
    }

    #[test]
    fn test_apply_edit_replace_all() {
        let content = "aaa\nbbb\naaa\n";
        let (result, count) = apply_edit(content, "aaa", "ccc", true).unwrap();
        assert_eq!(result.matches("ccc").count(), 2);
        assert_eq!(count, 2);
    }

    #[test]
    fn test_apply_edit_identical_strings() {
        let content = "hello\n";
        let err = apply_edit(content, "hello", "hello", false).unwrap_err();
        assert!(err.contains("identical"));
    }

    #[tokio::test]
    async fn test_edit_tool_execute() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        write!(tmp, "line one\nline two\nline three\n").expect("write");

        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": tmp.path().to_str().unwrap(),
            "oldString": "line two",
            "newString": "line TWO"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: EditResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.replacements_made, 1);

        // Verify file was actually written
        let content = std::fs::read_to_string(tmp.path()).expect("read");
        assert!(content.contains("line TWO"));
        assert!(!content.contains("line two"));
    }

    #[tokio::test]
    async fn test_edit_tool_not_found_file() {
        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": "/tmp/nonexistent_edit_test_xyz.txt",
            "oldString": "foo",
            "newString": "bar"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("not found") || err.contains("Not found"));
    }
}
