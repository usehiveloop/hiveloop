use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;
use tokio::io::AsyncReadExt;

use crate::ToolExecutor;

/// Arguments for the Bash tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct BashArgs {
    /// The bash command to execute.
    pub command: String,
    /// Timeout in milliseconds. Defaults to 120000 (2 minutes).
    pub timeout: Option<u64>,
    /// Working directory for the command. Defaults to current directory.
    pub workdir: Option<String>,
    /// A short description of what this command does.
    pub description: Option<String>,
}

/// Result returned by the Bash tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct BashResult {
    pub output: String,
    pub exit_code: Option<i32>,
    pub timed_out: bool,
}

/// Maximum output size in bytes before truncation.
const MAX_OUTPUT_BYTES: usize = 50_000;

pub struct BashTool;

impl BashTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for BashTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for BashTool {
    fn name(&self) -> &str {
        "bash"
    }

    fn description(&self) -> &str {
        include_str!("instructions/bash.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(BashArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: BashArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let timeout_ms = args.timeout.unwrap_or(120_000);
        let workdir = args.workdir.as_deref().unwrap_or(".");

        let mut cmd = tokio::process::Command::new("sh");
        cmd.arg("-c").arg(&args.command);
        cmd.current_dir(workdir);
        cmd.stdout(std::process::Stdio::piped());
        cmd.stderr(std::process::Stdio::piped());

        let mut child = cmd.spawn().map_err(|e| format!("Failed to spawn command: {e}"))?;

        let stdout = child.stdout.take();
        let stderr = child.stderr.take();

        // Read stdout and stderr concurrently
        let read_output = async {
            let mut combined = Vec::new();

            let mut stdout_buf = Vec::new();
            let mut stderr_buf = Vec::new();

            if let Some(mut stdout) = stdout {
                let _ = stdout.read_to_end(&mut stdout_buf).await;
            }
            if let Some(mut stderr) = stderr {
                let _ = stderr.read_to_end(&mut stderr_buf).await;
            }

            combined.extend_from_slice(&stdout_buf);
            if !stderr_buf.is_empty() {
                if !combined.is_empty() && !combined.ends_with(b"\n") {
                    combined.push(b'\n');
                }
                combined.extend_from_slice(&stderr_buf);
            }

            combined
        };

        let timeout_duration = Duration::from_millis(timeout_ms);

        match tokio::time::timeout(timeout_duration, async {
            let output = read_output.await;
            let status = child.wait().await;
            (output, status)
        })
        .await
        {
            Ok((output, status)) => {
                let exit_code = status.ok().and_then(|s| s.code());
                let output_str = truncate_output(&output);

                let result = BashResult {
                    output: output_str,
                    exit_code,
                    timed_out: false,
                };

                serde_json::to_string(&result)
                    .map_err(|e| format!("Failed to serialize result: {e}"))
            }
            Err(_) => {
                // Timeout — kill the process
                let _ = child.kill().await;

                let result = BashResult {
                    output: "[timed out]".to_string(),
                    exit_code: None,
                    timed_out: true,
                };

                serde_json::to_string(&result)
                    .map_err(|e| format!("Failed to serialize result: {e}"))
            }
        }
    }
}

fn truncate_output(bytes: &[u8]) -> String {
    let s = String::from_utf8_lossy(bytes);
    if s.len() > MAX_OUTPUT_BYTES {
        let truncated = &s[..MAX_OUTPUT_BYTES];
        format!("{truncated}\n[output truncated]")
    } else {
        s.into_owned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_bash_echo() {
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "echo hello",
            "description": "test echo"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: BashResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.output.trim(), "hello");
        assert_eq!(parsed.exit_code, Some(0));
        assert!(!parsed.timed_out);
    }

    #[tokio::test]
    async fn test_bash_exit_code() {
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "exit 42"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: BashResult = serde_json::from_str(&result).expect("parse");

        assert_eq!(parsed.exit_code, Some(42));
    }

    #[tokio::test]
    async fn test_bash_stderr() {
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "echo error >&2"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: BashResult = serde_json::from_str(&result).expect("parse");

        assert!(parsed.output.contains("error"));
    }

    #[tokio::test]
    async fn test_bash_timeout() {
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "sleep 10",
            "timeout": 500
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: BashResult = serde_json::from_str(&result).expect("parse");

        assert!(parsed.timed_out);
    }

    #[tokio::test]
    async fn test_bash_workdir() {
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "pwd",
            "workdir": "/tmp"
        });

        let result = tool.execute(args).await.expect("execute");
        let parsed: BashResult = serde_json::from_str(&result).expect("parse");

        // On macOS /tmp is a symlink to /private/tmp
        assert!(
            parsed.output.trim() == "/tmp" || parsed.output.trim() == "/private/tmp",
            "unexpected pwd: {}",
            parsed.output.trim()
        );
    }

    #[test]
    fn test_truncate_output() {
        let short = b"hello";
        assert_eq!(truncate_output(short), "hello");

        let long = vec![b'x'; MAX_OUTPUT_BYTES + 100];
        let result = truncate_output(&long);
        assert!(result.ends_with("[output truncated]"));
    }
}
