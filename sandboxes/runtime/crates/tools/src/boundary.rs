use std::path::{Path, PathBuf};

/// Path boundary for file operations.
///
/// Sandboxing is disabled — agents are allowed to read and write files
/// anywhere on the host. The struct is kept so the builder API
/// (`.with_boundary(…)`) continues to compile without changes.
#[derive(Clone)]
pub struct ProjectBoundary {
    root: PathBuf,
    disabled: bool,
}

impl ProjectBoundary {
    /// Create a new boundary rooted at the given path.
    ///
    /// Sandboxing is always disabled; agents may access any path.
    pub fn new(root: PathBuf) -> Self {
        let root = root.canonicalize().unwrap_or(root);
        Self {
            root,
            disabled: true,
        }
    }

    /// Check if a path is within the project root.
    ///
    /// Returns the canonical path on success, or an error message if the path
    /// escapes the boundary.
    pub fn check(&self, path: &str) -> Result<PathBuf, String> {
        if self.disabled {
            return Ok(PathBuf::from(path));
        }

        let p = Path::new(path);

        // Try to canonicalize; if the file doesn't exist yet, canonicalize
        // as much of the parent as possible and append the filename
        let canonical = if p.exists() {
            p.canonicalize()
                .map_err(|e| format!("Failed to resolve path '{}': {}", path, e))?
        } else {
            // For new files: canonicalize the parent, then append the filename
            let parent = p.parent().unwrap_or(Path::new("."));
            let parent_canonical = if parent.exists() {
                parent
                    .canonicalize()
                    .map_err(|e| format!("Failed to resolve parent of '{}': {}", path, e))?
            } else {
                // Parent doesn't exist either; just use the path as-is
                // (the tool will fail later when trying to write)
                return Ok(PathBuf::from(path));
            };
            parent_canonical.join(p.file_name().unwrap_or_default())
        };

        if canonical.starts_with(&self.root) {
            Ok(canonical)
        } else {
            Err(format!(
                "Access denied: '{}' is outside the project directory '{}'",
                path,
                self.root.display()
            ))
        }
    }

    /// Return the project root path.
    pub fn root(&self) -> &Path {
        &self.root
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn test_path_within_root_allowed() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("test.txt");
        fs::write(&file_path, "hello").expect("write");

        let boundary = ProjectBoundary::new(dir.path().to_path_buf());
        let result = boundary.check(file_path.to_str().unwrap());
        assert!(result.is_ok());
    }

    #[test]
    fn test_sandboxing_disabled_allows_any_path() {
        let dir = tempdir().expect("create temp dir");
        let boundary = ProjectBoundary::new(dir.path().to_path_buf());

        // Sandboxing is disabled — paths outside the root are allowed
        let result = boundary.check("/etc/passwd");
        assert!(result.is_ok());
    }

    #[test]
    fn test_new_file_within_root_allowed() {
        let dir = tempdir().expect("create temp dir");
        let boundary = ProjectBoundary::new(dir.path().to_path_buf());

        let new_file = dir.path().join("new_file.txt");
        let result = boundary.check(new_file.to_str().unwrap());
        assert!(result.is_ok());
    }

    #[test]
    fn test_boundary_always_disabled() {
        let dir = tempdir().expect("create temp dir");
        let boundary = ProjectBoundary::new(dir.path().to_path_buf());
        assert!(boundary.disabled, "sandboxing should always be disabled");
    }
}
