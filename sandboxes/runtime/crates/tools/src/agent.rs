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
pub struct AgentToolParams {
    /// Short (3-5 word) description of the task.
    #[schemars(description = "Short (3-5 word) description of the task. Example: 'Fix login bug'")]
    pub description: String,
    /// The detailed task for the subagent to perform.
    #[schemars(
        description = "The detailed task for the subagent to perform. Be specific and include all necessary context"
    )]
    pub prompt: String,
    /// Which subagent to invoke (must match a defined subagent name).
    #[schemars(description = "Which subagent to invoke. Must match an available subagent name")]
    pub subagent: String,
    /// Set to true to run in background (returns immediately, notifies on completion).
    #[schemars(
        description = "Set to true to run in background. Returns immediately with task_id; notifies on completion"
    )]
    #[serde(default)]
    pub background: bool,
    /// Resume a previous subagent session by task_id.
    #[schemars(description = "Resume a previous subagent session by providing its task_id")]
    #[serde(default)]
    pub task_id: Option<String>,
}

/// Tool that invokes subagents for autonomous task execution.
pub struct AgentTool {
    description: String,
}

impl Default for AgentTool {
    fn default() -> Self {
        Self::new()
    }
}

impl AgentTool {
    pub fn new() -> Self {
        Self {
            description: String::new(),
        }
    }

    /// Build the description by replacing the {agents} placeholder with available subagents.
    #[cfg(test)]
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
                .run_foreground(&params.subagent, &params.prompt, params.task_id.as_deref())
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
        };

        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "delayed task",
            "prompt": "do something slow",
            "subagent": "coder"
        });

        let start = Instant::now();
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        let elapsed = start.elapsed();

        assert!(result.is_ok());
        assert!(elapsed >= tokio::time::Duration::from_millis(100), 
            "foreground should block for at least 100ms, got {:?}", elapsed);
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
        };

        let tool = AgentTool::new();
        let args = serde_json::json!({
            "description": "background task",
            "prompt": "do something slow in background",
            "subagent": "coder",
            "background": true
        });

        let start = Instant::now();
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        let elapsed = start.elapsed();

        assert!(result.is_ok());
        assert!(elapsed < tokio::time::Duration::from_millis(50),
            "background should return immediately, got {:?}", elapsed);
        assert_eq!(CALL_COUNT.load(Ordering::SeqCst), 1);
        
        let output = result.unwrap();
        let parsed: serde_json::Value = serde_json::from_str(&output).unwrap();
        assert_eq!(parsed["task_id"], "bg-delayed-456");
        assert_eq!(parsed["status"], "running");
    }
}
