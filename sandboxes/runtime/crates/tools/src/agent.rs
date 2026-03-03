use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;
use std::sync::Arc;
use tokio::sync::mpsc;

use crate::ToolExecutor;

/// Trait for running subagents. Defined in tools crate, implemented in runtime.
#[async_trait]
pub trait SubAgentRunner: Send + Sync {
    /// List available subagent names with descriptions.
    fn available_subagents(&self) -> Vec<(String, String)>;
    /// Run a subagent synchronously, blocking until completion.
    async fn run_foreground(
        &self,
        subagent: &str,
        prompt: &str,
        task_id: Option<&str>,
    ) -> Result<AgentTaskResult, String>;
    /// Spawn a subagent in the background, returns immediately with a task handle.
    async fn run_background(
        &self,
        subagent: &str,
        prompt: &str,
        description: &str,
    ) -> Result<AgentTaskHandle, String>;
}

/// Per-conversation context injected via task_local.
#[derive(Clone)]
pub struct AgentContext {
    pub runner: Arc<dyn SubAgentRunner>,
    pub notification_tx: mpsc::Sender<AgentTaskNotification>,
    pub depth: usize,
    pub max_depth: usize,
}

tokio::task_local! {
    pub static AGENT_CONTEXT: AgentContext;
}

/// Result from a completed subagent run.
pub struct AgentTaskResult {
    pub task_id: String,
    pub output: String,
}

/// Handle returned for background tasks.
pub struct AgentTaskHandle {
    pub task_id: String,
}

/// Notification sent when a background task completes.
pub struct AgentTaskNotification {
    pub task_id: String,
    pub description: String,
    pub output: Result<String, String>,
}

/// Parameters for the agent tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct AgentToolParams {
    /// Short (3-5 word) description of the task.
    description: String,
    /// The task for the subagent to perform.
    prompt: String,
    /// Which subagent to invoke (must match a defined subagent name).
    subagent: String,
    /// Set to true to run in background (returns immediately, notifies on completion).
    #[serde(default)]
    background: bool,
    /// Resume a previous subagent session by task_id.
    #[serde(default)]
    task_id: Option<String>,
}

/// Tool that invokes subagents for autonomous task execution.
pub struct AgentTool {
    description: String,
}

impl AgentTool {
    pub fn new() -> Self {
        Self {
            description: String::new(),
        }
    }

    /// Build the description by replacing the {agents} placeholder with available subagents.
    fn build_description(agents: &[(String, String)]) -> String {
        let template = include_str!("instructions/agent.txt");
        let agent_list = if agents.is_empty() {
            "(none)".to_string()
        } else {
            agents
                .iter()
                .map(|(name, desc)| format!("- {}: {}", name, desc))
                .collect::<Vec<_>>()
                .join("\n")
        };
        template.replace("{agents}", &agent_list)
    }
}

#[async_trait]
impl ToolExecutor for AgentTool {
    fn name(&self) -> &str {
        "agent"
    }

    fn description(&self) -> &str {
        // Return the static description. The dynamic version with subagent list
        // is built in execute() since we need the task_local context.
        // For schema registration, the static template is sufficient.
        if self.description.is_empty() {
            include_str!("instructions/agent.txt")
        } else {
            &self.description
        }
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

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            return Err(format!(
                "Maximum subagent depth ({}) reached",
                ctx.max_depth
            ));
        }

        // Validate subagent exists
        let available = ctx.runner.available_subagents();
        let subagent_exists = available.iter().any(|(name, _)| name == &params.subagent);
        if !subagent_exists {
            if available.is_empty() {
                return Err(
                    "No subagents available. This agent has no subagents configured.".to_string(),
                );
            }
            let names: Vec<&str> = available.iter().map(|(n, _)| n.as_str()).collect();
            return Err(format!(
                "Unknown subagent '{}'. Available: [{}]",
                params.subagent,
                names.join(", ")
            ));
        }

        if params.background {
            // Background execution
            let handle = ctx
                .runner
                .run_background(&params.subagent, &params.prompt, &params.description)
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
                .run_foreground(
                    &params.subagent,
                    &params.prompt,
                    params.task_id.as_deref(),
                )
                .await?;

            Ok(format!(
                "task_id: {} (for resuming)\n\n<task_result>\n{}\n</task_result>",
                result.task_id, result.output
            ))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct MockRunner {
        subagents: Vec<(String, String)>,
    }

    #[async_trait]
    impl SubAgentRunner for MockRunner {
        fn available_subagents(&self) -> Vec<(String, String)> {
            self.subagents.clone()
        }

        async fn run_foreground(
            &self,
            subagent: &str,
            prompt: &str,
            _task_id: Option<&str>,
        ) -> Result<AgentTaskResult, String> {
            Ok(AgentTaskResult {
                task_id: "test-task-123".to_string(),
                output: format!("Result from {} for: {}", subagent, prompt),
            })
        }

        async fn run_background(
            &self,
            _subagent: &str,
            _prompt: &str,
            _description: &str,
        ) -> Result<AgentTaskHandle, String> {
            Ok(AgentTaskHandle {
                task_id: "bg-task-456".to_string(),
            })
        }
    }

    fn make_context(subagents: Vec<(String, String)>) -> AgentContext {
        let (tx, _rx) = mpsc::channel(16);
        AgentContext {
            runner: Arc::new(MockRunner { subagents }),
            notification_tx: tx,
            depth: 0,
            max_depth: 3,
        }
    }

    #[tokio::test]
    async fn test_no_context_returns_error() {
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something",
            "subagent": "explorer"
        });
        let result = tool.execute(args).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("conversation context"));
    }

    #[tokio::test]
    async fn test_no_subagents_returns_error() {
        let ctx = make_context(vec![]);
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something",
            "subagent": "explorer"
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
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something",
            "subagent": "explorer"
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
        };
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something",
            "subagent": "coder"
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
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test task",
            "prompt": "write hello world",
            "subagent": "coder"
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
        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "test task",
            "prompt": "write hello world",
            "subagent": "coder",
            "background": true
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
        let desc = AgentTool::build_description(&agents);
        assert!(desc.contains("- coder: A coding agent"));
        assert!(desc.contains("- explorer: An exploration agent"));
        assert!(!desc.contains("{agents}"));
    }

    #[test]
    fn test_build_description_no_agents() {
        let desc = AgentTool::build_description(&[]);
        assert!(desc.contains("(none)"));
    }
}
