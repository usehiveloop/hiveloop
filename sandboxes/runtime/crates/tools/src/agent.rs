use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use tokio::sync::mpsc;

use crate::ToolExecutor;

/// Tracks and limits the total number of subagent tasks spawned
/// within a single conversation's lifetime.
///
/// Shared (via `Arc`) across all subagent depths within a conversation,
/// ensuring a global ceiling on resource consumption regardless of nesting.
pub struct TaskBudget {
    /// Current count of spawned tasks (foreground + background).
    spawned: AtomicUsize,
    /// Maximum allowed tasks per conversation.
    max_tasks: usize,
}

impl TaskBudget {
    /// Create a new budget with the given maximum.
    pub fn new(max_tasks: usize) -> Self {
        Self {
            spawned: AtomicUsize::new(0),
            max_tasks,
        }
    }

    /// Try to acquire a task slot. Returns `Err` if budget exhausted.
    pub fn try_acquire(&self) -> Result<(), String> {
        // Optimistic increment — roll back on failure.
        let prev = self.spawned.fetch_add(1, Ordering::Relaxed);
        if prev >= self.max_tasks {
            self.spawned.fetch_sub(1, Ordering::Relaxed);
            Err(format!(
                "Task budget exhausted: {} of {} task slots used. \
                 Wait for existing tasks to complete before spawning more.",
                prev, self.max_tasks
            ))
        } else {
            Ok(())
        }
    }

    /// Try to acquire `n` task slots atomically. Returns `Err` if insufficient.
    pub fn try_acquire_many(&self, n: usize) -> Result<(), String> {
        let prev = self.spawned.fetch_add(n, Ordering::Relaxed);
        if prev + n > self.max_tasks {
            self.spawned.fetch_sub(n, Ordering::Relaxed);
            Err(format!(
                "Cannot spawn {} tasks: only {} of {} slots remaining.",
                n,
                self.max_tasks.saturating_sub(prev),
                self.max_tasks
            ))
        } else {
            Ok(())
        }
    }

    /// Returns the number of remaining task slots.
    pub fn remaining(&self) -> usize {
        self.max_tasks
            .saturating_sub(self.spawned.load(Ordering::Relaxed))
    }

    /// Returns the current number of spawned tasks.
    pub fn used(&self) -> usize {
        self.spawned.load(Ordering::Relaxed)
    }
}

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
    /// Shared task budget across the entire conversation tree.
    pub task_budget: Arc<TaskBudget>,
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

/// Parameters for the sub_agent tool.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct SubAgentToolParams {
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
    pub subagent_name: String,
    /// Set to true to run in background (returns immediately; result is injected
    /// into the next user turn as a `[Background Agent Task Completed]` message).
    #[schemars(
        description = "Set to true to run in background. Returns immediately with task_id; the final result is automatically injected into the next user turn when the subagent finishes."
    )]
    #[serde(default)]
    pub run_in_background: bool,
    /// Resume a previous subagent session by task_id.
    #[schemars(description = "Resume a previous subagent session by providing its task_id")]
    #[serde(default)]
    pub task_id: Option<String>,
}

/// Tool that invokes subagents for autonomous task execution.
pub struct SubAgentTool {
    description: String,
}

impl Default for SubAgentTool {
    fn default() -> Self {
        Self::new()
    }
}

impl SubAgentTool {
    pub fn new() -> Self {
        Self {
            description: String::new(),
        }
    }

