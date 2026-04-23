//! Read tool: reads files and directories with optional pagination.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

mod execute;
mod helpers;
mod text;

#[cfg(test)]
mod tests;

use execute::{
    detect_binary, fetch_metadata, is_pdf, read_directory, read_image, read_pdf,
    reject_binary_extension, BinaryDetection,
};
use text::read_text_file;

/// Default line limit when the caller does not provide one explicitly.
/// If a file has more lines than this and the caller omitted `limit`, the
/// tool fails with a hard gate and tells the agent to either pass `offset`
/// + `limit`, use `RipGrep`, or spawn a `self_agent`.
pub const DEFAULT_LINE_LIMIT: usize = 2000;

/// Absolute upper bound on file size, regardless of `limit`. Anything larger
/// is refused outright: the agent must search (`RipGrep`) or summarize
/// (`self_agent`) instead of ingesting. Prevents OOM on pathological files.
pub(super) const HARD_MAX_BYTES: u64 = 10 * 1024 * 1024; // 10MB

/// Maximum line length before truncation.
pub(super) const MAX_LINE_LENGTH: usize = 2000;

/// Arguments for the Read tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct ReadArgs {
    /// Absolute path to the file to read. Example: '/home/user/project/src/main.rs'
    #[schemars(
        description = "Absolute path to the file to read. Example: '/home/user/project/src/main.rs'"
    )]
    pub file_path: String,
    /// Line number to start reading from (1-based). Use with limit for large files.
    #[schemars(
        description = "Line number to start reading from (1-based). Use with limit for large files"
    )]
    pub offset: Option<usize>,
    /// Maximum number of lines to read. If the file has more than 2000 lines
    /// you MUST pass `limit` explicitly; otherwise the call fails with a
    /// steer toward ranged reads, RipGrep, or self_agent. Use with offset
    /// for pagination.
    #[schemars(
        description = "Maximum number of lines to read. If the file has more than 2000 lines you MUST pass `limit` explicitly — otherwise the call fails and steers you toward RipGrep or self_agent. Use with offset for pagination."
    )]
    pub limit: Option<usize>,
}

/// Result returned by the Read tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct ReadResult {
    pub content: String,
    pub total_lines: usize,
    pub lines_read: usize,
    pub truncated: bool,
}

pub struct ReadTool {
    file_tracker: Option<FileTracker>,
    boundary: Option<ProjectBoundary>,
}

impl ReadTool {
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

impl Default for ReadTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for ReadTool {
    fn name(&self) -> &str {
        "Read"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/read.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(ReadArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: ReadArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let file_path = &args.file_path;
        let offset = args.offset.unwrap_or(1);
        let limit_explicit = args.limit.is_some();
        let limit = args.limit.unwrap_or(DEFAULT_LINE_LIMIT);

        // Validate absolute path
        if !Path::new(file_path).is_absolute() {
            return Err(format!(
                "file_path must be an absolute path, got: {file_path}"
            ));
        }

        // Check project boundary
        if let Some(ref boundary) = self.boundary {
            boundary.check(file_path)?;
        }

        // Check if path is a directory
        let metadata = fetch_metadata(file_path).await?;

        // Hard byte cap: refuse to ingest enormous files regardless of limit.
        // Applies to regular files only (directories have their own gate below).
        if metadata.is_file() && metadata.len() > HARD_MAX_BYTES {
            return Err(format!(
                "File {} is too large to read in-context ({} bytes, max: {}). Use RipGrep with path=\"{}\" and a regex pattern to search, or spawn self_agent with a focused question to summarize.",
                file_path,
                metadata.len(),
                HARD_MAX_BYTES,
                file_path
            ));
        }

        if metadata.is_dir() {
            return read_directory(file_path, offset, limit, limit_explicit).await;
        }

        let path_obj = Path::new(file_path);

        // Check for known binary extensions first (skip content check)
        if let Some(rejection) = reject_binary_extension(path_obj, metadata.len()) {
            return rejection;
        }

        // Check for PDF files
        if is_pdf(path_obj) {
            return read_pdf(file_path, &self.file_tracker).await;
        }

        // Binary detection: sample bytes and check for null bytes or high non-printable ratio
        match detect_binary(file_path, path_obj, metadata.len()).await? {
            BinaryDetection::Image => {
                return read_image(file_path, path_obj, &self.file_tracker).await;
            }
            BinaryDetection::Reject(msg) => return Err(msg),
            BinaryDetection::Text | BinaryDetection::Svg => {}
        }

        read_text_file(file_path, offset, limit, limit_explicit, &self.file_tracker).await
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
