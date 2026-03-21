//! Self-delegation agent tool. Launches a clone of the parent agent
//! to handle a focused task autonomously.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;

use crate::agent::AGENT_CONTEXT;
use crate::ToolExecutor;

/// Reserved subagent name for the self-delegation entry.
pub const SELF_AGENT_NAME: &str = "__self__";

/// Parameters for the self-delegation agent tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct AgentToolParams {
    /// Short (3-5 word) description of the task.
    #[schemars(description = "Short (3-5 word) description of the task. Example: 'Fix login bug'")]
    pub description: String,
    /// The detailed task for the agent to perform.
    #[schemars(
        description = "The detailed task for the agent to perform. Be specific and include all necessary context"
    )]
    pub prompt: String,
    /// Set to true to run in background (returns immediately, notifies on completion).
    #[schemars(
        description = "Set to true to run in background. Returns immediately with task_id; notifies on completion"
    )]
    #[serde(default)]
    pub background: bool,
    /// Resume a previous agent session by task_id.
    #[schemars(description = "Resume a previous agent session by providing its task_id")]
    #[serde(default)]
    pub task_id: Option<String>,
}

/// Tool that launches a clone of the parent agent for self-delegation.
pub struct AgentTool;

impl Default for AgentTool {
    fn default() -> Self {
        Self::new()
    }
}

impl AgentTool {
    pub fn new() -> Self {
        Self
    }
}

#[async_trait]
impl ToolExecutor for AgentTool {
    fn name(&self) -> &str {
        "agent"
    }

    fn description(&self) -> &str {
        include_str!("instructions/self_agent.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(AgentToolParams))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let params: AgentToolParams =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        // Read context from task_local
        let ctx = AGENT_CONTEXT
            .try_with(|c| c.clone())
            .map_err(|_| "Agent tool requires a conversation context".to_string())?;

        // Check task budget before spawning
        ctx.task_budget.try_acquire()?;

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            return Err(format!("Maximum agent depth ({}) reached", ctx.max_depth));
        }

        if params.background {
            // Background execution
            let handle = ctx
                .runner
                .run_background(SELF_AGENT_NAME, &params.prompt, &params.description)
                .await?;

            serde_json::to_string(&serde_json::json!({
                "task_id": handle.task_id,
                "status": "running",
                "message": "Background task started. You will be notified when it completes."
            }))
            .map_err(|e| format!("Failed to serialize result: {e}"))
        } else {
            // Foreground execution
            let result = ctx
                .runner
                .run_foreground(SELF_AGENT_NAME, &params.prompt, params.task_id.as_deref())
                .await?;

            Ok(format!(
                "task_id: {} (for resuming)\n\n<task_result>\n{}\n</task_result>",
                result.task_id, result.output
            ))
        }
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::{
        AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, TaskBudget, AGENT_CONTEXT,
    };
    use std::sync::Arc;
    use tokio::sync::mpsc;

    struct MockRunner;

    #[async_trait]
    impl SubAgentRunner for MockRunner {
        fn available_subagents(&self) -> Vec<(String, String)> {
            vec![]
        }

        async fn run_foreground(
            &self,
            subagent: &str,
            prompt: &str,
            _task_id: Option<&str>,
        ) -> Result<AgentTaskResult, String> {
            assert_eq!(subagent, SELF_AGENT_NAME);
            Ok(AgentTaskResult {
                task_id: "self-task-123".to_string(),
                output: format!("Self-delegation result for: {}", prompt),
            })
        }

        async fn run_background(
            &self,
            subagent: &str,
            _prompt: &str,
            _description: &str,
        ) -> Result<AgentTaskHandle, String> {
            assert_eq!(subagent, SELF_AGENT_NAME);
            Ok(AgentTaskHandle {
                task_id: "self-bg-456".to_string(),
            })
        }
    }

    fn make_context() -> AgentContext {
        let (tx, _rx) = mpsc::channel(16);
        AgentContext {
            runner: Arc::new(MockRunner),
            notification_tx: tx,
            task_registry: None,
            depth: 0,
            max_depth: 3,
            task_budget: Arc::new(TaskBudget::new(50)),
        }
    }

    #[tokio::test]
    async fn test_no_context_returns_error() {
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something"
        });
        let result = tool.execute(args).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("conversation context"));
    }

    #[tokio::test]
    async fn test_foreground_execution() {
        let ctx = make_context();
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test task",
            "prompt": "write hello world"
        });
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        assert!(result.is_ok());
        let output = result.unwrap();
        assert!(output.contains("self-task-123"));
        assert!(output.contains("Self-delegation result"));
        assert!(output.contains("<task_result>"));
    }

    #[tokio::test]
    async fn test_background_execution() {
        let ctx = make_context();
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test task",
            "prompt": "write hello world",
            "background": true
        });
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        assert!(result.is_ok());
        let output = result.unwrap();
        let parsed: serde_json::Value = serde_json::from_str(&output).unwrap();
        assert_eq!(parsed["task_id"], "self-bg-456");
        assert_eq!(parsed["status"], "running");
    }

    #[tokio::test]
    async fn test_depth_limit_exceeded() {
        let (tx, _rx) = mpsc::channel(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner),
            notification_tx: tx,
            task_registry: None,
            depth: 3,
            max_depth: 3,
            task_budget: Arc::new(TaskBudget::new(50)),
        };
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something"
        });
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Maximum agent depth"));
    }

    #[tokio::test]
    async fn test_task_budget_enforced() {
        let budget = Arc::new(TaskBudget::new(1));
        let (tx, _rx) = mpsc::channel(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner),
            notification_tx: tx,
            task_registry: None,
            depth: 0,
            max_depth: 3,
            task_budget: budget,
        };

        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something"
        });

        // First call should succeed
        let result1 = AGENT_CONTEXT
            .scope(ctx.clone(), async { tool.execute(args.clone()).await })
            .await;
        assert!(result1.is_ok());

        // Second call should fail — budget exhausted
        let result2 = AGENT_CONTEXT
            .scope(ctx.clone(), async { tool.execute(args.clone()).await })
            .await;
        assert!(result2.is_err());
        assert!(result2.unwrap_err().contains("Task budget exhausted"));
    }
}
