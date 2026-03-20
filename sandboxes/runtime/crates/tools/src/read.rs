use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use strsim::normalized_levenshtein;
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

/// Number of bytes to sample for binary content detection.
const BINARY_CHECK_SIZE: usize = 4096;

/// If more than 30% of sampled bytes are non-printable, treat as binary.
const BINARY_THRESHOLD: f64 = 0.30;

/// Known binary file extensions (skip content check).
const BINARY_EXTENSIONS: &[&str] = &[
    "zip", "tar", "gz", "exe", "dll", "so", "class", "jar", "war", "7z", "doc", "docx", "xls",
    "xlsx", "ppt", "pptx", "odt", "ods", "odp", "bin", "dat", "obj", "o", "a", "lib", "wasm",
    "pyc", "pyo",
];

fn is_binary_extension(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| BINARY_EXTENSIONS.contains(&ext.to_lowercase().as_str()))
        .unwrap_or(false)
}

/// Check if a byte is non-printable (control chars excluding common whitespace).
fn is_non_printable(b: u8) -> bool {
    b < 9 || (b > 13 && b < 32)
}

/// Maximum bytes to read from a text file.
const MAX_READ_BYTES: usize = 50 * 1024; // 50KB

/// Suggest similar filenames when a file is not found.
/// Scans the parent directory and returns up to 3 similar names.
fn suggest_similar_files(path: &str) -> Vec<String> {
    let path = Path::new(path);
    let parent = match path.parent() {
        Some(p) if p.exists() => p,
        _ => return vec![],
    };
    let target_name = match path.file_name().and_then(|n| n.to_str()) {
        Some(n) => n.to_lowercase(),
        None => return vec![],
    };

    let mut candidates: Vec<(String, f64)> = Vec::new();
    if let Ok(entries) = std::fs::read_dir(parent) {
        for entry in entries.flatten() {
            let name = entry.file_name();
            let name_str = name.to_string_lossy().to_string();
            let name_lower = name_str.to_lowercase();

            // Substring match or Levenshtein similarity
            let score = if name_lower.contains(&target_name) || target_name.contains(&name_lower) {
                0.8
            } else {
                normalized_levenshtein(&target_name, &name_lower)
            };

            if score > 0.4 {
                let full_path = parent.join(&name_str);
                candidates.push((full_path.to_string_lossy().to_string(), score));
            }
        }
    }

    candidates.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
    candidates
        .into_iter()
        .take(3)
        .map(|(path, _)| path)
        .collect()
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
    #[schemars(
        description = "Absolute path to the file to read. Example: '/home/user/project/src/main.rs'"
    )]
    pub file_path: String,
    /// Line number to start reading from (1-based). Use with limit for large files.
    #[schemars(
        description = "Line number to start reading from (1-based). Use with limit for large files"
    )]
    pub offset: Option<usize>,
    /// Maximum number of lines to read. Default: 2000. Use with offset for pagination.
    #[schemars(
        description = "Maximum number of lines to read. Default: 2000. Use with offset for pagination"
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

