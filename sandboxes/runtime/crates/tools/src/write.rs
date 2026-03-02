use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;

use crate::ToolExecutor;

/// Arguments for the Write tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct WriteArgs {
    /// The absolute path to the file to write.
    pub file_path: String,
    /// The content to write to the file.
    pub content: String,
}

/// Result returned by the Write tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct WriteResult {
    pub path: String,
    pub bytes_written: usize,
    pub created: bool,
}

pub struct WriteTool;

impl WriteTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for WriteTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for WriteTool {
    fn name(&self) -> &str {
        "write"
    }

    fn description(&self) -> &str {
        include_str!("instructions/write.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WriteArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WriteArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let file_path = &args.file_path;
        let path = Path::new(file_path);

        // Check if file already exists
        let created = !path.exists();

        // Create parent directories if needed
        if let Some(parent) = path.parent() {
            if !parent.exists() {
                tokio::fs::create_dir_all(parent)
                    .await
                    .map_err(|e| format!("Failed to create parent directories: {e}"))?;
            }
        }

        let bytes_written = args.content.len();

        tokio::fs::write(file_path, &args.content)
            .await
            .map_err(|e| format!("Failed to write file: {e}"))?;

        let result = WriteResult {
            path: file_path.clone(),
            bytes_written,
            created,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test]
    async fn test_write_new_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("new_file.txt");

        let tool = WriteTool::new();
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
            "content": "hello world"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

        assert!(parsed.created);
        assert_eq!(parsed.bytes_written, 11);

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert_eq!(content, "hello world");
    }

    #[tokio::test]
    async fn test_write_overwrite_existing() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("existing.txt");
        std::fs::write(&file_path, "old content").expect("write");

        let tool = WriteTool::new();
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
            "content": "new content"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

        assert!(!parsed.created);
        assert_eq!(parsed.bytes_written, 11);

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert_eq!(content, "new content");
    }

    #[tokio::test]
    async fn test_write_creates_parent_dirs() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("a").join("b").join("c").join("deep.txt");

        let tool = WriteTool::new();
        let args = serde_json::json!({
            "filePath": file_path.to_str().unwrap(),
            "content": "deep content"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: WriteResult = serde_json::from_str(&result).expect("parse");

        assert!(parsed.created);
        let content = std::fs::read_to_string(&file_path).expect("read");
        assert_eq!(content, "deep content");
    }
}
