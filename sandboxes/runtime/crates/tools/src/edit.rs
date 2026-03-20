use async_trait::async_trait;
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::sync::Arc;

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

/// Normalize CRLF and CR line endings to LF.
fn normalize_line_endings(text: &str) -> String {
    text.replace("\r\n", "\n").replace('\r', "\n")
}

/// Arguments for the Edit tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct EditArgs {
    /// Absolute path to the file to modify.
    #[schemars(description = "Absolute path to the file to modify")]
    pub file_path: String,
    /// The exact text to find and replace. Must match uniquely in the file unless replaceAll is true.
    #[schemars(
        description = "The exact text to find and replace. Must match uniquely in the file unless replaceAll is true"
    )]
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
    pub lines_added: usize,
    pub lines_removed: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diff: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
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
    lsp_manager: Option<Arc<LspManager>>,
}

impl EditTool {
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

impl Default for EditTool {
    fn default() -> Self {
        Self::new()
    }
}

/// Core edit logic extracted so it can be called from within with_lock.
async fn do_edit(
    file_path: &str,
    old_string: &str,
    new_string: &str,
    replace_all: bool,
    boundary: &Option<ProjectBoundary>,
    file_tracker: &Option<FileTracker>,
    lsp_manager: &Option<Arc<LspManager>>,
) -> Result<String, String> {
    // Check project boundary
    if let Some(ref boundary) = boundary {
        boundary.check(file_path)?;
    }

    // Handle empty oldString = create/append file
    if old_string.is_empty() {
        let new_string_norm = normalize_line_endings(new_string);
        let content = if Path::new(file_path).exists() {
            // Append to existing file
            // Still enforce staleness when appending to existing file
            if let Some(ref tracker) = file_tracker {
                tracker.assert_not_stale(file_path)?;
            }
            let existing = tokio::fs::read_to_string(file_path)
                .await
                .map_err(|e| format!("Failed to read file: {e}"))?;
            format!("{}{}", existing, new_string_norm)
        } else {
            // Create new file
            if let Some(parent) = Path::new(file_path).parent() {
                if !parent.exists() {
                    tokio::fs::create_dir_all(parent)
                        .await
                        .map_err(|e| format!("Failed to create parent dirs: {e}"))?;
                }
            }
            new_string_norm.to_string()
        };

        tokio::fs::write(file_path, &content)
            .await
            .map_err(|e| format!("Failed to write file: {e}"))?;

        if let Some(ref tracker) = file_tracker {
            tracker.mark_written(file_path);
        }

        // Fetch LSP diagnostics for create/append case
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

        let result = EditResult {
            path: file_path.to_string(),
            old_content_snippet: String::new(),
            new_content_snippet: snippet(new_string, 200),
            replacements_made: 1,
            lines_added: new_string.lines().count(),
            lines_removed: 0,
            diff: None,
            diagnostics,
        };
        return serde_json::to_string(&result)
            .map_err(|e| format!("Failed to serialize result: {e}"));
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

    // Normalize line endings before matching
    let content = normalize_line_endings(&content);
    let old_string_norm = normalize_line_endings(old_string);
    let new_string_norm = normalize_line_endings(new_string);

    let (new_content, replacements_made) =
        apply_edit(&content, &old_string_norm, &new_string_norm, replace_all)?;

    // Compute line change statistics
    let old_line_count = content.lines().count();
    let new_line_count = new_content.lines().count();
    let lines_added = new_line_count.saturating_sub(old_line_count);
    let lines_removed = old_line_count.saturating_sub(new_line_count);

    tokio::fs::write(file_path, &new_content)
        .await
        .map_err(|e| format!("Failed to write file: {e}"))?;

    // Update tracked timestamp after successful write
    if let Some(ref tracker) = file_tracker {
        tracker.mark_written(file_path);
    }

    // Generate diff
    let diff = crate::diff_helper::generate_diff(file_path, &content, &new_content);
    let diff = if diff.is_empty() { None } else { Some(diff) };

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

    let result = EditResult {
        path: file_path.to_string(),
        old_content_snippet: snippet(old_string, 200),
        new_content_snippet: snippet(new_string, 200),
        replacements_made,
        lines_added,
        lines_removed,
        diff,
        diagnostics,
    };

    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
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

        let file_path = args.file_path.clone();
        let old_string = args.old_string.clone();
        let new_string = args.new_string.clone();
        let replace_all = args.replace_all.unwrap_or(false);
        let boundary = self.boundary.clone();
        let file_tracker = self.file_tracker.clone();
        let lsp_manager = self.lsp_manager.clone();

        if let Some(ref tracker) = self.file_tracker {
            let tracker = tracker.clone();
            tracker
                .with_lock(&file_path, || {
                    do_edit(
                        &file_path,
                        &old_string,
                        &new_string,
                        replace_all,
                        &boundary,
                        &file_tracker,
                        &lsp_manager,
                    )
                })
                .await
        } else {
            do_edit(
                &file_path,
                &old_string,
                &new_string,
                replace_all,
                &boundary,
                &file_tracker,
                &lsp_manager,
            )
            .await
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

    #[tokio::test]
    async fn test_edit_marks_written_after_success() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("tracked.txt");
        std::fs::write(&file_path, "aaa\nbbb\nccc\n").expect("write");

        let tracker = FileTracker::new();
        let path_str = file_path.to_str().unwrap();

        // Mark as read first
        tracker.mark_read(path_str);

        let tool = EditTool::new().with_file_tracker(tracker.clone());

        // First edit
        let args = serde_json::json!({
            "filePath": path_str,
            "oldString": "aaa",
            "newString": "AAA"
        });
        tool.execute(args).await.expect("first edit should succeed");

        // Second edit should work without re-reading (because mark_written was called)
        let args2 = serde_json::json!({
            "filePath": path_str,
            "oldString": "bbb",
            "newString": "BBB"
        });
        tool.execute(args2)
            .await
            .expect("second edit should succeed without re-reading");

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert!(content.contains("AAA"));
        assert!(content.contains("BBB"));
    }

    #[tokio::test]
    async fn test_edit_rejects_stale_file() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("stale_edit.txt");
        std::fs::write(&file_path, "original content\n").expect("write");

        let tracker = FileTracker::new();
        let path_str = file_path.to_str().unwrap();

        tracker.mark_read(path_str);

        // Modify externally
        std::thread::sleep(std::time::Duration::from_millis(100));
        std::fs::write(&file_path, "modified externally\n").expect("write");

        let tool = EditTool::new().with_file_tracker(tracker);
        let args = serde_json::json!({
            "filePath": path_str,
            "oldString": "original content",
            "newString": "new content"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("has been modified"));
    }

    #[tokio::test]
    async fn test_edit_empty_old_string_creates_file() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("new_created.txt");

        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
            "oldString": "",
            "newString": "new file content\nline two\n"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: EditResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.replacements_made, 1);
        let content = std::fs::read_to_string(&file_path).expect("read");
        assert_eq!(content, "new file content\nline two\n");
    }

