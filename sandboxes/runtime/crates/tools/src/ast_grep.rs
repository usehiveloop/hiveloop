use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::any::Any;

use crate::boundary::ProjectBoundary;
use crate::ToolExecutor;

#[derive(Debug, Deserialize, JsonSchema)]
pub struct AstGrepArgs {
    /// AST pattern to search for (e.g. "console.log($$$)", "if ($COND) { $$$ }")
    pub pattern: String,

    /// File or directory to search in. Defaults to the current working directory.
    pub path: Option<String>,

    /// Language to search (e.g. "javascript", "typescript", "rust", "python", "go").
    /// If not specified, ast-grep infers from file extensions.
    pub language: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AstGrepMatch {
    pub file: String,
    pub line: u64,
    pub column: u64,
    pub text: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct AstGrepResult {
    pub matches: Vec<AstGrepMatch>,
    pub count: usize,
}

pub struct AstGrepTool {
    boundary: Option<ProjectBoundary>,
}

impl AstGrepTool {
    pub fn new() -> Self {
        Self { boundary: None }
    }

    pub fn with_boundary(mut self, boundary: ProjectBoundary) -> Self {
        self.boundary = Some(boundary);
        self
    }
}

impl Default for AstGrepTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for AstGrepTool {
    fn name(&self) -> &str {
        "AstGrep"
    }

    fn description(&self) -> &str {
        include_str!("instructions/ast_grep.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(AstGrepArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: AstGrepArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if let Some(ref boundary) = self.boundary {
            if let Some(ref path) = args.path {
                if path != "." {
                    boundary.check(path)?;
                }
            }
        }

        let mut cmd = tokio::process::Command::new("sg");
        cmd.arg("run");
        cmd.arg("--pattern").arg(&args.pattern);
        cmd.arg("--json");

        if let Some(ref lang) = args.language {
            cmd.arg("--lang").arg(lang);
        }

        let search_path = args.path.unwrap_or_else(|| ".".to_string());
        cmd.arg(&search_path);

        cmd.stdin(std::process::Stdio::null());
        cmd.stdout(std::process::Stdio::piped());
        cmd.stderr(std::process::Stdio::piped());

        let output = cmd
            .output()
            .await
            .map_err(|e| format!("Failed to run ast-grep (sg): {e}"))?;

        let stdout = String::from_utf8_lossy(&output.stdout);
        let stderr = String::from_utf8_lossy(&output.stderr);

        // ast-grep returns exit code 0 for matches, 1 for no matches
        if !output.status.success() && output.status.code() != Some(1) {
            return Err(format!(
                "ast-grep failed (exit {}): {}",
                output.status.code().unwrap_or(-1),
                stderr
            ));
        }

        let matches: Vec<AstGrepMatch> = if stdout.trim().is_empty() {
            Vec::new()
        } else {
            let raw: serde_json::Value = serde_json::from_str(stdout.trim())
                .unwrap_or_else(|_| serde_json::Value::Array(vec![]));

            match raw {
                serde_json::Value::Array(items) => items
                    .into_iter()
                    .filter_map(|item| {
                        let file = item.get("file")?.as_str()?.to_string();
                        let range = item.get("range")?;
                        let start = range.get("start")?;
                        let line = start.get("line")?.as_u64().unwrap_or(0);
                        let column = start.get("column")?.as_u64().unwrap_or(0);
                        let text = item
                            .get("text")
                            .and_then(|t| t.as_str())
                            .unwrap_or("")
                            .to_string();

                        Some(AstGrepMatch {
                            file,
                            line,
                            column,
                            text,
                        })
                    })
                    .collect(),
                _ => Vec::new(),
            }
        };

        let count = matches.len();
        let result = AstGrepResult { matches, count };

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
    fn test_ast_grep_tool_name() {
        let tool = AstGrepTool::new();
        assert_eq!(tool.name(), "AstGrep");
    }

    #[test]
    fn test_ast_grep_description_not_empty() {
        let tool = AstGrepTool::new();
        assert!(!tool.description().is_empty());
    }

    #[test]
    fn test_ast_grep_schema_has_pattern() {
        let tool = AstGrepTool::new();
        let schema = tool.parameters_schema();
        let props = schema
            .get("properties")
            .expect("schema should have properties");
        assert!(props.get("pattern").is_some());
    }

    #[test]
    fn test_ast_grep_schema_has_no_rewrite() {
        let tool = AstGrepTool::new();
        let schema = tool.parameters_schema();
        let props = schema.get("properties").expect("properties");
        assert!(
            props.get("rewrite").is_none(),
            "AstGrep is read-only; rewrite must not be exposed"
        );
    }
}
