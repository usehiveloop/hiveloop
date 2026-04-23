use async_trait::async_trait;
use lsp::LspManager;
use std::path::Path;
use std::sync::Arc;

use super::apply::{apply_edit, normalize_line_endings, snippet};
use super::args::{EditArgs, EditResult};
use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

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
        include_str!("../instructions/edit.txt")
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
