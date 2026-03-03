use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use tokio::io::{AsyncBufReadExt, AsyncReadExt, BufReader};

use crate::boundary::ProjectBoundary;
use crate::file_tracker::FileTracker;
use crate::ToolExecutor;

/// Recognized image file extensions.
const IMAGE_EXTENSIONS: &[&str] = &["png", "jpg", "jpeg", "gif", "webp", "bmp", "ico"];

/// Check if a file extension is a recognized image type (not SVG — that's text).
fn is_image_extension(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| IMAGE_EXTENSIONS.contains(&ext.to_lowercase().as_str()))
        .unwrap_or(false)
}

/// Check if a file extension is SVG (text/XML, should be read normally).
fn is_svg(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.eq_ignore_ascii_case("svg"))
        .unwrap_or(false)
}

/// Arguments for the Read tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct ReadArgs {
    /// Absolute path to the file to read. Example: '/home/user/project/src/main.rs'
    #[schemars(description = "Absolute path to the file to read. Example: '/home/user/project/src/main.rs'")]
    pub file_path: String,
    /// Line number to start reading from (1-based). Use with limit for large files.
    #[schemars(description = "Line number to start reading from (1-based). Use with limit for large files")]
    pub offset: Option<usize>,
    /// Maximum number of lines to read. Default: 2000. Use with offset for pagination.
    #[schemars(description = "Maximum number of lines to read. Default: 2000. Use with offset for pagination")]
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
        include_str!("instructions/read.txt")
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

        // Check project boundary
        if let Some(ref boundary) = self.boundary {
            boundary.check(file_path)?;
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
        let path_obj = Path::new(file_path);
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
                // Binary file detected — check if it's a recognized image
                if is_image_extension(path_obj) {
                    // Read the full file and return as base64
                    let all_bytes = tokio::fs::read(file_path)
                        .await
                        .map_err(|e| format!("Failed to read image file: {e}"))?;

                    use base64::Engine;
                    let b64 = base64::engine::general_purpose::STANDARD.encode(&all_bytes);
                    let ext = path_obj
                        .extension()
                        .and_then(|e| e.to_str())
                        .unwrap_or("bin")
                        .to_lowercase();

                    // Mark image file as read
                    if let Some(ref tracker) = self.file_tracker {
                        tracker.mark_read(file_path);
                    }

                    let result = serde_json::json!({
                        "type": "image",
                        "format": ext,
                        "data": b64,
                        "size_bytes": all_bytes.len()
                    });
                    return serde_json::to_string(&result)
                        .map_err(|e| format!("Failed to serialize result: {e}"));
                }

                // SVG files are text/XML and should be handled by the normal read path below
                if !is_svg(path_obj) {
                    let file_size = metadata.len();
                    return Err(format!(
                        "Binary file detected ({file_size} bytes). Use the bash tool to inspect binary files."
                    ));
                }
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

        // Format lines as "{line_number}: {content}" (e.g., "1: foo")
        let mut content = String::new();
        for (i, line) in selected_lines.iter().enumerate() {
            let line_num = start + i + 1;
            let display_line = if line.len() > MAX_LINE_LENGTH {
                format!("{}...", &line[..MAX_LINE_LENGTH])
            } else {
                line.to_string()
            };
            content.push_str(&format!("{}: {}\n", line_num, display_line));
        }

        // Mark file as read for edit/write tracking
        if let Some(ref tracker) = self.file_tracker {
            tracker.mark_read(file_path);
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

    #[test]
    fn test_read_description_is_rich() {
        let tool = ReadTool::new();
        let desc = tool.description();
        assert!(!desc.is_empty());
        assert!(desc.contains("absolute path"), "should mention absolute path requirement");
        assert!(desc.contains("2000"), "should mention default line limit");
        assert!(desc.contains("image"), "should mention image support");
        assert!(desc.contains("grep"), "should mention cross-tool guidance");
    }

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
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("data.bin");
        std::fs::write(&file_path, &[0x00, 0x01, 0x02]).expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Binary file detected"));
        assert!(err.contains("bytes"), "should mention file size");
        assert!(err.contains("bash tool"), "should suggest bash tool");
    }

    #[tokio::test]
    async fn test_read_image_file_returns_base64() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("test.png");
        // Write some binary data with a null byte to trigger binary detection
        std::fs::write(&file_path, &[0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A])
            .expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("should succeed for image");
        let parsed: serde_json::Value = serde_json::from_str(&result).expect("parse");
        assert_eq!(parsed["type"], "image");
        assert_eq!(parsed["format"], "png");
        assert!(parsed["data"].is_string());
        assert!(parsed["size_bytes"].as_u64().unwrap() > 0);
    }

    #[tokio::test]
    async fn test_read_svg_as_text() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("icon.svg");
        std::fs::write(
            &file_path,
            r#"<svg xmlns="http://www.w3.org/2000/svg"><circle r="50"/></svg>"#,
        )
        .expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("should succeed for SVG");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");
        assert!(parsed.content.contains("<svg"));
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

        // Lines should be formatted as "N: content"
        assert!(parsed.content.contains("1: first"));
        assert!(parsed.content.contains("2: second"));
    }
}