/// Maximum line length before truncation.
const MAX_LINE_LENGTH: usize = 2000;

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
                std::io::ErrorKind::NotFound => {
                    let suggestions = suggest_similar_files(file_path);
                    if suggestions.is_empty() {
                        format!("File not found: {file_path}")
                    } else {
                        format!(
                            "File not found: {}\n\nDid you mean one of these?\n{}",
                            file_path,
                            suggestions.join("\n")
                        )
                    }
                }
                std::io::ErrorKind::PermissionDenied => {
                    format!("Permission denied: {file_path}")
                }
                _ => format!("Failed to read file metadata: {e}"),
            })?;

        if metadata.is_dir() {
            // List directory entries
            let mut entries: Vec<String> = Vec::new();
            let mut read_dir = tokio::fs::read_dir(file_path)
                .await
                .map_err(|e| format!("Failed to read directory: {e}"))?;

            while let Some(entry) = read_dir
                .next_entry()
                .await
                .map_err(|e| format!("Failed to read entry: {e}"))?
            {
                let name = entry.file_name().to_string_lossy().to_string();
                let ft = entry.file_type().await.ok();
                let is_dir = ft.as_ref().is_some_and(|t| t.is_dir());
                let is_symlink = ft.as_ref().is_some_and(|t| t.is_symlink());

                if is_dir {
                    entries.push(format!("{}/", name));
                } else if is_symlink {
                    // Resolve symlink to check if target is a directory
                    let target_is_dir = tokio::fs::metadata(entry.path())
                        .await
                        .map(|m| m.is_dir())
                        .unwrap_or(false);
                    if target_is_dir {
                        entries.push(format!("{}/", name));
                    } else {
                        entries.push(name);
                    }
                } else {
                    entries.push(name);
                }
            }

            entries.sort_by_key(|a| a.to_lowercase());

            // Apply offset/limit pagination
            let start = if offset > 0 { offset - 1 } else { 0 };
            let total = entries.len();
            let end = total.min(start + limit);
            let selected = if start < total {
                &entries[start..end]
            } else {
                &[][..]
            };
            let truncated = end < total;

            let content = selected.join("\n");
            let result = ReadResult {
                content,
                total_lines: total,
                lines_read: selected.len(),
                truncated,
            };
            return serde_json::to_string(&result).map_err(|e| format!("Failed to serialize: {e}"));
        }

        let path_obj = Path::new(file_path);

        // Check for known binary extensions first (skip content check)
        if is_binary_extension(path_obj) && !is_image_extension(path_obj) && !is_svg(path_obj) {
            let file_size = metadata.len();
            return Err(format!(
                "Binary file detected ({file_size} bytes). Use the bash tool to inspect binary files."
            ));
        }

        // Check for PDF files
        let is_pdf = path_obj
            .extension()
            .and_then(|ext| ext.to_str())
            .map(|ext| ext.eq_ignore_ascii_case("pdf"))
            .unwrap_or(false);

        if is_pdf {
            let all_bytes = tokio::fs::read(file_path)
                .await
                .map_err(|e| format!("Failed to read PDF file: {e}"))?;
            use base64::Engine;
            let b64 = base64::engine::general_purpose::STANDARD.encode(&all_bytes);

            if let Some(ref tracker) = self.file_tracker {
                tracker.mark_read(file_path);
            }

            let result = serde_json::json!({
                "type": "file",
                "format": "pdf",
                "data": b64,
                "size_bytes": all_bytes.len()
            });
            return serde_json::to_string(&result)
                .map_err(|e| format!("Failed to serialize result: {e}"));
        }

        // Binary detection: sample bytes and check for null bytes or high non-printable ratio
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

            let sample = &buf[..bytes_read];
            let has_null = sample.contains(&0);
            let non_printable_count = sample.iter().filter(|&&b| is_non_printable(b)).count();
            let non_printable_ratio = if bytes_read > 0 {
                non_printable_count as f64 / bytes_read as f64
            } else {
                0.0
            };

            let is_binary = has_null || non_printable_ratio > BINARY_THRESHOLD;

            if is_binary {
                // Check if it's a recognized image
                if is_image_extension(path_obj) {
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

                // SVG files are text/XML
                if !is_svg(path_obj) {
                    let file_size = metadata.len();
                    return Err(format!(
                        "Binary file detected ({file_size} bytes). Use the bash tool to inspect binary files."
                    ));
                }
            }
        }

        // Read lines using async BufReader with 50KB byte cap
        let file = tokio::fs::File::open(file_path)
            .await
            .map_err(|e| format!("Failed to open file: {e}"))?;
        let reader = BufReader::new(file);
        let mut lines_stream = reader.lines();

        let mut all_lines: Vec<String> = Vec::new();
        let mut byte_count: usize = 0;
        let mut truncated_by_bytes = false;
        while let Some(line) = lines_stream
            .next_line()
            .await
            .map_err(|e| format!("Failed to read line: {e}"))?
        {
            byte_count += line.len() + 1; // +1 for newline
            if byte_count > MAX_READ_BYTES {
                truncated_by_bytes = true;
                break;
            }
            all_lines.push(line);
        }

        let total_lines = all_lines.len();

        // Check for out-of-range offset (special case: offset=1 on empty file is OK)
        if offset > total_lines && !(total_lines == 0 && offset == 1) {
            return Err(format!(
                "Offset {offset} is out of range for this file ({total_lines} lines)"
            ));
        }

        // Apply offset (1-indexed) and limit
        let start = if offset > 0 { offset - 1 } else { 0 };
        let end = total_lines.min(start + limit);
        let selected_lines = if start < total_lines {
            &all_lines[start..end]
        } else {
            &[]
        };

        let lines_read = selected_lines.len();
        let truncated = end < total_lines || truncated_by_bytes;

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

    fn as_any(&self) -> &dyn std::any::Any {
        self
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
        assert!(
            desc.contains("absolute path"),
            "should mention absolute path requirement"
        );
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
    async fn test_read_directory_lists_entries() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let dir_path = dir.path();

        // Create files and a subdirectory
        std::fs::write(dir_path.join("file_a.txt"), "a").expect("write");
        std::fs::write(dir_path.join("file_b.txt"), "b").expect("write");
        std::fs::create_dir(dir_path.join("subdir")).expect("mkdir");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_lines, 3);
        assert!(parsed.content.contains("file_a.txt"));
        assert!(parsed.content.contains("file_b.txt"));
        assert!(parsed.content.contains("subdir/"));
    }

    #[tokio::test]
    async fn test_read_directory_pagination() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let dir_path = dir.path();

        for i in 0..10 {
            std::fs::write(dir_path.join(format!("file_{:02}.txt", i)), "x").expect("write");
        }

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": dir_path.to_str().unwrap(),
            "offset": 3,
            "limit": 2
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_lines, 10);
        assert_eq!(parsed.lines_read, 2);
        assert!(parsed.truncated);
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
    async fn test_read_not_found_suggests_similar_files() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let existing = dir.path().join("foo.rs");
        std::fs::write(&existing, "content").expect("write");

        let tool = ReadTool::new();
        // Try to read "fo.rs" which is similar to "foo.rs"
        let args = serde_json::json!({
            "file_path": dir.path().join("fo.rs").to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(
            err.contains("Did you mean"),
            "should suggest similar files: {err}"
        );
        assert!(err.contains("foo.rs"), "should suggest foo.rs: {err}");
    }

    #[tokio::test]
    async fn test_read_not_found_no_suggestions_when_dir_empty() {
        let dir = tempfile::tempdir().expect("create temp dir");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": dir.path().join("nonexistent.txt").to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("not found"));
        assert!(
            !err.contains("Did you mean"),
            "should not suggest when dir is empty"
        );
    }

    #[tokio::test]
    async fn test_read_not_found_no_suggestions_when_parent_missing() {
        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": "/tmp/nonexistent_parent_dir_xyz/file.txt"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("not found"));
        assert!(
            !err.contains("Did you mean"),
            "should not suggest when parent is missing"
        );
    }

    #[tokio::test]
    async fn test_read_binary_file_error() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("data.bin");
        std::fs::write(&file_path, [0x00, 0x01, 0x02]).expect("write");

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
    async fn test_read_binary_extension_detected() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("program.exe");
        // Write normal text — extension alone should trigger binary detection
        std::fs::write(&file_path, "this is text content").expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Binary file detected"));
    }

    #[tokio::test]
    async fn test_read_binary_threshold_detection() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("data.dat2");
        // Create file with >30% non-printable bytes (but no null bytes)
        let mut bytes = Vec::new();
        for i in 0u8..100 {
            if i < 35 {
                bytes.push(1); // non-printable (SOH)
            } else {
                bytes.push(b'A'); // printable
            }
        }
        std::fs::write(&file_path, &bytes).expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Binary file detected"));
    }

    #[tokio::test]
    async fn test_read_pdf_returns_base64() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("test.pdf");
        std::fs::write(&file_path, b"%PDF-1.4 fake pdf content").expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("should succeed for PDF");
        let parsed: serde_json::Value = serde_json::from_str(&result).expect("parse");
        assert_eq!(parsed["type"], "file");
        assert_eq!(parsed["format"], "pdf");
        assert!(parsed["data"].is_string());
        assert!(parsed["size_bytes"].as_u64().unwrap() > 0);
    }

    #[tokio::test]
    async fn test_read_byte_cap_truncation() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("large.txt");
        // Create a file larger than 50KB
        let big_content = "x".repeat(100) + "\n";
        let repeated = big_content.repeat(600); // ~60KB
        std::fs::write(&file_path, &repeated).expect("write");

        let tool = ReadTool::new();
        let args = serde_json::json!({
            "file_path": file_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ReadResult = serde_json::from_str(&result).expect("parse");

        assert!(parsed.truncated, "should be truncated by byte cap");
        // Should have read fewer than total lines
        assert!(parsed.lines_read < 600);
    }

    #[tokio::test]
    async fn test_read_image_file_returns_base64() {
        let dir = tempfile::tempdir().expect("create temp dir");
        let file_path = dir.path().join("test.png");
        // Write some binary data with a null byte to trigger binary detection
        std::fs::write(&file_path, [0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A]).expect("write");

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
