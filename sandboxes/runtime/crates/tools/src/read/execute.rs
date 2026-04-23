//! Helpers that implement the steps of `ReadTool::execute`, preserving the
//! original error paths and behavior exactly.

use std::path::Path;
use tokio::io::AsyncReadExt;

use super::helpers::{
    is_binary_extension, is_image_extension, is_non_printable, is_svg, suggest_similar_files,
    BINARY_CHECK_SIZE, BINARY_THRESHOLD,
};
use super::{ReadResult, DEFAULT_LINE_LIMIT};
use crate::file_tracker::FileTracker;

/// Fetch `fs::metadata` with the exact error-message shape the tool has always
/// produced (including not-found "Did you mean" suggestions).
pub(super) async fn fetch_metadata(file_path: &str) -> Result<std::fs::Metadata, String> {
    tokio::fs::metadata(file_path)
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
        })
}

/// List a directory, applying the default-limit hard gate and pagination.
pub(super) async fn read_directory(
    file_path: &str,
    offset: usize,
    limit: usize,
    limit_explicit: bool,
) -> Result<String, String> {
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

    // Hard gate: require explicit limit when the directory has more
    // entries than the default cap. Same three-option steer as files.
    let total = entries.len();
    if !limit_explicit && total > DEFAULT_LINE_LIMIT {
        return Err(format!(
            "Directory {} has {} entries (default limit: {}). Choose one:\n\
            - Ranged read: call Read again with explicit offset and limit (e.g., offset=1, limit=500)\n\
            - Narrow: call Glob with pattern=\"...\" and path=\"{}\" to match only what you need\n\
            - Summarize: spawn self_agent with a focused question so the listing stays out of this context",
            file_path, total, DEFAULT_LINE_LIMIT, file_path
        ));
    }

    // Apply offset/limit pagination
    let start = if offset > 0 { offset - 1 } else { 0 };
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
    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize: {e}"))
}

/// Read a PDF file and return it as a base64-encoded payload.
pub(super) async fn read_pdf(
    file_path: &str,
    tracker: &Option<FileTracker>,
) -> Result<String, String> {
    let all_bytes = tokio::fs::read(file_path)
        .await
        .map_err(|e| format!("Failed to read PDF file: {e}"))?;
    use base64::Engine;
    let b64 = base64::engine::general_purpose::STANDARD.encode(&all_bytes);

    if let Some(ref t) = tracker {
        t.mark_read(file_path);
    }

    let result = serde_json::json!({
        "type": "file",
        "format": "pdf",
        "data": b64,
        "size_bytes": all_bytes.len()
    });
    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
}

/// Read a recognized image file and return it as a base64-encoded payload.
pub(super) async fn read_image(
    file_path: &str,
    path_obj: &Path,
    tracker: &Option<FileTracker>,
) -> Result<String, String> {
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

    if let Some(ref t) = tracker {
        t.mark_read(file_path);
    }

    let result = serde_json::json!({
        "type": "image",
        "format": ext,
        "data": b64,
        "size_bytes": all_bytes.len()
    });
    serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
}

/// Outcome of content-based binary detection.
pub(super) enum BinaryDetection {
    /// File looks textual, proceed with normal read.
    Text,
    /// File is an image — caller should dispatch to `read_image`.
    Image,
    /// File is binary and not a recognized image or SVG. Error string included.
    Reject(String),
    /// File is SVG which passes the "is binary" test but should be read as text.
    Svg,
}

/// Content-based binary detection: sample bytes and classify the file.
pub(super) async fn detect_binary(
    file_path: &str,
    path_obj: &Path,
    metadata_len: u64,
) -> Result<BinaryDetection, String> {
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
        if is_image_extension(path_obj) {
            return Ok(BinaryDetection::Image);
        }
        if is_svg(path_obj) {
            return Ok(BinaryDetection::Svg);
        }
        return Ok(BinaryDetection::Reject(format!(
            "Binary file detected ({metadata_len} bytes). Use the bash tool to inspect binary files."
        )));
    }

    Ok(BinaryDetection::Text)
}

/// Reject files with a known binary extension (unless they're an image/SVG).
pub(super) fn reject_binary_extension(
    path_obj: &Path,
    metadata_len: u64,
) -> Option<Result<String, String>> {
    if is_binary_extension(path_obj) && !is_image_extension(path_obj) && !is_svg(path_obj) {
        return Some(Err(format!(
            "Binary file detected ({metadata_len} bytes). Use the bash tool to inspect binary files."
        )));
    }
    None
}

/// Check whether `path` is a `.pdf`.
pub(super) fn is_pdf(path_obj: &Path) -> bool {
    path_obj
        .extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.eq_ignore_ascii_case("pdf"))
        .unwrap_or(false)
}
