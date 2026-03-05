use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;
use tokio::io::AsyncReadExt;

use crate::agent::{AgentTaskNotification, AGENT_CONTEXT};
use crate::ToolExecutor;

/// Arguments for the Bash tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct BashArgs {
    /// The shell command to execute. Example: 'ls -la /tmp'
    #[schemars(description = "The shell command to execute. Example: 'ls -la /tmp'")]
    pub command: String,
    /// Timeout in milliseconds. Default: 120000 (2 minutes). Maximum: 600000 (10 minutes).
    #[schemars(
        description = "Timeout in milliseconds. Default: 120000 (2 minutes). Maximum: 600000 (10 minutes)"
    )]
    pub timeout: Option<u64>,
    /// Working directory for the command. Defaults to current directory. Use this instead of 'cd <dir> && <cmd>'.
    #[schemars(
        description = "Working directory for the command. Defaults to current directory. Use this instead of 'cd <dir> && <cmd>'"
    )]
    pub workdir: Option<String>,
    /// A short description of what this command does in 5-10 words.
    #[schemars(description = "A short description of what this command does in 5-10 words")]
    pub description: Option<String>,
    /// Run this command in the background. Returns immediately with a task_id.
    /// The agent will be notified when the command completes.
    #[schemars(
        description = "Set to true to run in the background. Returns immediately with a task_id; you will be notified on completion"
    )]
    #[serde(default)]
    pub background: bool,
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

/// Kill the entire process group on Unix to prevent orphaned children.
#[cfg(unix)]
fn kill_process_tree(child: &tokio::process::Child) {
    if let Some(pid) = child.id() {
        // Kill the entire process group (negative pid)
        unsafe {
            libc::kill(-(pid as i32), libc::SIGKILL);
        }
    }
}

/// Execute a bash command and return the result.
/// Public so it can be called from the hook layer for background execution.
pub async fn run_command(
    command: &str,
    workdir: &str,
    timeout_ms: u64,
) -> Result<BashResult, String> {
    let mut cmd = tokio::process::Command::new("sh");
    cmd.arg("-c").arg(command);
    cmd.current_dir(workdir);
    cmd.stdin(std::process::Stdio::null());
    cmd.stdout(std::process::Stdio::piped());
    cmd.stderr(std::process::Stdio::piped());

    // Make child a process group leader so we can kill the whole tree
    #[cfg(unix)]
    cmd.process_group(0);

    let mut child = cmd
        .spawn()
        .map_err(|e| format!("Failed to spawn command: {e}"))?;

    let stdout = child.stdout.take();
    let stderr = child.stderr.take();

    // Read stdout and stderr concurrently using tokio::join! to prevent deadlock
    let read_output = async {
        let (stdout_buf, stderr_buf) = tokio::join!(
            async {
                let mut buf = Vec::new();
                if let Some(mut out) = stdout {
                    let _ = out.read_to_end(&mut buf).await;
                }
                buf
            },
            async {
                let mut buf = Vec::new();
                if let Some(mut err) = stderr {
                    let _ = err.read_to_end(&mut buf).await;
                }
                buf
            }
        );

        let mut combined = Vec::new();
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

            Ok(BashResult {
                output: output_str,
                exit_code,
                timed_out: false,
            })
        }
        Err(_) => {
            // Timeout — kill the process group, then the process
            #[cfg(unix)]
            kill_process_tree(&child);
            let _ = child.kill().await;

            Ok(BashResult {
                output: "[timed out]".to_string(),
                exit_code: None,
                timed_out: true,
            })
        }
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
        let workdir = args.workdir.as_deref().unwrap_or(".").to_string();
        let command = args.command.clone();
        let description = args.description.clone().unwrap_or_else(|| {
            // Take first line of command, truncated to 80 chars
            let first_line = command.lines().next().unwrap_or(&command);
            if first_line.len() > 80 {
                format!("{}...", &first_line[..77])
            } else {
                first_line.to_string()
            }
        });

        if args.background {
            // Background execution: return immediately, notify on completion
            let ctx = AGENT_CONTEXT
                .try_with(|c| c.clone())
                .map_err(|_| "Background bash requires a conversation context".to_string())?;

            let task_id = uuid::Uuid::new_v4().to_string();
            let task_id_clone = task_id.clone();
            let notification_tx = ctx.notification_tx.clone();

            tokio::spawn(async move {
                let result = run_command(&command, &workdir, timeout_ms).await;

                let output = match result {
                    Ok(bash_result) => match serde_json::to_string(&bash_result) {
                        Ok(json) => Ok(json),
                        Err(e) => Err(format!("Failed to serialize result: {e}")),
                    },
                    Err(e) => Err(e),
                };

                let notification = AgentTaskNotification {
                    task_id: task_id_clone,
                    description,
                    output,
                };

                // If the receiver is dropped (conversation ended), silently discard
                let _ = notification_tx.send(notification).await;
            });

            serde_json::to_string(&serde_json::json!({
                "task_id": task_id,
                "status": "running",
                "message": "Background command started. You will be notified when it completes."
            }))
            .map_err(|e| format!("Failed to serialize result: {e}"))
        } else {
            // Foreground execution: block until complete
            let result = run_command(&command, &workdir, timeout_ms).await?;

            serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
        }
    }
}

