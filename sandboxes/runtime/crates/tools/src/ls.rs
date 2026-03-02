use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::SystemTime;

use crate::ToolExecutor;

/// Arguments for the LS tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct LsArgs {
    /// The absolute path of the directory to list.
    pub path: String,
}

/// The type of a directory entry.
#[derive(Debug, Serialize, Deserialize, PartialEq, Eq, PartialOrd, Ord, Clone)]
#[serde(rename_all = "lowercase")]
pub enum EntryType {
    Directory,
    File,
    Symlink,
}

/// A single directory entry.
#[derive(Debug, Serialize, Deserialize)]
pub struct LsEntry {
    pub name: String,
    pub entry_type: EntryType,
    pub size: Option<u64>,
    pub modified: Option<String>,
}

/// Result returned by the LS tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct LsResult {
    pub entries: Vec<LsEntry>,
    pub total_entries: usize,
}

/// Maximum number of entries to return.
const MAX_ENTRIES: usize = 1000;

pub struct LsTool;

impl LsTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for LsTool {
    fn default() -> Self {
        Self::new()
    }
}

fn format_system_time(time: SystemTime) -> Option<String> {
    time.duration_since(SystemTime::UNIX_EPOCH)
        .ok()
        .and_then(|d| {
            chrono::DateTime::from_timestamp(d.as_secs() as i64, d.subsec_nanos())
                .map(|dt| dt.to_rfc3339())
        })
}

#[async_trait]
impl ToolExecutor for LsTool {
    fn name(&self) -> &str {
        "LS"
    }

    fn description(&self) -> &str {
        "Lists the contents of a directory. Directories are listed first, then files, sorted alphabetically within each group."
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(LsArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: LsArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let dir_path = &args.path;
        let path = Path::new(dir_path);

        if !path.exists() {
            return Err(format!("Path does not exist: {dir_path}"));
        }

        if !path.is_dir() {
            return Err(format!("Not a directory: {dir_path}"));
        }

        let mut read_dir = tokio::fs::read_dir(dir_path)
            .await
            .map_err(|e| match e.kind() {
                std::io::ErrorKind::PermissionDenied => {
                    format!("Permission denied: {dir_path}")
                }
                _ => format!("Failed to read directory: {e}"),
            })?;

        let mut entries: Vec<LsEntry> = Vec::new();

        while let Some(entry) = read_dir
            .next_entry()
            .await
            .map_err(|e| format!("Failed to read directory entry: {e}"))?
        {
            let name = entry.file_name().to_string_lossy().to_string();

            let metadata = match entry.metadata().await {
                Ok(m) => m,
                Err(_) => continue,
            };

            let file_type = entry.file_type().await.ok();

            let entry_type = if file_type.as_ref().is_some_and(|ft| ft.is_symlink()) {
                EntryType::Symlink
            } else if metadata.is_dir() {
                EntryType::Directory
            } else {
                EntryType::File
            };

            let size = if entry_type == EntryType::File {
                Some(metadata.len())
            } else {
                None
            };

            let modified = metadata.modified().ok().and_then(format_system_time);

            entries.push(LsEntry {
                name,
                entry_type,
                size,
                modified,
            });
        }

        // Sort: directories first, then files, alphabetically within each group.
        // Symlinks sort alongside files.
        entries.sort_by(|a, b| {
            let type_order = |t: &EntryType| -> u8 {
                match t {
                    EntryType::Directory => 0,
                    EntryType::File | EntryType::Symlink => 1,
                }
            };
            let ord = type_order(&a.entry_type).cmp(&type_order(&b.entry_type));
            if ord == std::cmp::Ordering::Equal {
                a.name.to_lowercase().cmp(&b.name.to_lowercase())
            } else {
                ord
            }
        });

        let total_entries = entries.len();
        entries.truncate(MAX_ENTRIES);

        let result = LsResult {
            entries,
            total_entries,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_ls_basic_listing() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("file_a.txt"), "a").expect("write");
        fs::write(dir_path.join("file_b.txt"), "b").expect("write");
        fs::create_dir(dir_path.join("subdir")).expect("mkdir");

        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: LsResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_entries, 3);
        // Directory should come first
        assert_eq!(parsed.entries[0].name, "subdir");
        assert_eq!(parsed.entries[0].entry_type, EntryType::Directory);
        // Then files alphabetically
        assert_eq!(parsed.entries[1].name, "file_a.txt");
        assert_eq!(parsed.entries[2].name, "file_b.txt");
    }

    #[tokio::test]
    async fn test_ls_directories_first() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("aaa.txt"), "a").expect("write");
        fs::create_dir(dir_path.join("zzz_dir")).expect("mkdir");
        fs::create_dir(dir_path.join("aaa_dir")).expect("mkdir");

        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: LsResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_entries, 3);
        // Directories come first, alphabetically
        assert_eq!(parsed.entries[0].name, "aaa_dir");
        assert_eq!(parsed.entries[0].entry_type, EntryType::Directory);
        assert_eq!(parsed.entries[1].name, "zzz_dir");
        assert_eq!(parsed.entries[1].entry_type, EntryType::Directory);
        // Then files
        assert_eq!(parsed.entries[2].name, "aaa.txt");
        assert_eq!(parsed.entries[2].entry_type, EntryType::File);
    }

    #[tokio::test]
    async fn test_ls_empty_directory() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: LsResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.total_entries, 0);
        assert!(parsed.entries.is_empty());
    }

    #[tokio::test]
    async fn test_ls_nonexistent_path() {
        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": "/tmp/nonexistent_ls_test_xyz"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("does not exist"));
    }

    #[tokio::test]
    async fn test_ls_not_a_directory() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("file.txt");
        fs::write(&file_path, "hello").expect("write");

        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": file_path.to_str().unwrap()
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("Not a directory"));
    }

    #[tokio::test]
    async fn test_ls_file_has_size() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::write(dir_path.join("test.txt"), "hello world").expect("write");

        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: LsResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.entries.len(), 1);
        assert_eq!(parsed.entries[0].name, "test.txt");
        assert_eq!(parsed.entries[0].size, Some(11)); // "hello world" = 11 bytes
    }

    #[tokio::test]
    async fn test_ls_directory_has_no_size() {
        let dir = tempdir().expect("create temp dir");
        let dir_path = dir.path();

        fs::create_dir(dir_path.join("subdir")).expect("mkdir");

        let tool = LsTool::new();
        let args = serde_json::json!({
            "path": dir_path.to_str().unwrap()
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: LsResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.entries.len(), 1);
        assert_eq!(parsed.entries[0].name, "subdir");
        assert!(parsed.entries[0].size.is_none());
    }
}