    #[tokio::test]
    async fn test_edit_empty_old_string_appends_existing() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("existing.txt");
        std::fs::write(&file_path, "original\n").expect("write");

        let tracker = FileTracker::new();
        tracker.mark_read(file_path.to_str().unwrap());

        let tool = EditTool::new().with_file_tracker(tracker);
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
            "oldString": "",
            "newString": "appended\n"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: EditResult = serde_json::from_str(&result).expect("parse");
        assert_eq!(parsed.replacements_made, 1);

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert_eq!(content, "original\nappended\n");
    }

    #[tokio::test]
    async fn test_edit_crlf_normalization() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("crlf.txt");
        // Write file with CRLF line endings
        std::fs::write(&file_path, "line one\r\nline two\r\nline three\r\n").expect("write");

        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
            "oldString": "line two",
            "newString": "line TWO"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: EditResult = serde_json::from_str(&result).expect("parse");
        assert_eq!(parsed.replacements_made, 1);

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert!(content.contains("line TWO"));
    }

    #[tokio::test]
    async fn test_edit_line_counts_in_output() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        write!(tmp, "line1\nline2\nline3\n").expect("write");

        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": tmp.path().to_str().unwrap(),
            "oldString": "line2",
            "newString": "line2a\nline2b\nline2c"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: EditResult = serde_json::from_str(&result).expect("parse");

        // Replaced 1 line with 3 lines = +2 lines added, 0 removed
        assert_eq!(parsed.lines_added, 2);
        assert_eq!(parsed.lines_removed, 0);
    }
}
