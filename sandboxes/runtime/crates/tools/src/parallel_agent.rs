//! Parallel agent tool for spawning multiple subagents concurrently.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::agent::AGENT_CONTEXT;
use crate::ToolExecutor;

/// A single subagent task specification.
#[derive(Debug, Deserialize, JsonSchema, Clone)]
pub struct SubAgentTask {
    /// Short (3-5 word) description of the task.
    pub description: String,
    /// The detailed task for the subagent to perform.
    pub prompt: String,
    /// Which subagent to invoke.
    pub subagent: String,
}

/// Arguments for the parallel agent tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct ParallelAgentArgs {
    /// List of subagent tasks to execute in parallel.
    pub tasks: Vec<SubAgentTask>,
    /// Optional timeout in seconds for all tasks. Default: 300 (5 minutes).
    #[serde(default = "default_timeout_secs")]
    pub timeout_secs: u64,
    /// Maximum number of concurrent subagents. Default: 5.
    #[serde(default = "default_max_concurrent")]
    pub max_concurrent: usize,
}

fn default_timeout_secs() -> u64 {
    300
}

fn default_max_concurrent() -> usize {
    5
}

/// Result of a single subagent task.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ParallelTaskResult {
    pub description: String,
    pub subagent: String,
    pub status: String, // "completed", "failed", "timeout"
    #[serde(skip_serializing_if = "Option::is_none")]
    pub task_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// Overall result of the parallel execution.
#[derive(Debug, Serialize, Deserialize)]
pub struct ParallelAgentResult {
    pub results: Vec<ParallelTaskResult>,
    pub all_succeeded: bool,
    pub total: usize,
    pub succeeded: usize,
    pub failed: usize,
    pub elapsed_secs: f64,
}

/// Semaphore for limiting concurrent subagent execution.
pub struct ConcurrencyLimiter {
    semaphore: tokio::sync::Semaphore,
}

impl ConcurrencyLimiter {
    pub fn new(max_concurrent: usize) -> Self {
        Self {
            semaphore: tokio::sync::Semaphore::new(max_concurrent),
        }
    }

    pub async fn acquire(&self) -> tokio::sync::SemaphorePermit<'_> {
        self.semaphore
            .acquire()
            .await
            .expect("Semaphore should never be closed")
    }
}

/// Tool that spawns multiple subagents in parallel and waits for all to complete.
pub struct ParallelAgentTool;

impl ParallelAgentTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for ParallelAgentTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for ParallelAgentTool {
    fn name(&self) -> &str {
        "parallel_agent"
    }

    fn description(&self) -> &str {
        include_str!("instructions/parallel_agent.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(ParallelAgentArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: ParallelAgentArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.tasks.is_empty() {
            return Err("No tasks provided".to_string());
        }

        if args.tasks.len() > 25 {
            return Err("Maximum 25 tasks allowed".to_string());
        }

        // Get context
        let ctx = AGENT_CONTEXT
            .try_with(|c| c.clone())
            .map_err(|_| "Parallel agent tool requires a conversation context".to_string())?;

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            return Err(format!(
                "Maximum subagent depth ({}) reached",
                ctx.max_depth
            ));
        }

        // Validate all subagents exist
        let available = ctx.runner.available_subagents();
        for task in &args.tasks {
            let exists = available.iter().any(|(name, _)| name == &task.subagent);
            if !exists {
                let names: Vec<&str> = available.iter().map(|(n, _)| n.as_str()).collect();
                return Err(format!(
                    "Unknown subagent '{}'. Available: [{}]",
                    task.subagent,
                    names.join(", ")
                ));
            }
        }

        let start = std::time::Instant::now();
        let limiter = Arc::new(ConcurrencyLimiter::new(args.max_concurrent));
        let timeout = std::time::Duration::from_secs(args.timeout_secs);

        // Spawn all tasks with concurrency limiting
        let mut handles = Vec::new();

        for task in args.tasks.clone() {
            let runner = ctx.runner.clone();
            let limiter = limiter.clone();
            let _notification_tx = ctx.notification_tx.clone();
            let _depth = ctx.depth + 1;
            let _max_depth = ctx.max_depth;

            let handle = tokio::spawn(async move {
                // Acquire permit (blocks if at max concurrency)
                let _permit = limiter.acquire().await;

                // Run the subagent
                let result = tokio::time::timeout(timeout, async {
                    runner
                        .run_foreground(&task.subagent, &task.prompt, None)
                        .await
                })
                .await;

                match result {
                    Ok(Ok(agent_result)) => ParallelTaskResult {
                        description: task.description.clone(),
                        subagent: task.subagent.clone(),
                        status: "completed".to_string(),
                        task_id: Some(agent_result.task_id),
                        output: Some(agent_result.output),
                        error: None,
                    },
                    Ok(Err(e)) => ParallelTaskResult {
                        description: task.description.clone(),
                        subagent: task.subagent.clone(),
                        status: "failed".to_string(),
                        task_id: None,
                        output: None,
                        error: Some(e),
                    },
                    Err(_) => ParallelTaskResult {
                        description: task.description.clone(),
                        subagent: task.subagent.clone(),
                        status: "timeout".to_string(),
                        task_id: None,
                        output: None,
                        error: Some(format!("Timeout after {}s", args.timeout_secs)),
                    },
                }
            });

            handles.push(handle);
        }

