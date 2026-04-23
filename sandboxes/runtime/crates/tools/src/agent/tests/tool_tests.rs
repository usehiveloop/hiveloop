use super::common::{make_context, MockRunner};
use crate::agent::{
    AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, SubAgentTool, TaskBudget,
    AGENT_CONTEXT,
};
use crate::ToolExecutor;
use async_trait::async_trait;
use std::sync::Arc;
use tokio::sync::mpsc;

#[tokio::test]
async fn test_no_context_returns_error() {
    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test",
        "prompt": "do something",
        "subagentName": "explorer"
    });
    let result = tool.execute(args).await;
    assert!(result.is_err());
    assert!(result.unwrap_err().contains("conversation context"));
}

#[tokio::test]
async fn test_no_subagents_returns_error() {
    let ctx = make_context(vec![]);
    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test",
        "prompt": "do something",
        "subagentName": "explorer"
    });
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    assert!(result.is_err());
    assert!(result.unwrap_err().contains("No subagents available"));
}

#[tokio::test]
async fn test_unknown_subagent_returns_error() {
    let ctx = make_context(vec![("coder".to_string(), "A coding agent".to_string())]);
    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test",
        "prompt": "do something",
        "subagentName": "explorer"
    });
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    assert!(result.is_err());
    let err = result.unwrap_err();
    assert!(err.contains("Unknown subagent 'explorer'"));
    assert!(err.contains("coder"));
}

#[tokio::test]
async fn test_depth_limit_exceeded() {
    let (tx, _rx) = mpsc::channel(16);
    let ctx = AgentContext {
        runner: Arc::new(MockRunner {
            subagents: vec![("coder".to_string(), "A coding agent".to_string())],
        }),
        notification_tx: tx,
        depth: 3,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    };
    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test",
        "prompt": "do something",
        "subagentName": "coder"
    });
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    assert!(result.is_err());
    assert!(result.unwrap_err().contains("Maximum subagent depth"));
}

#[tokio::test]
async fn test_foreground_execution() {
    let ctx = make_context(vec![("coder".to_string(), "A coding agent".to_string())]);
    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test task",
        "prompt": "write hello world",
        "subagentName": "coder"
    });
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    assert!(result.is_ok());
    let output = result.unwrap();
    assert!(output.contains("test-task-123"));
    assert!(output.contains("Result from coder"));
    assert!(output.contains("<task_result>"));
}

#[tokio::test]
async fn test_background_execution() {
    let ctx = make_context(vec![("coder".to_string(), "A coding agent".to_string())]);
    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "test task",
        "prompt": "write hello world",
        "subagentName": "coder",
        "runInBackground": true
    });
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    assert!(result.is_ok());
    let output = result.unwrap();
    let parsed: serde_json::Value = serde_json::from_str(&output).unwrap();
    assert_eq!(parsed["task_id"], "bg-task-456");
    assert_eq!(parsed["status"], "running");
}

#[test]
fn test_build_description_with_agents() {
    let agents = vec![
        ("coder".to_string(), "A coding agent".to_string()),
        ("explorer".to_string(), "An exploration agent".to_string()),
    ];
    let desc = SubAgentTool::build_description(&agents);
    assert!(desc.contains("- coder: A coding agent"));
    assert!(desc.contains("- explorer: An exploration agent"));
    assert!(!desc.contains("{agents}"));
}

#[test]
fn test_build_description_no_agents() {
    let desc = SubAgentTool::build_description(&[]);
    assert!(desc.contains("(none)"));
}

#[tokio::test]
async fn test_foreground_blocks_until_complete() {
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::time::Instant;

    static CALL_COUNT: AtomicUsize = AtomicUsize::new(0);

    struct DelayedMockRunner;

    #[async_trait]
    impl SubAgentRunner for DelayedMockRunner {
        fn available_subagents(&self) -> Vec<(String, String)> {
            vec![("coder".to_string(), "A coding agent".to_string())]
        }

        async fn run_foreground(
            &self,
            _subagent: &str,
            _prompt: &str,
            _task_id: Option<&str>,
        ) -> Result<AgentTaskResult, String> {
            CALL_COUNT.fetch_add(1, Ordering::SeqCst);
            tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
            Ok(AgentTaskResult {
                task_id: "delayed-task-123".to_string(),
                output: "delayed result".to_string(),
            })
        }

        async fn run_background(
            &self,
            _subagent: &str,
            _prompt: &str,
            _description: &str,
        ) -> Result<AgentTaskHandle, String> {
            unreachable!("background should not be called in this test")
        }
    }

    let (tx, _rx) = mpsc::channel(16);
    let ctx = AgentContext {
        runner: Arc::new(DelayedMockRunner),
        notification_tx: tx,
        depth: 0,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    };

    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "delayed task",
        "prompt": "do something slow",
        "subagentName": "coder"
    });

    let start = Instant::now();
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    let elapsed = start.elapsed();

    assert!(result.is_ok());
    assert!(
        elapsed >= tokio::time::Duration::from_millis(100),
        "foreground should block for at least 100ms, got {:?}",
        elapsed
    );
    assert_eq!(CALL_COUNT.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn test_background_returns_immediately() {
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::time::Instant;

    static CALL_COUNT: AtomicUsize = AtomicUsize::new(0);

    struct DelayedMockRunner;

    #[async_trait]
    impl SubAgentRunner for DelayedMockRunner {
        fn available_subagents(&self) -> Vec<(String, String)> {
            vec![("coder".to_string(), "A coding agent".to_string())]
        }

        async fn run_foreground(
            &self,
            _subagent: &str,
            _prompt: &str,
            _task_id: Option<&str>,
        ) -> Result<AgentTaskResult, String> {
            unreachable!("foreground should not be called in this test")
        }

        async fn run_background(
            &self,
            _subagent: &str,
            _prompt: &str,
            _description: &str,
        ) -> Result<AgentTaskHandle, String> {
            CALL_COUNT.fetch_add(1, Ordering::SeqCst);
            // Simulate slow operation that continues after return
            tokio::spawn(async {
                tokio::time::sleep(tokio::time::Duration::from_secs(10)).await;
            });
            Ok(AgentTaskHandle {
                task_id: "bg-delayed-456".to_string(),
            })
        }
    }

    let (tx, _rx) = mpsc::channel(16);
    let ctx = AgentContext {
        runner: Arc::new(DelayedMockRunner),
        notification_tx: tx,
        depth: 0,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    };

    let tool = SubAgentTool::new();
    let args = serde_json::json!({
        "description": "background task",
        "prompt": "do something slow in background",
        "subagentName": "coder",
        "runInBackground": true
    });

    let start = Instant::now();
    let result = AGENT_CONTEXT
        .scope(ctx, async { tool.execute(args).await })
        .await;
    let elapsed = start.elapsed();

    assert!(result.is_ok());
    assert!(
        elapsed < tokio::time::Duration::from_millis(50),
        "background should return immediately, got {:?}",
        elapsed
    );
    assert_eq!(CALL_COUNT.load(Ordering::SeqCst), 1);

    let output = result.unwrap();
    let parsed: serde_json::Value = serde_json::from_str(&output).unwrap();
    assert_eq!(parsed["task_id"], "bg-delayed-456");
    assert_eq!(parsed["status"], "running");
}
