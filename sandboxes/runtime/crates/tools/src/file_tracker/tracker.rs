use dashmap::DashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, SystemTime};
use tokio::sync::Mutex;

/// Tracks which files have been read during a session, their modification times,
/// and provides per-file locking for concurrent edit safety.
///
/// Edit and Write tools check this tracker to ensure a file was read
/// before being modified, and that the file hasn't been modified externally
/// since it was last read (staleness detection).
#[derive(Clone)]
pub struct FileTracker {
    /// Maps canonical file path -> mtime at the time it was last read/written in this session.
    read_files: Arc<DashMap<PathBuf, SystemTime>>,
    /// Per-file async locks to serialize concurrent edits.
    file_locks: Arc<DashMap<PathBuf, Arc<Mutex<()>>>>,
}

/// 50ms tolerance for filesystem timestamp precision (NTFS, async flush).
const STALENESS_TOLERANCE: Duration = Duration::from_millis(50);

impl FileTracker {
    /// Create a new empty tracker.
    pub fn new() -> Self {
        Self {
            read_files: Arc::new(DashMap::new()),
            file_locks: Arc::new(DashMap::new()),
        }
    }

    /// Mark a file as having been read, recording its current mtime.
    pub fn mark_read(&self, path: &str) {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));
        // Record the file's current mtime (or now if we can't stat)
        let mtime = std::fs::metadata(&canonical)
            .and_then(|m| m.modified())
            .unwrap_or_else(|_| SystemTime::now());
        self.read_files.insert(canonical, mtime);
    }

    /// Update the tracked timestamp after a successful write/edit.
    /// This allows subsequent edits without requiring a re-read.
    pub fn mark_written(&self, path: &str) {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));
        // Re-stat the file to get the actual post-write mtime
        let mtime = std::fs::metadata(&canonical)
            .and_then(|m| m.modified())
            .unwrap_or_else(|_| SystemTime::now());
        self.read_files.insert(canonical, mtime);
    }

    /// Check that a file has been read and hasn't been modified externally.
    /// Returns an error if the file was never read or if it's stale.
    pub fn assert_not_stale(&self, path: &str) -> Result<(), String> {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));

        let recorded_mtime = match self.read_files.get(&canonical) {
            Some(entry) => *entry.value(),
            None => {
                return Err(format!(
                    "File '{}' must be read before it can be edited or written. Use the Read tool first.",
                    path
                ));
            }
        };

        // Check current mtime on disk
        let current_mtime = match std::fs::metadata(&canonical).and_then(|m| m.modified()) {
            Ok(mtime) => mtime,
            Err(_) => return Ok(()), // File deleted or inaccessible — let the write fail naturally
        };

        if current_mtime > recorded_mtime + STALENESS_TOLERANCE {
            return Err(format!(
                "File '{}' has been modified since it was last read. Please read the file again before modifying it.",
                path
            ));
        }

        Ok(())
    }

    /// Check if a file was previously read.
    pub fn was_read(&self, path: &str) -> bool {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));
        self.read_files.contains_key(&canonical)
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

    /// Execute an async closure while holding the per-file lock.
    /// This serializes concurrent edits to the same file.
    pub async fn with_lock<F, Fut, T>(&self, path: &str, f: F) -> T
    where
        F: FnOnce() -> Fut,
        Fut: std::future::Future<Output = T>,
    {
        let canonical = Path::new(path)
            .canonicalize()
            .unwrap_or_else(|_| PathBuf::from(path));

        let lock = self
            .file_locks
            .entry(canonical)
            .or_insert_with(|| Arc::new(Mutex::new(())))
            .clone();

        let _guard = lock.lock().await;
        f().await
    }
}

impl Default for FileTracker {
    fn default() -> Self {
        Self::new()
    }
}