        // Wait for all tasks to complete
        let mut results = Vec::new();
        for handle in handles {
            match handle.await {
                Ok(result) => results.push(result),
                Err(e) => results.push(ParallelTaskResult {
                    description: "unknown".to_string(),
                    subagent: "unknown".to_string(),
                    status: "failed".to_string(),
                    task_id: None,
                    output: None,
                    error: Some(format!("Task panicked: {e}")),
                }),
            }
        }

        let elapsed = start.elapsed();
        let total = results.len();
        let succeeded = results.iter().filter(|r| r.status == "completed").count();
        let failed = total - succeeded;

        let parallel_result = ParallelAgentResult {
            all_succeeded: failed == 0,
            total,
            succeeded,
            failed,
            elapsed_secs: elapsed.as_secs_f64(),
            results,
        };

        serde_json::to_string_pretty(&parallel_result)
            .map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::{AgentContext, AgentTaskHandle, SubAgentRunner};
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Mutex;
    use tokio::sync::mpsc;

    struct MockRunner {
        call_count: AtomicUsize,
        call_order: Mutex<Vec<String>>,
        delay_ms: u64,
    }

    #[async_trait]
    impl SubAgentRunner for MockRunner {
        fn available_subagents(&self) -> Vec<(String, String)> {
            vec![
                ("explorer".to_string(), "Explorer agent".to_string()),
                ("coder".to_string(), "Coder agent".to_string()),
            ]
        }

        async fn run_foreground(
            &self,
            subagent: &str,
            prompt: &str,
            _task_id: Option<&str>,
        ) -> Result<crate::agent::AgentTaskResult, String> {
            let count = self.call_count.fetch_add(1, Ordering::SeqCst);
            self.call_order.lock().unwrap().push(subagent.to_string());

            // Simulate work
            if self.delay_ms > 0 {
                tokio::time::sleep(tokio::time::Duration::from_millis(self.delay_ms)).await;
            }

            Ok(crate::agent::AgentTaskResult {
                task_id: format!("task-{}", count),
                output: format!("Result from {} for: {}", subagent, prompt),
            })
        }

        async fn run_background(
            &self,
            _subagent: &str,
            _prompt: &str,
            _description: &str,
        ) -> Result<AgentTaskHandle, String> {
            unreachable!("background should not be called in these tests")
        }
    }

    fn make_context(delay_ms: u64) -> AgentContext {
        let (tx, _rx) = mpsc::channel(16);
        AgentContext {
            runner: Arc::new(MockRunner {
                call_count: AtomicUsize::new(0),
                call_order: Mutex::new(Vec::new()),
                delay_ms,
            }),
            notification_tx: tx,
            task_registry: None,
            depth: 0,
            max_depth: 3,
        }
    }

