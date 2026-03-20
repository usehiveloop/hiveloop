use async_trait::async_trait;
use lsp::LspManager;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::sync::Arc;

use crate::ToolExecutor;

// ---------------------------------------------------------------------------
// Patch types
// ---------------------------------------------------------------------------

/// A single parsed hunk from the patch.
#[derive(Debug, Clone)]
enum Hunk {
    Add {
        path: String,
        contents: String,
    },
    Delete {
        path: String,
    },
    Update {
        path: String,
        move_path: Option<String>,
        chunks: Vec<UpdateChunk>,
    },
}

/// A chunk within an Update hunk.
#[derive(Debug, Clone)]
struct UpdateChunk {
    context: Option<String>,
    old_lines: Vec<String>,
    new_lines: Vec<String>,
    is_end_of_file: bool,
}

// ---------------------------------------------------------------------------
// Patch parser
// ---------------------------------------------------------------------------

fn strip_heredoc(input: &str) -> String {
    // Match heredoc patterns like: cat <<'EOF'\n...\nEOF or <<EOF\n...\nEOF
    // Can't use regex backreferences in Rust's regex crate, so parse manually.
    let trimmed = input.trim();
    let first_newline = match trimmed.find('\n') {
        Some(pos) => pos,
        None => return input.to_string(),
    };

    let first_line = &trimmed[..first_newline];

    // Check if first line matches: (cat )? << 'DELIM' or <<DELIM
    let rest = first_line.trim();
    let rest = rest.strip_prefix("cat").map_or(rest, |r| r.trim_start());
    if !rest.starts_with("<<") {
        return input.to_string();
    }
    let after_arrows = rest[2..].trim_start();

    // Strip optional quotes around delimiter
    let delimiter = after_arrows
        .trim_start_matches(['\'', '"'])
        .trim_end_matches(['\'', '"'])
        .trim();

    if delimiter.is_empty() {
        return input.to_string();
    }

    // Check if the last line matches the delimiter
    let body = &trimmed[first_newline + 1..];
    if let Some(last_newline) = body.rfind('\n') {
        let last_line = body[last_newline + 1..].trim();
        if last_line == delimiter {
            return body[..last_newline].to_string();
        }
    }

    input.to_string()
}

