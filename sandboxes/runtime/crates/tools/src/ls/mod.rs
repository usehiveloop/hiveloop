use async_trait::async_trait;
use ignore::WalkBuilder;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::path::Path;

use crate::ToolExecutor;

#[cfg(test)]
mod tests;

/// Arguments for the LS tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct LsArgs {
    /// The absolute path of the directory to list.
    pub path: String,
}

/// Result returned by the LS tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct LsResult {
    pub output: String,
    pub total_entries: usize,
    pub truncated: bool,
}

/// Maximum number of entries to return.
const MAX_ENTRIES: usize = 100;

/// Default directories/files to ignore.
const DEFAULT_IGNORE_PATTERNS: &[&str] = &[
    "node_modules",
    "__pycache__",
    ".git",
    "dist",
    "build",
    "target",
    "vendor",
    "bin",
    "obj",
    ".idea",
    ".vscode",
    ".zig-cache",
    "zig-out",
    ".coverage",
    "coverage",
    "tmp",
    "temp",
    ".cache",
    "cache",
    "logs",
    ".venv",
    "venv",
    "env",
];

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

/// Render directory tree using ignore::WalkBuilder (respects .gitignore).
fn render_tree(
    root: &Path,
    ignore_patterns: &[&str],
    limit: usize,
) -> Result<(String, usize, bool), String> {
    let walker = WalkBuilder::new(root)
        .hidden(false)
        .git_ignore(true)
        .build();

    let mut files: Vec<String> = Vec::new();
    for entry in walker {
        let entry = match entry {
            Ok(e) => e,
            Err(_) => continue,
        };
        let path = entry.path();
        let relative = path.strip_prefix(root).unwrap_or(path);
        let name = relative.to_string_lossy().to_string();
        if name.is_empty() {
            continue;
        }

        // Check ignore patterns on any path component
        let should_ignore = relative.components().any(|c| {
            let s = c.as_os_str().to_string_lossy();
            ignore_patterns.contains(&s.as_ref())
        });
        if should_ignore {
            continue;
        }

        if path.is_dir() {
            files.push(format!("{}/", name));
        } else {
            files.push(name);
        }

        if files.len() >= limit {
            break;
        }
    }

    let truncated = files.len() >= limit;
    let total = files.len();

    // Build tree-like output with 2-space indentation
    let mut output = String::new();
    for f in &files {
        let depth = f
            .matches('/')
            .count()
            .saturating_sub(if f.ends_with('/') { 1 } else { 0 });
        let indent = "  ".repeat(depth);
        let basename = f.rsplit('/').find(|s| !s.is_empty()).unwrap_or(f);
        let suffix = if f.ends_with('/') { "/" } else { "" };
        output.push_str(&format!("{}{}{}\n", indent, basename, suffix));
    }

    Ok((output, total, truncated))
}

#[async_trait]
impl ToolExecutor for LsTool {
    fn name(&self) -> &str {
        "LS"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/ls.txt")
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

        let root = path.to_path_buf();
        let (output, total_entries, truncated) = tokio::task::spawn_blocking(move || {
            render_tree(&root, DEFAULT_IGNORE_PATTERNS, MAX_ENTRIES)
        })
        .await
        .map_err(|e| format!("Task join error: {e}"))??;

        let output = if truncated {
            format!(
                "{output}\n\
                [Listing truncated at {} entries. Choose one:\n\
                - Narrow: call LS on a deeper path, or use Glob with pattern=\"...\" path=\"{}\" to match only what you need\n\
                - Search: call RipGrep with path=\"{}\" and a regex pattern to locate specific files\n\
                - Summarize: spawn self_agent with a focused question so the full listing stays out of this context]",
                MAX_ENTRIES, dir_path, dir_path,
            )
        } else {
            output
        };

        let result = LsResult {
            output,
            total_entries,
            truncated,
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