    #[tokio::test]
    async fn test_parallel_agent_empty_tasks() {
        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [],
            "timeout_secs": 10,
            "max_concurrent": 5
        });

        let result = tool.execute(args).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("No tasks"));
    }

    #[tokio::test]
    async fn test_parallel_agent_unknown_subagent() {
        let ctx = make_context(0);
        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [
                {"description": "test", "prompt": "do something", "subagent": "unknown"}
            ],
            "timeout_secs": 10,
            "max_concurrent": 5
        });

        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;

        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Unknown subagent"));
    }

    #[tokio::test]
    async fn test_parallel_agent_runs_concurrently() {
        let ctx = make_context(100); // 100ms delay per task
        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [
                {"description": "task 1", "prompt": "find files", "subagent": "explorer"},
                {"description": "task 2", "prompt": "write code", "subagent": "coder"},
                {"description": "task 3", "prompt": "find more", "subagent": "explorer"}
            ],
            "timeout_secs": 10,
            "max_concurrent": 5
        });

        let start = std::time::Instant::now();
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        let elapsed = start.elapsed();

        assert!(result.is_ok(), "Result: {:?}", result);

        // Should complete in ~100ms (parallel), not ~300ms (sequential)
        assert!(
            elapsed < std::time::Duration::from_millis(250),
            "Should run in parallel (~100ms), took {:?}",
            elapsed
        );

        let parsed: ParallelAgentResult = serde_json::from_str(&result.unwrap()).unwrap();
        assert_eq!(parsed.total, 3);
        assert_eq!(parsed.succeeded, 3);
        assert!(parsed.all_succeeded);
    }

    #[tokio::test]
    async fn test_parallel_agent_respects_concurrency_limit() {
        let ctx = make_context(50); // 50ms delay per task
        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [
                {"description": "task 1", "prompt": "work", "subagent": "explorer"},
                {"description": "task 2", "prompt": "work", "subagent": "explorer"},
                {"description": "task 3", "prompt": "work", "subagent": "explorer"},
                {"description": "task 4", "prompt": "work", "subagent": "explorer"}
            ],
            "timeout_secs": 10,
            "max_concurrent": 2  // Only 2 at a time
        });

        let start = std::time::Instant::now();
        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;
        let elapsed = start.elapsed();

        assert!(result.is_ok());

        // With 4 tasks, 50ms each, max 2 concurrent:
        // Batch 1: tasks 1&2 (50ms)
        // Batch 2: tasks 3&4 (50ms)
        // Total: ~100ms, not ~200ms (sequential) or ~50ms (unlimited parallel)
        assert!(
            elapsed >= std::time::Duration::from_millis(80),
            "Should take at least ~100ms with limit 2, took {:?}",
            elapsed
        );
        assert!(
            elapsed < std::time::Duration::from_millis(180),
            "Should complete in ~100ms, took {:?}",
            elapsed
        );
    }

    #[tokio::test]
    async fn test_parallel_agent_depth_limit() {
        let (tx, _rx) = mpsc::channel(16);
        let ctx = AgentContext {
            runner: Arc::new(MockRunner {
                call_count: AtomicUsize::new(0),
                call_order: Mutex::new(Vec::new()),
                delay_ms: 0,
            }),
            notification_tx: tx,
            task_registry: None,
            depth: 3, // At max depth
            max_depth: 3,
        };

        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [
                {"description": "task 1", "prompt": "work", "subagent": "explorer"}
            ],
            "timeout_secs": 10,
            "max_concurrent": 5
        });

        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;

        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Maximum subagent depth"));
    }

    #[tokio::test]
    async fn test_parallel_agent_handles_partial_failure() {
        struct FailingMockRunner;

        #[async_trait]
        impl SubAgentRunner for FailingMockRunner {
            fn available_subagents(&self) -> Vec<(String, String)> {
                vec![
                    ("explorer".to_string(), "Explorer".to_string()),
                    ("coder".to_string(), "Coder".to_string()),
                ]
            }

            async fn run_foreground(
                &self,
                subagent: &str,
                _prompt: &str,
                _task_id: Option<&str>,
            ) -> Result<crate::agent::AgentTaskResult, String> {
                if subagent == "coder" {
                    Err("Coder failed!".to_string())
                } else {
                    Ok(crate::agent::AgentTaskResult {
                        task_id: "task-ok".to_string(),
                        output: "Success".to_string(),
                    })
                }
            }

            async fn run_background(
                &self,
                _subagent: &str,
                _prompt: &str,
                _description: &str,
            ) -> Result<AgentTaskHandle, String> {
                unreachable!()
            }
        }

        let (tx, _rx) = mpsc::channel(16);
        let ctx = AgentContext {
            runner: Arc::new(FailingMockRunner),
            notification_tx: tx,
            task_registry: None,
            depth: 0,
            max_depth: 3,
        };

        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [
                {"description": "task 1", "prompt": "work", "subagent": "explorer"},
                {"description": "task 2", "prompt": "work", "subagent": "coder"},  // Will fail
                {"description": "task 3", "prompt": "work", "subagent": "explorer"}
            ],
            "timeout_secs": 10,
            "max_concurrent": 5
        });

        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;

        assert!(result.is_ok());
        let parsed: ParallelAgentResult = serde_json::from_str(&result.unwrap()).unwrap();
        assert_eq!(parsed.total, 3);
        assert_eq!(parsed.succeeded, 2);
        assert_eq!(parsed.failed, 1);
        assert!(!parsed.all_succeeded);
    }

    #[tokio::test]
    async fn test_parallel_agent_timeout() {
        let ctx = make_context(5000); // 5 second delay
        let tool = ParallelAgentTool::new();
        let args = serde_json::json!({
            "tasks": [
                {"description": "slow task", "prompt": "work", "subagent": "explorer"}
            ],
            "timeout_secs": 1,  // 1 second timeout
            "max_concurrent": 5
        });

        let result = AGENT_CONTEXT
            .scope(ctx, async { tool.execute(args).await })
            .await;

        assert!(result.is_ok());
        let parsed: ParallelAgentResult = serde_json::from_str(&result.unwrap()).unwrap();
        assert_eq!(parsed.failed, 1);
        assert!(parsed.results[0]
            .error
            .as_ref()
            .unwrap()
            .contains("Timeout"));
    }
}