fn parse_patch(patch_text: &str) -> Result<Vec<Hunk>, String> {
    // Normalize CRLF and CR line endings to LF
    let patch_text = patch_text.replace("\r\n", "\n").replace('\r', "\n");
    let cleaned = strip_heredoc(patch_text.trim());
    let lines: Vec<&str> = cleaned.split('\n').collect();

    let begin_marker = "*** Begin Patch";
    let end_marker = "*** End Patch";

    let begin_idx = lines.iter().position(|l| l.trim() == begin_marker);
    let end_idx = lines.iter().position(|l| l.trim() == end_marker);

    let (begin_idx, end_idx) = match (begin_idx, end_idx) {
        (Some(b), Some(e)) if b < e => (b, e),
        _ => return Err("Invalid patch format: missing Begin/End markers".to_string()),
    };

    let mut hunks = Vec::new();
    let mut i = begin_idx + 1;

    while i < end_idx {
        let line = lines[i];

        if let Some(rest) = line.strip_prefix("*** Add File:") {
            let file_path = rest.trim().to_string();
            if file_path.is_empty() {
                return Err("Add File header has empty path".to_string());
            }
            i += 1;

            // Parse add content (+ prefixed lines)
            let mut content = String::new();
            while i < end_idx && !lines[i].starts_with("***") {
                if let Some(stripped) = lines[i].strip_prefix('+') {
                    if !content.is_empty() {
                        content.push('\n');
                    }
                    content.push_str(stripped);
                }
                i += 1;
            }

            hunks.push(Hunk::Add {
                path: file_path,
                contents: content,
            });
        } else if let Some(rest) = line.strip_prefix("*** Delete File:") {
            let file_path = rest.trim().to_string();
            if file_path.is_empty() {
                return Err("Delete File header has empty path".to_string());
            }
            hunks.push(Hunk::Delete { path: file_path });
            i += 1;
        } else if let Some(rest) = line.strip_prefix("*** Update File:") {
            let file_path = rest.trim().to_string();
            if file_path.is_empty() {
                return Err("Update File header has empty path".to_string());
            }
            i += 1;

            // Check for move directive
            let mut move_path = None;
            if i < end_idx && lines[i].starts_with("*** Move to:") {
                move_path = Some(lines[i]["*** Move to:".len()..].trim().to_string());
                i += 1;
            }

            // Parse update chunks
            let mut chunks = Vec::new();
            while i < end_idx && !lines[i].starts_with("***") {
                if lines[i].starts_with("@@") {
                    let context_line = lines[i][2..].trim().to_string();
                    let context = if context_line.is_empty() {
                        None
                    } else {
                        Some(context_line)
                    };
                    i += 1;

                    let mut old_lines = Vec::new();
                    let mut new_lines = Vec::new();
                    let mut is_end_of_file = false;

                    while i < end_idx && !lines[i].starts_with("@@") && !lines[i].starts_with("***")
                    {
                        let change_line = lines[i];

                        if change_line == "*** End of File" {
                            is_end_of_file = true;
                            i += 1;
                            break;
                        }

                        if let Some(kept) = change_line.strip_prefix(' ') {
                            old_lines.push(kept.to_string());
                            new_lines.push(kept.to_string());
                        } else if let Some(removed) = change_line.strip_prefix('-') {
                            old_lines.push(removed.to_string());
                        } else if let Some(added) = change_line.strip_prefix('+') {
                            new_lines.push(added.to_string());
                        }

                        i += 1;
                    }

                    chunks.push(UpdateChunk {
                        context,
                        old_lines,
                        new_lines,
                        is_end_of_file,
                    });
                } else {
                    i += 1;
                }
            }

            hunks.push(Hunk::Update {
                path: file_path,
                move_path,
                chunks,
            });
        } else {
            i += 1;
        }
    }

    Ok(hunks)
}

// ---------------------------------------------------------------------------
// Sequence matching (4-pass with fallback)
// ---------------------------------------------------------------------------

fn normalize_unicode(s: &str) -> String {
    use unicode_normalization::UnicodeNormalization;
    s.nfkc().collect()
}

fn try_match<F>(
    lines: &[String],
    pattern: &[String],
    start_index: usize,
    compare: F,
    eof: bool,
) -> Option<usize>
where
    F: Fn(&str, &str) -> bool,
{
    if pattern.is_empty() || pattern.len() > lines.len() {
        return None;
    }

    // If EOF anchor, try matching from end of file first
    if eof {
        let from_end = lines.len().saturating_sub(pattern.len());
        if from_end >= start_index {
            let matches = pattern
                .iter()
                .enumerate()
                .all(|(j, p)| compare(&lines[from_end + j], p));
            if matches {
                return Some(from_end);
            }
        }
    }

    // Forward search from start_index
    let max_start = lines.len().saturating_sub(pattern.len());
    for i in start_index..=max_start {
        let matches = pattern
            .iter()
            .enumerate()
            .all(|(j, p)| compare(&lines[i + j], p));
        if matches {
            return Some(i);
        }
    }

    None
}

fn seek_sequence(
    lines: &[String],
    pattern: &[String],
    start_index: usize,
    eof: bool,
) -> Option<usize> {
    if pattern.is_empty() {
        return None;
    }

    // Pass 1: exact match
    if let Some(idx) = try_match(lines, pattern, start_index, |a, b| a == b, eof) {
        return Some(idx);
    }

    // Pass 2: rstrip (trim trailing whitespace)
    if let Some(idx) = try_match(
        lines,
        pattern,
        start_index,
        |a, b| a.trim_end() == b.trim_end(),
        eof,
    ) {
        return Some(idx);
    }

    // Pass 3: trim (both ends)
    if let Some(idx) = try_match(
        lines,
        pattern,
        start_index,
        |a, b| a.trim() == b.trim(),
        eof,
    ) {
        return Some(idx);
    }

    // Pass 4: normalized Unicode
    try_match(
        lines,
        pattern,
        start_index,
        |a, b| normalize_unicode(a.trim()) == normalize_unicode(b.trim()),
        eof,
    )
}

