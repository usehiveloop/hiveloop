use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::any::Any;

use crate::boundary::ProjectBoundary;
use crate::ToolExecutor;

#[derive(Debug, Deserialize, JsonSchema)]
pub struct RipGrepArgs {
    /// Regex pattern to search for
    pub pattern: String,

    /// File or directory to search in. Defaults to the current working directory.
    pub path: Option<String>,

    /// Filter by file type (e.g. "rust", "ts", "py", "go")
    pub file_type: Option<String>,

    /// Filter by glob pattern (e.g. "*.rs", "*.{ts,tsx}")
    pub glob: Option<String>,

    /// Treat the pattern as a literal string, not a regex
    pub fixed_strings: Option<bool>,

    /// Limit matches per file
    pub max_count: Option<usize>,

    /// Number of context lines to show before and after each match
    pub context: Option<usize>,

    /// Enable multiline matching
    pub multiline: Option<bool>,

    /// Return structured JSON output instead of raw text
    pub json: Option<bool>,

    /// Only match whole words
    pub word_regexp: Option<bool>,

    /// Show lines that do NOT match the pattern
    pub invert_match: Option<bool>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RipGrepResult {
    pub output: String,
    pub exit_code: Option<i32>,
}

pub struct RipGrepTool {
    boundary: Option<ProjectBoundary>,
}

impl RipGrepTool {
    pub fn new() -> Self {
        Self { boundary: None }
    }

    pub fn with_boundary(mut self, boundary: ProjectBoundary) -> Self {
        self.boundary = Some(boundary);
        self
    }
}

impl Default for RipGrepTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for RipGrepTool {
    fn name(&self) -> &str {
        "RipGrep"
    }

    fn description(&self) -> &str {
        include_str!("instructions/rip_grep.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(RipGrepArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: RipGrepArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        // Check project boundary for the search path
        if let Some(ref boundary) = self.boundary {
            if let Some(ref path) = args.path {
                if path != "." {
                    boundary.check(path)?;
                }
            }
        }

        let mut cmd = tokio::process::Command::new("rg");
        cmd.arg(&args.pattern);

        if let Some(ref file_type) = args.file_type {
            cmd.arg("--type").arg(file_type);
        }

        if let Some(ref glob) = args.glob {
            cmd.arg("--glob").arg(glob);
        }

        if args.fixed_strings.unwrap_or(false) {
            cmd.arg("--fixed-strings");
        }

        if let Some(max_count) = args.max_count {
            cmd.arg("--max-count").arg(max_count.to_string());
        }

        if let Some(context) = args.context {
            cmd.arg("--context").arg(context.to_string());
        }

        if args.multiline.unwrap_or(false) {
            cmd.arg("--multiline");
        }

        if args.json.unwrap_or(false) {
            cmd.arg("--json");
        }

        if args.word_regexp.unwrap_or(false) {
            cmd.arg("--word-regexp");
        }

        if args.invert_match.unwrap_or(false) {
            cmd.arg("--invert-match");
        }

        // Always show line numbers
        cmd.arg("--line-number");

        let search_path = args.path.unwrap_or_else(|| ".".to_string());
        cmd.arg(&search_path);

        cmd.stdin(std::process::Stdio::null());
        cmd.stdout(std::process::Stdio::piped());
        cmd.stderr(std::process::Stdio::piped());

        let output = cmd
            .output()
            .await
            .map_err(|e| format!("Failed to run ripgrep (rg): {e}"))?;

        let stdout = String::from_utf8_lossy(&output.stdout);
        let stderr = String::from_utf8_lossy(&output.stderr);
        let exit_code = output.status.code();

        // rg returns exit code 0 for matches, 1 for no matches, 2 for errors.
        // On 0 and 1 stderr may hold noise like "No files were searched" — drop it.
        if exit_code == Some(2) {
            return Err(format!("ripgrep error: {}", stderr));
        }

        let result = RipGrepResult {
            output: stdout.to_string(),
            exit_code,
        };

        let serialized = serde_json::to_string(&result)
            .map_err(|e| format!("Failed to serialize result: {e}"))?;

        let truncated = crate::truncation::truncate_output(
            &serialized,
            crate::truncation::MAX_LINES,
            crate::truncation::MAX_BYTES,
        );
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn Any {
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rip_grep_tool_name() {
        let tool = RipGrepTool::new();
        assert_eq!(tool.name(), "RipGrep");
    }

    #[test]
    fn test_rip_grep_description_not_empty() {
        let tool = RipGrepTool::new();
        assert!(!tool.description().is_empty());
    }

    #[test]
    fn test_rip_grep_schema_has_pattern() {
        let tool = RipGrepTool::new();
        let schema = tool.parameters_schema();
        let props = schema
            .get("properties")
            .expect("schema should have properties");
        assert!(props.get("pattern").is_some());
    }
}
