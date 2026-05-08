use std::path::PathBuf;
use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use domain::WriteFileConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::diff::{apply_edits, unified_diff, EditMatchError, PendingEdit};
use crate::mutation_queue::with_file_lock;
use crate::operations::EditOperations;
use crate::path::{build_glob_set, enforce_deny_globs, resolve_within_workspace, PathPolicyError};

const TOOL_NAME: &str = "edit_file";
const TOOL_DESCRIPTION: &str =
    "Apply targeted text replacements to a file. Each `edits[].old_text` \
     must match exactly one region of the original file (uniqueness is \
     required). Edits are applied to the original file content, not \
     incrementally — overlapping or nested edits are rejected. The file's \
     line endings and BOM are preserved.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct EditOperationArgs {
    /// Exact text in the original file to be replaced. Must be unique in the file.
    pub old_text: String,
    /// Replacement text.
    pub new_text: String,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct EditArgs {
    /// Path to the file to edit (relative to the workspace root or absolute).
    pub path: String,
    /// One or more targeted replacements.
    pub edits: Vec<EditOperationArgs>,
}

pub struct EditTool {
    config: WriteFileConfig,
    workspace_root: PathBuf,
    operations: Arc<dyn EditOperations>,
}

impl EditTool {
    pub fn new(
        config: WriteFileConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn EditOperations>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            operations,
        }
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<EditArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: EditArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        if parsed.edits.is_empty() {
            return Err(AdkError::tool("`edits` must contain at least one entry"));
        }

        let resolved = resolve_within_workspace(
            &self.workspace_root,
            &parsed.path,
            &self.config.allowed_roots,
        )
        .map_err(map_path_error)?;
        let deny_globs = build_glob_set(&self.config.deny_globs);
        enforce_deny_globs(&resolved, &deny_globs).map_err(map_path_error)?;

        self.operations
            .access(&resolved)
            .await
            .map_err(|e| AdkError::tool(format!("{}: {e}", parsed.path)))?;
        let original_bytes = self
            .operations
            .read_file(&resolved)
            .await
            .map_err(|e| AdkError::tool(format!("read failed for {}: {e}", parsed.path)))?;
        let original_text = String::from_utf8(original_bytes)
            .map_err(|_| AdkError::tool(format!("{} is not valid UTF-8", parsed.path)))?;

        let (bom, content_without_bom) = strip_utf8_bom(&original_text);
        let line_ending = detect_line_ending(content_without_bom);
        let normalized = normalize_to_lf(content_without_bom);

        let pending: Vec<PendingEdit> = parsed
            .edits
            .iter()
            .map(|entry| PendingEdit {
                old_text: entry.old_text.clone(),
                new_text: entry.new_text.clone(),
            })
            .collect();
        let edited = apply_edits(&normalized, &pending).map_err(map_edit_error)?;
        let restored = restore_line_ending(&edited, line_ending);
        let final_text = format!("{bom}{restored}");
        let final_bytes = final_text.as_bytes();
        let max_bytes = self.config.max_file_size_bytes as usize;
        if final_bytes.len() > max_bytes {
            return Err(AdkError::tool(format!(
                "edited content size {} exceeds max_file_size_bytes ({})",
                final_bytes.len(),
                max_bytes
            )));
        }

        let resolved_for_lock = resolved.clone();
        let operations = self.operations.clone();
        let bytes_for_write = final_bytes.to_vec();
        let path_for_write = resolved.clone();
        let outcome = with_file_lock(&resolved_for_lock, move || {
            let operations = operations.clone();
            let path_for_write = path_for_write.clone();
            let bytes_for_write = bytes_for_write.clone();
            async move { operations.write_file(&path_for_write, &bytes_for_write).await }
        })
        .await;
        outcome.map_err(|e| AdkError::tool(format!("write failed for {}: {e}", parsed.path)))?;

        let diff = unified_diff(&original_text, &final_text, &resolved.display().to_string());

        Ok(json!({
            "path": resolved.display().to_string(),
            "edits_applied": parsed.edits.len(),
            "bytes_written": final_bytes.len(),
            "diff": diff,
        }))
    }
}

fn strip_utf8_bom(text: &str) -> (String, &str) {
    if let Some(rest) = text.strip_prefix('\u{FEFF}') {
        ("\u{FEFF}".to_string(), rest)
    } else {
        (String::new(), text)
    }
}

fn detect_line_ending(text: &str) -> &'static str {
    if text.contains("\r\n") {
        "\r\n"
    } else {
        "\n"
    }
}

fn normalize_to_lf(text: &str) -> String {
    text.replace("\r\n", "\n")
}

fn restore_line_ending(text: &str, line_ending: &str) -> String {
    if line_ending == "\n" {
        text.to_string()
    } else {
        text.replace('\n', line_ending)
    }
}

fn map_path_error(error: PathPolicyError) -> AdkError {
    AdkError::tool(error.to_string())
}

fn map_edit_error(error: EditMatchError) -> AdkError {
    AdkError::tool(error.to_string())
}
