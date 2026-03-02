use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use tokio::io::{AsyncBufReadExt, AsyncReadExt, BufReader};

use crate::ToolExecutor;

/// Arguments for the Read tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct ReadArgs {
    /// The absolute path to the file to read.
    pub file_path: String,
    /// The line number to start reading from (1-indexed). Defaults to 1.
    pub offset: Option<usize>,
    /// The number of lines to read. Defaults to 2000.
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

/// Maximum line length before truncation.
const MAX_LINE_LENGTH: usize = 2000;

/// Number of bytes to check for binary content.
const BINARY_CHECK_SIZE: usize = 8192;

pub struct ReadTool;

impl ReadTool {
    pub fn new() -> Self {
        Self
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
        "Reads a file from the local filesystem. The file_path must be an absolute path."
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
        let limit = args.limit.unwrap_or(2000);

        // Validate absolute path
        if !Path::new(file_path).is_absolute() {
            return Err(format!(
                "file_path must be an absolute path, got: {file_path}"
            ));
        }

        // Check if path is a directory
        let metadata = tokio::fs::metadata(file_path)
            .await
            .map_err(|e| match e.kind() {
                std::io::ErrorKind::NotFound => format!("File not found: {file_path}"),
                std::io::ErrorKind::PermissionDenied => {
                    format!("Permission denied: {file_path}")
                }
                _ => format!("Failed to read file metadata: {e}"),
            })?;

        if metadata.is_dir() {
            return Err(format!("Is a directory: {file_path}"));
        }

        // Binary detection: read first 8192 bytes and check for null bytes
        {
            let mut file = tokio::fs::File::open(file_path)
                .await
                .map_err(|e| match e.kind() {
                    std::io::ErrorKind::PermissionDenied => {
                        format!("Permission denied: {file_path}")
                    }
                    _ => format!("Failed to open file: {e}"),
                })?;
            let mut buf = vec![0u8; BINARY_CHECK_SIZE];
            let bytes_read = file
                .read(&mut buf)
                .await
                .map_err(|e| format!("Failed to read file: {e}"))?;
            if buf[..bytes_read].contains(&0) {
                return Err(format!("Binary file detected: {file_path}"));
            }
        }

        // Read lines using async BufReader
        let file = tokio::fs::File::open(file_path)
            .await
            .map_err(|e| format!("Failed to open file: {e}"))?;
        let reader = BufReader::new(file);
        let mut lines_stream = reader.lines();

        let mut all_lines: Vec<String> = Vec::new();
        while let Some(line) = lines_stream
            .next_line()
            .await
            .map_err(|e| format!("Failed to read line: {e}"))?
        {
            all_lines.push(line);
        }

        let total_lines = all_lines.len();

        // Apply offset (1-indexed) and limit
        let start = if offset > 0 { offset - 1 } else { 0 };
        let end = total_lines.min(start + limit);
        let selected_lines = if start < total_lines {
            &all_lines[start..end]
        } else {
            &[]
        };

        let lines_read = selected_lines.len();
        let truncated = end < total_lines;

        // Compute width needed for line numbers
        let max_line_num = if lines_read > 0 {
            start + lines_read
        } else {
            1
        };
        let num_width = max_line_num.to_string().len();

        // Format lines as "  {line_number}\t{content}" with right-aligned line numbers
        let mut content = String::new();
        for (i, line) in selected_lines.iter().enumerate() {
            let line_num = start + i + 1;
            let display_line = if line.len() > MAX_LINE_LENGTH {
                format!("{}...", &line[..MAX_LINE_LENGTH])
            } else {
                line.to_string()
            };
            content.push_str(&format!(
                "{:>width$}\t{}\n",
                line_num,
                display_line,
                width = num_width
            ));
        }

        let result = ReadResult {
            content,
            total_lines,
            lines_read,
            truncated,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;

    #[tokio::test]
    async fn test_read_simple_file() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        writeln!(tmp, "line one").expect("write");
        writeln!(tmp, "line two").expect("write");
        writeln!(tmp, "line three").expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": tmp.path().to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_lines, 3);
        assert_eq!(parsed.lines_read, 3);
        assert!(!parsed.truncated);
        assert!(parsed.content.contains("line one"));
        assert!(parsed.content.contains("line two"));
        assert!(parsed.content.contains("line three"));
    }

    #[tokio::test]
    async fn test_read_with_offset_and_limit() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        for i in 1..=10 {
            writeln!(tmp, "line {i}").expect("write");
        }

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": tmp.path().to_str().unwrap(),
            "offset": 3,
            "limit": 2
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_lines, 10);
        assert_eq!(parsed.lines_read, 2);
        assert!(parsed.truncated);
        assert!(parsed.content.contains("line 3"));
        assert!(parsed.content.contains("line 4"));
        assert!(!parsed.content.contains("line 2"));
        assert!(!parsed.content.contains("line 5"));
    }

    #[tokio::test]
    async fn test_read_relative_path_error() {
        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": "relative/path.txt"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("absolute path"));
    }

    #[tokio::test]
    async fn test_read_directory_error() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": dir.path().to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Is a directory"));
    }

    #[tokio::test]
    async fn test_read_not_found_error() {
        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": "/tmp/nonexistent_file_read_test_xyz.txt"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("not found"));
    }

    #[tokio::test]
    async fn test_read_binary_file_error() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        tmp.write_all(&[0x00, 0x01, 0x02]).expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": tmp.path().to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Binary file detected"));
    }

    #[tokio::test]
    async fn test_read_line_truncation() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        let long_line = "x".repeat(3000);
        writeln!(tmp, "{long_line}").expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": tmp.path().to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        // The content should have the truncated line ending with "..."
        assert!(parsed.content.contains("..."));
        // The displayed line should be at most MAX_LINE_LENGTH + "..." = 2003 chars (plus line num prefix)
    }

    #[tokio::test]
    async fn test_read_line_number_formatting() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        writeln!(tmp, "first").expect("write");
        writeln!(tmp, "second").expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": tmp.path().to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        // Lines should be formatted with right-aligned numbers and tab
        assert!(parsed.content.contains("1\tfirst"));
        assert!(parsed.content.contains("2\tsecond"));
    }
}
