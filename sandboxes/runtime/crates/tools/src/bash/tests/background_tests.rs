use super::super::args::BashResult;
use super::super::tool::BashTool;
use super::common::{make_context, MockRunner};
use crate::agent::{AgentContext, TaskBudget, AGENT_CONTEXT};
use crate::ToolExecutor;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;

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
async fn test_bash_background_delivers_notification() {
    let (tx, mut rx) = mpsc::channel(16);
    let ctx = AgentContext {
        runner: Arc::new(MockRunner),
        notification_tx: tx,
        depth: 0,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    };

    let tool = BashTool::new();
    let args = serde_json::json!({
        "command": "echo background_bash_output",
        "background": true,
        "description": "test notification delivery"
    });

    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;

    assert!(result.is_ok());
    let output = result.unwrap();
    let parsed: serde_json::Value = serde_json::from_str(&output).expect("parse JSON");
    let task_id = parsed["task_id"].as_str().unwrap();

    let notification = tokio::time::timeout(Duration::from_secs(5), rx.recv())
        .await
        .expect("notification should arrive")
        .expect("channel should not be closed");

    assert_eq!(notification.task_id, task_id);
    assert!(notification
        .output
        .expect("should be Ok")
        .contains("background_bash_output"));
}
