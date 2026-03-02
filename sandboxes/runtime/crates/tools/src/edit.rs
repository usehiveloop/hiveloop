use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::ToolExecutor;

/// Arguments for the Edit tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct EditArgs {
    /// The absolute path to the file to modify.
    pub file_path: String,
    /// The text to find and replace.
    pub old_string: String,
    /// The replacement text.
    pub new_string: String,
    /// If true, replace all occurrences of oldString. Defaults to false.
    pub replace_all: Option<bool>,
}

/// Result returned by the Edit tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct EditResult {
    pub path: String,
    pub old_content_snippet: String,
    pub new_content_snippet: String,
    pub replacements_made: usize,
}

/// Shared edit logic used by both Edit and MultiEdit tools.
///
/// Applies a single find-and-replace operation on `content`.
/// Returns the new content on success.
pub(crate) fn apply_edit(
    content: &str,
    old_string: &str,
    new_string: &str,
    replace_all: bool,
) -> Result<(String, usize), String> {
    if old_string == new_string {
        return Err("oldString and newString are identical".to_string());
    }

    // Strategy 1: exact match
    let count = content.matches(old_string).count();

    if count > 0 {
        if count > 1 && !replace_all {
            return Err(
                "Found multiple matches for oldString. Provide more surrounding lines in oldString to identify the correct match, or use replaceAll.".to_string()
            );
        }
        if replace_all {
            let new_content = content.replace(old_string, new_string);
            return Ok((new_content, count));
        } else {
            // Replace only the first occurrence
            let new_content = content.replacen(old_string, new_string, 1);
            return Ok((new_content, 1));
        }
    }

    // Strategy 2: trimmed whitespace match
    // Trim each line and compare
    let old_lines: Vec<&str> = old_string.lines().map(|l| l.trim()).collect();
    let content_lines: Vec<&str> = content.lines().collect();
    let content_trimmed: Vec<&str> = content_lines.iter().map(|l| l.trim()).collect();

    if old_lines.is_empty() {
        return Err("oldString not found in file content".to_string());
    }

    let mut matches: Vec<usize> = Vec::new();
    for i in 0..=content_trimmed.len().saturating_sub(old_lines.len()) {
        if content_trimmed[i..i + old_lines.len()] == old_lines[..] {
            matches.push(i);
        }
    }

    if matches.is_empty() {
        return Err("oldString not found in file content".to_string());
    }

    if matches.len() > 1 && !replace_all {
        return Err(
            "Found multiple matches for oldString. Provide more surrounding lines in oldString to identify the correct match, or use replaceAll.".to_string()
        );
    }

    // Replace matches in reverse order to preserve indices
    let mut result_lines: Vec<&str> = content_lines.clone();
    let new_lines_vec: Vec<&str> = new_string.lines().collect();

    let matches_to_apply = if replace_all {
        matches.clone()
    } else {
        vec![matches[0]]
    };

    // Apply in reverse order
    let mut sorted_matches = matches_to_apply.clone();
    sorted_matches.sort_unstable_by(|a, b| b.cmp(a));

    for start in &sorted_matches {
        let end = start + old_lines.len();
        let mut new_result: Vec<&str> = Vec::new();
        new_result.extend_from_slice(&result_lines[..* start]);
        new_result.extend_from_slice(&new_lines_vec);
        new_result.extend_from_slice(&result_lines[end..]);
        result_lines = new_result;
    }

    let new_content = result_lines.join("\n");
    // Preserve trailing newline if original had one
    let new_content = if content.ends_with('\n') && !new_content.ends_with('\n') {
        format!("{new_content}\n")
    } else {
        new_content
    };

    Ok((new_content, matches_to_apply.len()))
}

fn snippet(s: &str, max_len: usize) -> String {
    if s.len() <= max_len {
        s.to_string()
    } else {
        format!("{}...", &s[..max_len])
    }
}

pub struct EditTool;

impl EditTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for EditTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for EditTool {
    fn name(&self) -> &str {
        "edit"
    }

    fn description(&self) -> &str {
        include_str!("instructions/edit.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(EditArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: EditArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let file_path = &args.file_path;
        let replace_all = args.replace_all.unwrap_or(false);

        let content = tokio::fs::read_to_string(file_path)
            .await
            .map_err(|e| match e.kind() {
                std::io::ErrorKind::NotFound => format!("File not found: {file_path}"),
                std::io::ErrorKind::PermissionDenied => {
                    format!("Permission denied: {file_path}")
                }
                _ => format!("Failed to read file: {e}"),
            })?;

        let (new_content, replacements_made) =
            apply_edit(&content, &args.old_string, &args.new_string, replace_all)?;

        tokio::fs::write(file_path, &new_content)
            .await
            .map_err(|e| format!("Failed to write file: {e}"))?;

        let result = EditResult {
            path: file_path.clone(),
            old_content_snippet: snippet(&args.old_string, 200),
            new_content_snippet: snippet(&args.new_string, 200),
            replacements_made,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write as IoWrite;
    use tempfile::NamedTempFile;

    #[test]
    fn test_apply_edit_exact_match() {
        let content = "hello world\nfoo bar\nbaz qux\n";
        let (result, count) = apply_edit(content, "foo bar", "foo replaced", false).unwrap();
        assert!(result.contains("foo replaced"));
        assert!(!result.contains("foo bar"));
        assert_eq!(count, 1);
    }

    #[test]
    fn test_apply_edit_not_found() {
        let content = "hello world\n";
        let err = apply_edit(content, "not here", "replacement", false).unwrap_err();
        assert!(err.contains("not found"));
    }

    #[test]
    fn test_apply_edit_multiple_matches_no_replace_all() {
        let content = "aaa\nbbb\naaa\n";
        let err = apply_edit(content, "aaa", "ccc", false).unwrap_err();
        assert!(err.contains("multiple matches"));
    }

    #[test]
    fn test_apply_edit_replace_all() {
        let content = "aaa\nbbb\naaa\n";
        let (result, count) = apply_edit(content, "aaa", "ccc", true).unwrap();
        assert_eq!(result.matches("ccc").count(), 2);
        assert_eq!(count, 2);
    }

    #[test]
    fn test_apply_edit_identical_strings() {
        let content = "hello\n";
        let err = apply_edit(content, "hello", "hello", false).unwrap_err();
        assert!(err.contains("identical"));
    }

    #[tokio::test]
    async fn test_edit_tool_execute() {
        let mut tmp = NamedTempFile::new().expect("create temp file");
        write!(tmp, "line one\nline two\nline three\n").expect("write");

        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": tmp.path().to_str().unwrap(),
            "oldString": "line two",
            "newString": "line TWO"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: EditResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.replacements_made, 1);

        // Verify file was actually written
        let content = std::fs::read_to_string(tmp.path()).expect("read");
        assert!(content.contains("line TWO"));
        assert!(!content.contains("line two"));
    }

    #[tokio::test]
    async fn test_edit_tool_not_found_file() {
        let tool = EditTool::new();
        let args = serde_json::json!({
            "filePath": "/tmp/nonexistent_edit_test_xyz.txt",
            "oldString": "foo",
            "newString": "bar"
        });

        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("not found") || err.contains("Not found"));
    }
}
