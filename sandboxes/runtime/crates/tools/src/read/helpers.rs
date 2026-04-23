//! Support helpers for the Read tool: extension classification, binary
//! detection, and suggest-similar-files for not-found errors.

use std::path::Path;
use strsim::normalized_levenshtein;

/// Recognized image file extensions.
pub(super) const IMAGE_EXTENSIONS: &[&str] = &["png", "jpg", "jpeg", "gif", "webp", "bmp", "ico"];

/// Check if a file extension is a recognized image type (not SVG — that's text).
pub(super) fn is_image_extension(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| IMAGE_EXTENSIONS.contains(&ext.to_lowercase().as_str()))
        .unwrap_or(false)
}

/// Number of bytes to sample for binary content detection.
pub(super) const BINARY_CHECK_SIZE: usize = 4096;

/// If more than 30% of sampled bytes are non-printable, treat as binary.
pub(super) const BINARY_THRESHOLD: f64 = 0.30;

/// Known binary file extensions (skip content check).
pub(super) const BINARY_EXTENSIONS: &[&str] = &[
    "zip", "tar", "gz", "exe", "dll", "so", "class", "jar", "war", "7z", "doc", "docx", "xls",
    "xlsx", "ppt", "pptx", "odt", "ods", "odp", "bin", "dat", "obj", "o", "a", "lib", "wasm",
    "pyc", "pyo",
];

pub(super) fn is_binary_extension(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| BINARY_EXTENSIONS.contains(&ext.to_lowercase().as_str()))
        .unwrap_or(false)
}

/// Check if a byte is non-printable (control chars excluding common whitespace).
pub(super) fn is_non_printable(b: u8) -> bool {
    b < 9 || (b > 13 && b < 32)
}

/// Suggest similar filenames when a file is not found.
/// Scans the parent directory and returns up to 3 similar names.
pub(super) fn suggest_similar_files(path: &str) -> Vec<String> {
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
pub(super) fn is_svg(path: &Path) -> bool {
    path.extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.eq_ignore_ascii_case("svg"))
        .unwrap_or(false)
}
