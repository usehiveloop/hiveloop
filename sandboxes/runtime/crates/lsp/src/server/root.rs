use std::path::{Path, PathBuf};

/// Walk up from `file` looking for any of the given marker files/directories.
/// Returns the directory containing the first marker found, or `None`.
pub fn find_root(file: &Path, markers: &[String]) -> Option<PathBuf> {
    let mut dir = if file.is_file() {
        file.parent()?.to_path_buf()
    } else {
        file.to_path_buf()
    };

    loop {
        for marker in markers {
            if dir.join(marker).exists() {
                return Some(dir);
            }
        }
        if !dir.pop() {
            return None;
        }
    }
}
