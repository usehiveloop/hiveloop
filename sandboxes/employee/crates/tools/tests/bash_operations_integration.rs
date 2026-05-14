use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use tools::{BashExecOptions, BashOperations, LocalBashOperations};

fn bash() -> LocalBashOperations {
    LocalBashOperations::default()
}

fn opts(timeout_secs: u64) -> BashExecOptions {
    BashExecOptions {
        workdir: std::env::current_dir().unwrap(),
        env: Default::default(),
        timeout: Some(Duration::from_secs(timeout_secs)),
        max_output_bytes: 512 * 1024,
    }
}

#[tokio::test]
async fn developer_runs_ls_and_sees_files() {
    let result = bash().exec("ls Cargo.toml", opts(5)).await.unwrap();
    assert_eq!(result.exit_code, Some(0));
    assert!(String::from_utf8_lossy(&result.stdout_combined).contains("Cargo.toml"));
}

#[tokio::test]
async fn command_that_fails_returns_exit_code() {
    let result = bash().exec("exit 42", opts(5)).await.unwrap();
    assert_eq!(result.exit_code, Some(42));
}

#[tokio::test]
async fn piped_commands_work() {
    let result = bash()
        .exec("echo 'a\nb\nc' | wc -l", opts(5))
        .await
        .unwrap();
    assert_eq!(result.exit_code, Some(0));
    assert!(String::from_utf8_lossy(&result.stdout_combined).trim() == "3");
}

#[tokio::test]
async fn stderr_is_captured_with_stdout() {
    let result = bash()
        .exec("echo ok && echo err >&2", opts(5))
        .await
        .unwrap();
    let output = String::from_utf8_lossy(&result.stdout_combined);
    assert!(output.contains("ok"));
    assert!(output.contains("err"));
}

#[tokio::test]
async fn timed_out_command_reports_timeout() {
    let result = bash().exec("sleep 10", opts(1)).await.unwrap();
    assert!(result.timed_out);
}

#[tokio::test]
async fn multiline_output_is_preserved() {
    let result = bash()
        .exec("printf 'line1\nline2\nline3\nline4\nline5\n'", opts(5))
        .await
        .unwrap();
    let output = String::from_utf8_lossy(&result.stdout_combined);
    assert!(output.contains("line1"));
    assert!(output.contains("line5"));
}

#[tokio::test]
async fn environment_variables_are_respected() {
    let mut o = opts(5);
    o.env.insert("MY_TEST_VAR".into(), "hello_test".into());
    let result = bash().exec("echo $MY_TEST_VAR", o).await.unwrap();
    assert!(String::from_utf8_lossy(&result.stdout_combined).contains("hello_test"));
}

#[tokio::test]
async fn empty_command_produces_output() {
    let result = bash().exec("", opts(5)).await.unwrap();
    assert_eq!(result.exit_code, Some(0));
}
