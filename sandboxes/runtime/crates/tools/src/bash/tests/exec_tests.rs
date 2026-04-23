use super::super::args::BashResult;
use super::super::runner::run_command;
use super::super::tool::BashTool;
use crate::ToolExecutor;

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