// ---------------------------------------------------------------------------
// Patch application
// ---------------------------------------------------------------------------

fn compute_replacements(
    original_lines: &[String],
    file_path: &str,
    chunks: &[UpdateChunk],
) -> Result<Vec<(usize, usize, Vec<String>)>, String> {
    let mut replacements: Vec<(usize, usize, Vec<String>)> = Vec::new();
    let mut line_index: usize = 0;

    for chunk in chunks {
        // Handle context-based seeking
        if let Some(ref context) = chunk.context {
            let context_pattern = vec![context.clone()];
            let context_idx = seek_sequence(original_lines, &context_pattern, line_index, false)
                .ok_or_else(|| format!("Failed to find context '{}' in {}", context, file_path))?;
            line_index = context_idx + 1;
        }

        // Handle pure addition (no old lines)
        if chunk.old_lines.is_empty() {
            let insertion_idx = if !original_lines.is_empty()
                && original_lines.last().is_some_and(|l| l.is_empty())
            {
                original_lines.len() - 1
            } else {
                original_lines.len()
            };
            replacements.push((insertion_idx, 0, chunk.new_lines.clone()));
            continue;
        }

        // Try to match old lines in the file
        let mut pattern = chunk.old_lines.clone();
        let mut new_slice = chunk.new_lines.clone();
        let found = seek_sequence(original_lines, &pattern, line_index, chunk.is_end_of_file);

        let found = match found {
            Some(idx) => Some(idx),
            None => {
                // Retry without trailing empty line
                if !pattern.is_empty() && pattern.last().is_some_and(|l| l.is_empty()) {
                    pattern.pop();
                    if !new_slice.is_empty() && new_slice.last().is_some_and(|l| l.is_empty()) {
                        new_slice.pop();
                    }
                    seek_sequence(original_lines, &pattern, line_index, chunk.is_end_of_file)
                } else {
                    None
                }
            }
        };

        if let Some(idx) = found {
            replacements.push((idx, pattern.len(), new_slice));
            line_index = idx + pattern.len();
        } else {
            return Err(format!(
                "Failed to find expected lines in {}:\n{}",
                file_path,
                chunk.old_lines.join("\n")
            ));
        }
    }

    // Sort replacements by index
    replacements.sort_by_key(|r| r.0);

    Ok(replacements)
}

fn apply_replacements(
    lines: &[String],
    replacements: &[(usize, usize, Vec<String>)],
) -> Vec<String> {
    let mut result: Vec<String> = lines.to_vec();

    // Apply replacements in reverse order to avoid index shifting
    for &(start_idx, old_len, ref new_segment) in replacements.iter().rev() {
        // Remove old lines and insert new ones
        let end = (start_idx + old_len).min(result.len());
        result.splice(start_idx..end, new_segment.iter().cloned());
    }

    result
}

