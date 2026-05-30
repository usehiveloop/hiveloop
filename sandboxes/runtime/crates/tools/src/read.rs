use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::ReadFileConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::operations::ReadOperations;
use crate::path::{build_glob_set, enforce_deny_globs, resolve_within_workspace, PathPolicyError};
use crate::truncate::{truncate_head, TruncationReason, DEFAULT_MAX_BYTES, DEFAULT_MAX_LINES};
use crate::{schema_for, JsonTool, ToolDefinition};

const TOOL_NAME: &str = "read_file";
const TOOL_DESCRIPTION: &str =
    "Read the contents of a file in the workspace. Supports text files. \
     Output is truncated to 2000 lines or 50KB, whichever comes first. Use \
     `offset` and `limit` for partial reads of large files. Returns an error \
     if the path is outside the allowed workspace roots or matches a denied \
     pattern.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct ReadArgs {
    /// Path to the file to read (relative to the workspace root or absolute).
    pub path: String,
    /// Optional 1-indexed line number to start reading from.
    #[serde(default)]
    pub offset: Option<usize>,
    /// Optional maximum number of lines to read.
    #[serde(default)]
    pub limit: Option<usize>,
}

pub struct ReadTool {
    config: ReadFileConfig,
    workspace_root: PathBuf,
    operations: Arc<dyn ReadOperations>,
    files_read: Option<Arc<Mutex<HashSet<PathBuf>>>>,
}

impl ReadTool {
    pub fn new(
        config: ReadFileConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn ReadOperations>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            operations,
            files_read: None,
        }
    }

    pub fn with_files_read(mut self, files_read: Arc<Mutex<HashSet<PathBuf>>>) -> Self {
        self.files_read = Some(files_read);
        self
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: ReadArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
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
            .map_err(|e| anyhow!("{}: {e}", parsed.path))?;

        if let Some(mime) = self.operations.detect_image_mime(&resolved).await {
            return Ok(json!({
                "path": resolved.display().to_string(),
                "mime_type": mime,
                "note": "image files are not inlined here; the runtime forwards image attachments through the multimodal model when applicable",
            }));
        }

        let bytes = self
            .operations
            .read_file(&resolved)
            .await
            .map_err(|e| anyhow!("read failed for {}: {e}", parsed.path))?;

        // Track that this file was read, for read-before-edit enforcement
        if let Some(ref files_read) = self.files_read {
            if let Ok(mut guard) = files_read.lock() {
                guard.insert(resolved.clone());
            }
        }
        let max_bytes = self.config.max_file_size_bytes as usize;
        if bytes.len() > max_bytes {
            return Err(anyhow!(
                "{} exceeds max_file_size_bytes ({} > {})",
                parsed.path,
                bytes.len(),
                max_bytes
            ));
        }
        let text = match String::from_utf8(bytes) {
            Ok(text) => text,
            Err(_) => {
                return Err(anyhow!("{} is not valid UTF-8", parsed.path));
            }
        };

        let sliced = slice_for_offset_limit(&text, parsed.offset, parsed.limit)?;
        let truncated = truncate_head(&sliced, DEFAULT_MAX_LINES, DEFAULT_MAX_BYTES);

        Ok(json!({
            "path": resolved.display().to_string(),
            "content": truncated.content,
            "truncated": truncated.truncated,
            "truncated_by": match truncated.truncated_by {
                TruncationReason::NotTruncated => "none",
                TruncationReason::Lines => "lines",
                TruncationReason::Bytes => "bytes",
            },
            "total_lines": truncated.total_lines,
            "total_bytes": truncated.total_bytes,
            "shown_lines": truncated.output_lines,
            "shown_bytes": truncated.output_bytes,
            "offset": parsed.offset,
            "limit": parsed.limit,
        }))
    }
}

#[async_trait]
impl JsonTool for ReadTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: TOOL_NAME.to_string(),
            description: TOOL_DESCRIPTION.to_string(),
            parameters: schema_for::<ReadArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

fn slice_for_offset_limit(
    text: &str,
    offset: Option<usize>,
    limit: Option<usize>,
) -> Result<String> {
    if offset.is_none() && limit.is_none() {
        return Ok(text.to_string());
    }
    let lines: Vec<&str> = text.split_inclusive('\n').collect();
    let start = offset.unwrap_or(1).saturating_sub(1);
    if start >= lines.len() && !lines.is_empty() {
        return Err(anyhow!(
            "offset {} is beyond end of file ({} lines total)",
            start + 1,
            lines.len()
        ));
    }
    let end = match limit {
        Some(n) => (start + n).min(lines.len()),
        None => lines.len(),
    };
    Ok(lines[start..end].concat())
}

fn map_path_error(error: PathPolicyError) -> anyhow::Error {
    anyhow!(error.to_string())
}