    /// Build the description by replacing the {agents} placeholder with available subagents.
    #[cfg(test)]
    fn build_description(agents: &[(String, String)]) -> String {
        let template = include_str!("instructions/sub_agent.txt");
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
impl ToolExecutor for SubAgentTool {
    fn name(&self) -> &str {
        "sub_agent"
    }

    fn description(&self) -> &str {
        // Return the static description. The dynamic version with subagent list
        // is built in execute() since we need the task_local context.
        // For schema registration, the static template is sufficient.
        if self.description.is_empty() {
            include_str!("instructions/sub_agent.txt")
        } else {
            &self.description
        }
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(SubAgentToolParams))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let params: SubAgentToolParams =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        // Read context from task_local
        let ctx = AGENT_CONTEXT
            .try_with(|c| c.clone())
            .map_err(|_| "Sub-agent tool requires a conversation context".to_string())?;

        // Check task budget before spawning
        ctx.task_budget.try_acquire()?;

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            return Err(format!(
                "Maximum subagent depth ({}) reached",
                ctx.max_depth
            ));
        }

        // Validate subagent exists
        let available = ctx.runner.available_subagents();
        let subagent_exists = available
            .iter()
            .any(|(name, _)| name == &params.subagent_name);
        if !subagent_exists {
            if available.is_empty() {
                return Err(
                    "No subagents available. This agent has no subagents configured.".to_string(),
                );
            }
            let names: Vec<&str> = available.iter().map(|(n, _)| n.as_str()).collect();
            return Err(format!(
                "Unknown subagent '{}'. Available: [{}]",
                params.subagent_name,
                names.join(", ")
            ));
        }

        if params.run_in_background {
            // Background execution — result will arrive as a user-turn injection
            // via the notification channel when the subagent finishes.
            let handle = ctx
                .runner
                .run_background(&params.subagent_name, &params.prompt, &params.description)
                .await?;

            serde_json::to_string(&serde_json::json!({
                "task_id": handle.task_id,
                "status": "running",
                "message": "Background subagent started. Its final output will appear in your next user turn — do not poll or wait."
            }))
            .map_err(|e| format!("Failed to serialize result: {e}"))
        } else {
            // Foreground execution
            let result = ctx
                .runner
                .run_foreground(
                    &params.subagent_name,
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

    fn as_any(&self) -> &dyn std::any::Any {
        self
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
            task_budget: Arc::new(TaskBudget::new(50)),
        }
    }

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

    // ── Fix #4: TaskBudget tests ───────────────────────────────────────

    #[test]
    fn test_task_budget_basic_acquire() {
        let budget = TaskBudget::new(3);
        assert_eq!(budget.remaining(), 3);
        assert_eq!(budget.used(), 0);

        assert!(budget.try_acquire().is_ok());
        assert_eq!(budget.remaining(), 2);
        assert_eq!(budget.used(), 1);

        assert!(budget.try_acquire().is_ok());
        assert!(budget.try_acquire().is_ok());
        assert_eq!(budget.remaining(), 0);
        assert_eq!(budget.used(), 3);
    }

    #[test]
    fn test_task_budget_exhaustion() {
        let budget = TaskBudget::new(2);
        assert!(budget.try_acquire().is_ok());
        assert!(budget.try_acquire().is_ok());

        let err = budget.try_acquire();
        assert!(err.is_err());
        assert!(err.unwrap_err().contains("Task budget exhausted"));
        // Should not have incremented past max
        assert_eq!(budget.used(), 2);
    }

    #[test]
    fn test_task_budget_acquire_many_success() {
        let budget = TaskBudget::new(10);
        assert!(budget.try_acquire_many(5).is_ok());
        assert_eq!(budget.used(), 5);
        assert_eq!(budget.remaining(), 5);

        assert!(budget.try_acquire_many(5).is_ok());
        assert_eq!(budget.used(), 10);
        assert_eq!(budget.remaining(), 0);
    }

    #[test]
    fn test_task_budget_acquire_many_insufficient() {
        let budget = TaskBudget::new(5);
        assert!(budget.try_acquire_many(3).is_ok());

        let err = budget.try_acquire_many(5);
        assert!(err.is_err());
        assert!(err.unwrap_err().contains("Cannot spawn 5 tasks"));
        // Should have rolled back
        assert_eq!(budget.used(), 3);
    }

    #[test]
    fn test_task_budget_zero_max() {
        let budget = TaskBudget::new(0);
        assert_eq!(budget.remaining(), 0);
        assert!(budget.try_acquire().is_err());
    }

    #[test]
    fn test_task_budget_thread_safety() {
        use std::sync::Arc;
        let budget = Arc::new(TaskBudget::new(100));
        let handles: Vec<_> = (0..10)
            .map(|_| {
                let b = budget.clone();
                std::thread::spawn(move || {
                    let mut acquired = 0;
                    for _ in 0..20 {
                        if b.try_acquire().is_ok() {
                            acquired += 1;
                        }
                    }
                    acquired
                })
            })
            .collect();

        let total: usize = handles
            .into_iter()
            .map(|h| h.join().unwrap())
            .collect::<Vec<_>>()
            .into_iter()
            .sum();
        assert_eq!(
            total, 100,
            "exactly 100 slots should be acquired across all threads"
        );
        assert_eq!(budget.used(), 100);
        assert_eq!(budget.remaining(), 0);
    }

    #[tokio::test]
    async fn test_task_budget_enforced_by_sub_agent_tool() {
        // Budget of 1 — second call should fail
        let budget = Arc::new(TaskBudget::new(1));
        let (tx, _rx) = mpsc::channel(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner {
                subagents: vec![("coder".to_string(), "A coding agent".to_string())],
            }),
            notification_tx: tx,
            depth: 0,
            max_depth: 3,
            task_budget: budget,
        };

        let tool = SubAgentTool::new();
        let args = serde_json::json!({
            "description": "test",
            "prompt": "do something",
            "subagentName": "coder"
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