async fn apply_hunks(hunks: &[Hunk]) -> Result<Vec<String>, String> {
    let mut summary = Vec::new();

    for hunk in hunks {
        match hunk {
            Hunk::Add { path, contents } => {
                let p = Path::new(path);
                if let Some(parent) = p.parent() {
                    if !parent.exists() {
                        tokio::fs::create_dir_all(parent)
                            .await
                            .map_err(|e| format!("Failed to create dirs for {path}: {e}"))?;
                    }
                }
                tokio::fs::write(path, contents)
                    .await
                    .map_err(|e| format!("Failed to write {path}: {e}"))?;
                summary.push(format!("A {path}"));
            }
            Hunk::Delete { path } => {
                tokio::fs::remove_file(path)
                    .await
                    .map_err(|e| format!("Failed to delete {path}: {e}"))?;
                summary.push(format!("D {path}"));
            }
            Hunk::Update {
                path,
                move_path,
                chunks,
            } => {
                let content = tokio::fs::read_to_string(path)
                    .await
                    .map_err(|e| format!("Failed to read {path}: {e}"))?;
                // Normalize CRLF/CR to LF
                let content = content.replace("\r\n", "\n").replace('\r', "\n");

                let mut original_lines: Vec<String> =
                    content.split('\n').map(|s| s.to_string()).collect();

                // Drop trailing empty element for consistent line counting
                if original_lines.last().is_some_and(|l| l.is_empty()) {
                    original_lines.pop();
                }

                let replacements = compute_replacements(&original_lines, path, chunks)?;
                let mut new_lines = apply_replacements(&original_lines, &replacements);

                // Ensure trailing newline
                if new_lines.is_empty() || !new_lines.last().is_none_or(|l| l.is_empty()) {
                    new_lines.push(String::new());
                }

                let new_content = new_lines.join("\n");

                let target_path = move_path.as_deref().unwrap_or(path.as_str());

                if let Some(mp) = move_path {
                    let target = Path::new(mp.as_str());
                    if let Some(parent) = target.parent() {
                        if !parent.exists() {
                            tokio::fs::create_dir_all(parent)
                                .await
                                .map_err(|e| format!("Failed to create dirs for {mp}: {e}"))?;
                        }
                    }
                    tokio::fs::write(mp, &new_content)
                        .await
                        .map_err(|e| format!("Failed to write {mp}: {e}"))?;
                    tokio::fs::remove_file(path)
                        .await
                        .map_err(|e| format!("Failed to remove old file {path}: {e}"))?;
                } else {
                    tokio::fs::write(path, &new_content)
                        .await
                        .map_err(|e| format!("Failed to write {path}: {e}"))?;
                }

                summary.push(format!("M {target_path}"));
            }
        }
    }

    Ok(summary)
}

// ---------------------------------------------------------------------------
// Tool definition
// ---------------------------------------------------------------------------

/// Arguments for the ApplyPatch tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct ApplyPatchArgs {
    /// The full patch text that describes all changes to be made.
    pub patch_text: String,
}

/// Result returned by the ApplyPatch tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct ApplyPatchResult {
    pub summary: Vec<String>,
    pub files_changed: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub diagnostics: Option<String>,
}

pub struct ApplyPatchTool {
    lsp_manager: Option<Arc<LspManager>>,
}

impl ApplyPatchTool {
    pub fn new() -> Self {
        Self { lsp_manager: None }
    }

    pub fn with_lsp_manager(mut self, m: Arc<LspManager>) -> Self {
        self.lsp_manager = Some(m);
        self
    }

    pub fn with_lsp_manager_opt(mut self, m: Option<Arc<LspManager>>) -> Self {
        self.lsp_manager = m;
        self
    }
}

impl Default for ApplyPatchTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for ApplyPatchTool {
    fn name(&self) -> &str {
        "apply_patch"
    }

