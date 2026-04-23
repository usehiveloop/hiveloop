use lsp_types::Location;
use std::path::{Path, PathBuf};

/// Convert a file path to a `file://` URI.
pub(super) fn path_to_uri(path: &Path) -> String {
    let abs = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir().unwrap_or_default().join(path)
    };
    format!("file://{}", abs.display())
}

/// Convert a `file://` URI back to a file path.
pub fn uri_to_path(uri: &str) -> Option<PathBuf> {
    uri.strip_prefix("file://").map(PathBuf::from)
}

/// Format a Location as a human-readable string.
pub fn format_location(loc: &Location) -> String {
    let path = uri_to_path(loc.uri.as_str())
        .map(|p| p.display().to_string())
        .unwrap_or_else(|| loc.uri.to_string());
    format!(
        "{}:{}:{}",
        path,
        loc.range.start.line + 1,
        loc.range.start.character + 1,
    )
}
