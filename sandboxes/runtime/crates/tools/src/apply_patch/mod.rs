//! ApplyPatch tool: parse patch text and apply Add/Delete/Update hunks.

use async_trait::async_trait;
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::ToolExecutor;

mod apply;
mod matcher;
mod parser;

#[cfg(test)]
mod tests;

use apply::apply_hunks;
use parser::parse_patch;

/// Arguments for the ApplyPatch tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct ApplyPatchArgs {
    /// The full patch text that describes all changes to be made.
    pub patch_text: String,
}

/// Result returned by the ApplyPatch tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct ApplyPatchResult {
    pub summary: Vec<String>,
    pub files_changed: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
}

pub struct ApplyPatchTool {
    lsp_manager: Option<Arc<LspManager>>,
}

impl ApplyPatchTool {
    pub fn new() -> Self {
        Self { lsp_manager: None }
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

impl Default for ApplyPatchTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for ApplyPatchTool {
    fn name(&self) -> &str {
        "apply_patch"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/apply_patch.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(ApplyPatchArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: ApplyPatchArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let hunks = parse_patch(&args.patch_text)?;
        if hunks.is_empty() {
            return Err("No files were modified.".to_string());
        }

        let summary = apply_hunks(&hunks).await?;
        let files_changed = summary.len();

        // Fetch LSP diagnostics for all modified/added files
        let diagnostics = if let Some(ref lsp) = self.lsp_manager {
            let mut all_diag = String::new();
            for entry in &summary {
                // entries are like "M path" or "A path"
                let file_path = entry.split_whitespace().nth(1).unwrap_or("");
                if !file_path.is_empty() && (entry.starts_with("M ") || entry.starts_with("A ")) {
                    let output =
                        crate::diagnostics_helper::fetch_diagnostics_output(lsp, file_path).await;
                    if !output.is_empty() {
                        all_diag.push_str(&output);
                    }
                }
            }
            if all_diag.is_empty() {
                None
            } else {
                Some(all_diag)
            }
        } else {
            None
        };

        let result = ApplyPatchResult {
            summary,
            files_changed,
            diagnostics,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