    fn description(&self) -> &str {
        include_str!("instructions/apply_patch.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(ApplyPatchArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: ApplyPatchArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let hunks = parse_patch(&args.patch_text)?;
        if hunks.is_empty() {
            return Err("No files were modified.".to_string());
        }

        let summary = apply_hunks(&hunks).await?;
        let files_changed = summary.len();

        // Fetch LSP diagnostics for all modified/added files
        let diagnostics = if let Some(ref lsp) = self.lsp_manager {
            let mut all_diag = String::new();
            for entry in &summary {
                // entries are like "M path" or "A path"
                let file_path = entry.split_whitespace().nth(1).unwrap_or("");
                if !file_path.is_empty() && (entry.starts_with("M ") || entry.starts_with("A ")) {
                    let output =
                        crate::diagnostics_helper::fetch_diagnostics_output(lsp, file_path).await;
                    if !output.is_empty() {
                        all_diag.push_str(&output);
                    }
                }
            }
            if all_diag.is_empty() {
                None
            } else {
                Some(all_diag)
            }
        } else {
            None
        };

        let result = ApplyPatchResult {
            summary,
            files_changed,
            diagnostics,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_parse_patch_add_file() {
        let patch = r#"*** Begin Patch
*** Add File: hello.txt
+Hello world
+Second line
*** End Patch"#;

        let hunks = parse_patch(patch).expect("parse");
        assert_eq!(hunks.len(), 1);
        match &hunks[0] {
            Hunk::Add { path, contents } => {
                assert_eq!(path, "hello.txt");
                assert_eq!(contents, "Hello world\nSecond line");
            }
            _ => panic!("expected Add hunk"),
        }
    }

    #[test]
    fn test_parse_patch_delete_file() {
        let patch = r#"*** Begin Patch
*** Delete File: old.txt
*** End Patch"#;

        let hunks = parse_patch(patch).expect("parse");
        assert_eq!(hunks.len(), 1);
        match &hunks[0] {
            Hunk::Delete { path } => assert_eq!(path, "old.txt"),
            _ => panic!("expected Delete hunk"),
        }
    }

    #[test]
    fn test_parse_patch_update_file() {
        let patch = r#"*** Begin Patch
*** Update File: src/main.rs
@@ fn main() {
-    println!("old");
+    println!("new");
*** End Patch"#;

        let hunks = parse_patch(patch).expect("parse");
        assert_eq!(hunks.len(), 1);
        match &hunks[0] {
            Hunk::Update { path, chunks, .. } => {
                assert_eq!(path, "src/main.rs");
                assert_eq!(chunks.len(), 1);
                assert_eq!(chunks[0].old_lines, vec!["    println!(\"old\");"]);
                assert_eq!(chunks[0].new_lines, vec!["    println!(\"new\");"]);
            }
            _ => panic!("expected Update hunk"),
        }
    }

    #[test]
    fn test_parse_patch_with_move() {
        let patch = r#"*** Begin Patch
*** Update File: old/path.rs
*** Move to: new/path.rs
@@ fn hello() {
-    println!("hi");
+    println!("hello");
*** End Patch"#;

        let hunks = parse_patch(patch).expect("parse");
        assert_eq!(hunks.len(), 1);
        match &hunks[0] {
            Hunk::Update {
                path, move_path, ..
            } => {
                assert_eq!(path, "old/path.rs");
                assert_eq!(move_path.as_deref(), Some("new/path.rs"));
            }
            _ => panic!("expected Update hunk"),
        }
    }

    #[test]
    fn test_parse_patch_missing_markers() {
        let patch = "just some text";
        assert!(parse_patch(patch).is_err());
    }

    #[test]
    fn test_seek_sequence_exact() {
        let lines: Vec<String> = vec![
            "line 1".to_string(),
            "line 2".to_string(),
            "line 3".to_string(),
        ];
        let pattern = vec!["line 2".to_string()];
        assert_eq!(seek_sequence(&lines, &pattern, 0, false), Some(1));
    }

    #[test]
    fn test_seek_sequence_trimmed() {
        let lines: Vec<String> = vec!["  line 1  ".to_string(), "  line 2  ".to_string()];
        let pattern = vec!["line 2".to_string()];
        assert_eq!(seek_sequence(&lines, &pattern, 0, false), Some(1));
    }

    #[test]
    fn test_seek_sequence_not_found() {
        let lines: Vec<String> = vec!["line 1".to_string()];
        let pattern = vec!["line 99".to_string()];
        assert_eq!(seek_sequence(&lines, &pattern, 0, false), None);
    }

    #[tokio::test]
    async fn test_apply_patch_add_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("new_file.txt");

        let patch = format!(
            "*** Begin Patch\n*** Add File: {}\n+Hello\n+World\n*** End Patch",
            file_path.display()
        );

        let tool = ApplyPatchTool::new();
        let args = serde_json::json!({ "patchText": patch });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.files_changed, 1);
        assert!(parsed.summary[0].starts_with("A "));

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert_eq!(content, "Hello\nWorld");
    }

    #[tokio::test]
    async fn test_apply_patch_delete_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("to_delete.txt");
        std::fs::write(&file_path, "content").expect("write");

        let patch = format!(
            "*** Begin Patch\n*** Delete File: {}\n*** End Patch",
            file_path.display()
        );

        let tool = ApplyPatchTool::new();
        let args = serde_json::json!({ "patchText": patch });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.files_changed, 1);
        assert!(!file_path.exists());
    }

    #[tokio::test]
    async fn test_apply_patch_update_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("update_me.txt");
        std::fs::write(&file_path, "fn main() {\n    println!(\"old\");\n}\n").expect("write");

        let patch = format!(
            "*** Begin Patch\n*** Update File: {}\n@@ fn main() {{\n-    println!(\"old\");\n+    println!(\"new\");\n*** End Patch",
            file_path.display()
        );

        let tool = ApplyPatchTool::new();
        let args = serde_json::json!({ "patchText": patch });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.files_changed, 1);

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert!(content.contains("println!(\"new\")"));
        assert!(!content.contains("println!(\"old\")"));
    }

    #[test]
    fn test_strip_heredoc() {
        let input = "cat <<'EOF'\nhello\nworld\nEOF";
        assert_eq!(strip_heredoc(input), "hello\nworld");
    }

    #[test]
    fn test_strip_heredoc_no_match() {
        let input = "just regular text";
        assert_eq!(strip_heredoc(input), "just regular text");
    }

    #[test]
    fn test_parse_patch_crlf_normalized() {
        // Patch with CRLF line endings should parse correctly
        let patch = "*** Begin Patch\r\n*** Add File: hello.txt\r\n+Hello world\r\n+Second line\r\n*** End Patch\r\n";

        let hunks = parse_patch(patch).expect("parse");
        assert_eq!(hunks.len(), 1);
        match &hunks[0] {
            Hunk::Add { path, contents } => {
                assert_eq!(path, "hello.txt");
                assert_eq!(contents, "Hello world\nSecond line");
            }
            _ => panic!("expected Add hunk"),
        }
    }

    #[tokio::test]
    async fn test_apply_patch_crlf_file() {
        let dir = tempdir().expect("create temp dir");
        let file_path = dir.path().join("crlf_file.txt");
        // Write file with CRLF line endings
        std::fs::write(&file_path, "fn main() {\r\n    println!(\"old\");\r\n}\r\n")
            .expect("write");

        let patch = format!(
            "*** Begin Patch\n*** Update File: {}\n@@ fn main() {{\n-    println!(\"old\");\n+    println!(\"new\");\n*** End Patch",
            file_path.display()
        );

        let tool = ApplyPatchTool::new();
        let args = serde_json::json!({ "patchText": patch });

        let result = tool.execute(args).await.expect("execute");
        let parsed: ApplyPatchResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.files_changed, 1);

        let content = std::fs::read_to_string(&file_path).expect("read");
        assert!(content.contains("println!(\"new\")"));
        assert!(!content.contains("println!(\"old\")"));
    }

    #[test]
    fn test_apply_replacements_reverse_order() {
        let lines: Vec<String> = vec![
            "a".to_string(),
            "b".to_string(),
            "c".to_string(),
            "d".to_string(),
        ];
        let replacements = vec![
            (1, 1, vec!["B1".to_string(), "B2".to_string()]),
            (3, 1, vec!["D1".to_string()]),
        ];
        let result = apply_replacements(&lines, &replacements);
        assert_eq!(result, vec!["a", "B1", "B2", "c", "D1"]);
    }
}