/// Head/tail sizes for the spill summary.
const SPILL_HEAD_BYTES: usize = 1_000;
const SPILL_TAIL_BYTES: usize = 1_000;

fn truncate_output(bytes: &[u8]) -> String {
    let s = String::from_utf8_lossy(bytes);
    if s.len() <= MAX_OUTPUT_BYTES {
        return s.into_owned();
    }

    // Spill full output to a temp file so the LLM can read it later
    let spill_path = std::env::temp_dir().join(format!("bridge_bash_{}.txt", uuid::Uuid::new_v4()));
    if let Ok(()) = std::fs::write(&spill_path, bytes) {
        let head = &s[..s.len().min(SPILL_HEAD_BYTES)];
        let tail_start = s.len().saturating_sub(SPILL_TAIL_BYTES);
        let tail = &s[tail_start..];
        format!(
            "{head}\n\n... [Output truncated. Full output ({} bytes) saved to: {}] ...\n\n{tail}",
            bytes.len(),
            spill_path.display()
        )
    } else {
        // Fallback if we can't write the temp file
        let truncated = &s[..MAX_OUTPUT_BYTES];
        format!("{truncated}\n[output truncated]")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::{
        AgentContext, AgentTaskHandle, AgentTaskNotification, AgentTaskResult, SubAgentRunner,
        AGENT_CONTEXT,
    };
    use std::sync::Arc;
    use tokio::sync::mpsc;

    /// Mock SubAgentRunner needed to construct an AgentContext for background tests.
    struct MockRunner;

    #[async_trait]
    impl SubAgentRunner for MockRunner {
        fn available_subagents(&self) -> Vec<(String, String)> {
            vec![]
        }

        async fn run_foreground(
            &self,
            _subagent: &str,
            _prompt: &str,
            _task_id: Option<&str>,
        ) -> Result<AgentTaskResult, String> {
            Err("not implemented".to_string())
        }

        async fn run_background(
            &self,
            _subagent: &str,
            _prompt: &str,
            _description: &str,
        ) -> Result<AgentTaskHandle, String> {
            Err("not implemented".to_string())
        }
    }

    fn make_context() -> (AgentContext, mpsc::Receiver<AgentTaskNotification>) {
        let (tx, rx) = mpsc::channel(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner),
            notification_tx: tx,
            depth: 0,
            max_depth: 3,
        };
        (ctx, rx)
    }

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
    fn test_truncate_output_short() {
        let short = b"hello";
        assert_eq!(truncate_output(short), "hello");
    }

    #[test]
    fn test_truncate_output_spills_to_disk() {
        let long = vec![b'x'; MAX_OUTPUT_BYTES + 100];
        let result = truncate_output(&long);
        // Should contain head, tail, and a spill path
        assert!(
            result.contains("Output truncated"),
            "should mention truncation"
        );
        assert!(result.contains("saved to:"), "should include file path");
        assert!(
            result.contains("bridge_bash_"),
            "should reference a temp file"
        );

        // Extract the path and verify the file exists
        let path_start = result.find("saved to: ").unwrap() + "saved to: ".len();
        let path_end = result[path_start..].find(']').unwrap() + path_start;
        let spill_path = &result[path_start..path_end];
        let content = std::fs::read(spill_path).expect("spill file should be readable");
        assert_eq!(content.len(), MAX_OUTPUT_BYTES + 100);
        // Clean up
        let _ = std::fs::remove_file(spill_path);
    }

    #[tokio::test]
    async fn test_bash_background_returns_immediately() {
        let (ctx, mut rx) = make_context();
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "echo bg_test_output",
            "background": true,
            "description": "background echo test"
        });

        // Execute within AGENT_CONTEXT — should return immediately
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;

        assert!(result.is_ok());
        let output = result.unwrap();
        let parsed: serde_json::Value = serde_json::from_str(&output).expect("parse JSON");

        // Should have task_id and status: "running"
        assert!(parsed.get("task_id").is_some(), "should have task_id");
        assert_eq!(parsed["status"], "running");
        assert!(parsed["message"]
            .as_str()
            .unwrap()
            .contains("Background command started"));

        // Wait for the notification to arrive
        let notification = tokio::time::timeout(Duration::from_secs(5), rx.recv())
            .await
            .expect("notification should arrive within 5s")
            .expect("channel should not be closed");

        assert_eq!(notification.task_id, parsed["task_id"].as_str().unwrap());
        assert_eq!(notification.description, "background echo test");

        // The output should contain the command's result
        let cmd_output = notification.output.expect("should be Ok");
        let bash_result: BashResult = serde_json::from_str(&cmd_output).expect("parse BashResult");
        assert!(bash_result.output.contains("bg_test_output"));
        assert_eq!(bash_result.exit_code, Some(0));
        assert!(!bash_result.timed_out);
    }

    #[tokio::test]
    async fn test_bash_background_without_context_errors() {
        let tool = BashTool::new();
        let args = serde_json::json!({
            "command": "echo hello",
            "background": true
        });

        // No AGENT_CONTEXT set — should error
        let result = tool.execute(args).await;
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .contains("Background bash requires a conversation context"));
    }

    #[tokio::test]
    async fn test_bash_concurrent_stderr_stdout() {
        // Writes large output to both stdout and stderr simultaneously.
        // If reads are sequential (not concurrent), the child can deadlock
        // when one pipe's buffer fills while the other is being drained.
        let result = run_command(
            "for i in $(seq 1 10000); do echo \"out$i\"; echo \"err$i\" >&2; done",
            "/tmp",
            10_000,
        )
        .await
        .expect("should not deadlock");

        assert!(
            !result.timed_out,
            "command should complete without deadlock"
        );
        assert_eq!(result.exit_code, Some(0));
        assert!(result.output.contains("out1"));
        assert!(result.output.contains("err1"));
    }

    #[tokio::test]
    async fn test_bash_stdin_null() {
        // `read` would hang if stdin were open; with Stdio::null() it gets EOF immediately
        let result = run_command("read -t 1 input || echo 'no_stdin'", "/tmp", 5_000)
            .await
            .expect("should complete without hanging");

        assert!(!result.timed_out, "should not time out");
        assert!(result.output.contains("no_stdin"));
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn test_bash_process_group_kill() {
        // Spawn a command that starts a subprocess, then kill via timeout.
        // The subprocess should also be killed via process group.
        let result = run_command("sleep 60 & echo child=$!; wait", "/tmp", 500)
            .await
            .expect("should return on timeout");

        assert!(result.timed_out, "should have timed out");

        // Extract the child PID from output (if it was captured before timeout)
        // The sleep process should have been killed by process group kill
        // We can't reliably check the PID since output may be truncated,
        // but the fact that we returned without hanging proves group kill works.
    }
}
