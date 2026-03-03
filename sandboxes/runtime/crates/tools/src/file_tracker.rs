use dashmap::DashSet;
use std::path::{Path, PathBuf};
use std::sync::Arc;

/// Tracks which files have been read during a session.
///
/// Edit and Write tools check this tracker to ensure a file was read
/// before being modified, preventing blind edits that corrupt files.
#[derive(Clone)]
pub struct FileTracker {
    read_files: Arc<DashSet<PathBuf>>,
}

impl FileTracker {
    /// Create a new empty tracker.
    pub fn new() -> Self {
        Self {
            read_files: Arc::new(DashSet::new()),
        }
    }

    /// Mark a file as having been read.
    pub fn mark_read(&self, path: &str) {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));
        self.read_files.insert(canonical);
    }

    /// Check if a file was previously read.
    pub fn was_read(&self, path: &str) -> bool {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));
        self.read_files.contains(&canonical)
    }

    /// Return error message if the file was not read.
    pub fn require_read(&self, path: &str) -> Result<(), String> {
        if self.was_read(path) {
            Ok(())
        } else {
            Err(format!(
                "File '{}' must be read before it can be edited or written. Use the Read tool first.",
                path
            ))
        }
    }
}

impl Default for FileTracker {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn test_mark_and_check() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("test.txt");
        fs::write(&file_path, "hello").expect("write");

        let tracker = FileTracker::new();
        let path_str = file_path.to_str().unwrap();

        assert!(!tracker.was_read(path_str));
        tracker.mark_read(path_str);
        assert!(tracker.was_read(path_str));
    }

    #[test]
    fn test_require_read_fails_when_not_read() {
        let tracker = FileTracker::new();
        let result = tracker.require_read("/some/path/file.txt");
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("must be read before"));
    }

    #[test]
    fn test_require_read_succeeds_after_read() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("test.txt");
        fs::write(&file_path, "hello").expect("write");

        let tracker = FileTracker::new();
        let path_str = file_path.to_str().unwrap();

        tracker.mark_read(path_str);
        assert!(tracker.require_read(path_str).is_ok());
    }

    #[test]
    fn test_tracker_is_shared_across_clones() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("shared.txt");
        fs::write(&file_path, "hello").expect("write");

        let tracker1 = FileTracker::new();
        let tracker2 = tracker1.clone();
        let path_str = file_path.to_str().unwrap();

        tracker1.mark_read(path_str);
        assert!(tracker2.was_read(path_str));
    }
}
