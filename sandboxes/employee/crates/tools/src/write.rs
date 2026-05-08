use std::path::PathBuf;
use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use domain::WriteFileConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::mutation_queue::with_file_lock;
use crate::operations::WriteOperations;
use crate::path::{build_glob_set, enforce_deny_globs, resolve_within_workspace, PathPolicyError};

const TOOL_NAME: &str = "write_file";
const TOOL_DESCRIPTION: &str =
    "Write content to a file inside the workspace. Creates the file if it \
     does not exist, overwrites if it does. Parent directories are created \
     automatically. Refuses paths outside the configured allowed roots or \
     paths matching a deny glob.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct WriteArgs {
    /// Path to the file to write (relative to the workspace root or absolute).
    pub path: String,
    /// Full UTF-8 content to place in the file.
    pub content: String,
}

pub struct WriteTool {
    config: WriteFileConfig,
    workspace_root: PathBuf,
    operations: Arc<dyn WriteOperations>,
}

impl WriteTool {
    pub fn new(
        config: WriteFileConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn WriteOperations>,
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
        .with_parameters_schema::<WriteArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: WriteArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let resolved = resolve_within_workspace(
            &self.workspace_root,
            &parsed.path,
            &self.config.allowed_roots,
        )
        .map_err(map_path_error)?;
        let deny_globs = build_glob_set(&self.config.deny_globs);
        enforce_deny_globs(&resolved, &deny_globs).map_err(map_path_error)?;

        let bytes = parsed.content.as_bytes();
        let max_bytes = self.config.max_file_size_bytes as usize;
        if bytes.len() > max_bytes {
            return Err(AdkError::tool(format!(
                "content size {} exceeds max_file_size_bytes ({})",
                bytes.len(),
                max_bytes
            )));
        }

        if let Some(parent) = resolved.parent() {
            self.operations
                .mkdir_all(parent)
                .await
                .map_err(|e| AdkError::tool(format!("mkdir {}: {e}", parent.display())))?;
        }

        let resolved_for_lock = resolved.clone();
        let path_for_write = resolved.clone();
        let operations = self.operations.clone();
        let payload = parsed.content.clone();
        let bytes_count = bytes.len();
        let outcome = with_file_lock(&resolved_for_lock, move || {
            let operations = operations.clone();
            let path_for_write = path_for_write.clone();
            let payload = payload.into_bytes();
            async move { operations.write_file(&path_for_write, &payload).await }
        })
        .await;
        outcome.map_err(|e| AdkError::tool(format!("write failed for {}: {e}", parsed.path)))?;

        Ok(json!({
            "path": resolved.display().to_string(),
            "bytes_written": bytes_count,
        }))
    }
}

fn map_path_error(error: PathPolicyError) -> AdkError {
    AdkError::tool(error.to_string())
}
