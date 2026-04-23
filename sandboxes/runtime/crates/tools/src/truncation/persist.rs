use std::path::PathBuf;

/// Directory for persisted tool output files.
pub(super) fn output_dir() -> PathBuf {
    let dir = std::env::temp_dir().join("bridge_tool_output");
    let _ = std::fs::create_dir_all(&dir);
    dir
}

/// Persist full text to disk and return the file path.
pub(super) fn persist_full_output(text: &str) -> Option<String> {
    let path = output_dir().join(format!("{}.txt", uuid::Uuid::new_v4()));
    std::fs::write(&path, text).ok()?;
    Some(path.to_string_lossy().to_string())
}

/// Clean up output files older than 7 days.
pub fn cleanup_old_outputs() {
    let retention = std::time::Duration::from_secs(7 * 24 * 60 * 60);
    let cutoff = std::time::SystemTime::now() - retention;
    if let Ok(entries) = std::fs::read_dir(output_dir()) {
        for entry in entries.flatten() {
            if let Ok(metadata) = entry.metadata() {
                if let Ok(modified) = metadata.modified() {
                    if modified < cutoff {
                        let _ = std::fs::remove_file(entry.path());
                    }
                }
            }
        }
    }
}
