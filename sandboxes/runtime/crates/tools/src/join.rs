//! Join tool for waiting on multiple background subagent tasks.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::{mpsc, oneshot};

use crate::ToolExecutor;

/// Arguments for the join tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct JoinArgs {
    /// List of task IDs to wait for.
    pub task_ids: Vec<String>,
    /// Optional timeout in seconds. Default: 300 (5 minutes).
    #[serde(default = "default_timeout_secs")]
    pub timeout_secs: u64,
}

fn default_timeout_secs() -> u64 {
    300
}

/// Result of a single task in the join.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TaskResult {
    pub task_id: String,
    pub status: String, // "completed", "failed", "timeout", "not_found"
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// Overall result of the join operation.
#[derive(Debug, Serialize, Deserialize)]
pub struct JoinResult {
    pub completed: Vec<TaskResult>,
    pub all_succeeded: bool,
    pub total: usize,
    pub succeeded: usize,
    pub failed: usize,
    pub not_found: usize,
}

/// Request to the join monitor.
pub enum JoinRequest {
    /// Register a task ID to wait for.
    Register {
        task_id: String,
        response_tx: oneshot::Sender<TaskResult>,
    },
    /// Check if a task is already completed.
    CheckCompleted(String),
}

/// A handle for tasks to signal completion.
pub struct TaskCompletionHandle {
    pub tx: mpsc::Sender<TaskCompletion>,
}

/// Task completion notification.
pub struct TaskCompletion {
    pub task_id: String,
    pub output: Result<String, String>,
}

/// Background task registry for tracking and joining tasks.
pub struct TaskRegistry {
    /// Pending tasks waiting for completion.
    pending: dashmap::DashMap<String, Vec<oneshot::Sender<TaskResult>>>,
    /// Completed task results.
    completed: dashmap::DashMap<String, TaskResult>,
}

impl TaskRegistry {
    pub fn new() -> Self {
        Self {
            pending: dashmap::DashMap::new(),
            completed: dashmap::DashMap::new(),
        }
    }

    /// Register interest in a task. Returns immediately if already completed.
    pub fn register(
        &self,
        task_id: String,
    ) -> Option<oneshot::Receiver<TaskResult>> {
        // Check if already completed
        if let Some(result) = self.completed.get(&task_id) {
            let (tx, rx) = oneshot::channel();
            let _ = tx.send(result.clone());
            return Some(rx);
        }

        // Register as pending
        let (tx, rx) = oneshot::channel();
        self.pending
            .entry(task_id)
            .or_insert_with(Vec::new)
            .push(tx);
        Some(rx)
    }

    /// Mark a task as completed and notify all waiters.
    pub fn complete(&self, task_id: String, output: Result<String, String>) {
        let status = if output.is_ok() {
            "completed"
        } else {
            "failed"
        };

        let result = TaskResult {
            task_id: task_id.clone(),
            status: status.to_string(),
            output: output.as_ref().ok().cloned(),
            error: output.as_ref().err().cloned(),
        };

        // Store completed result
        self.completed.insert(task_id.clone(), result.clone());

        // Notify all pending waiters
        if let Some((_, waiters)) = self.pending.remove(&task_id) {
            for tx in waiters {
                let _ = tx.send(result.clone());
            }
        }
    }

    /// Check if a task is completed.
    pub fn is_completed(&self, task_id: &str) -> Option<TaskResult> {
        self.completed.get(task_id).map(|r| r.clone())
    }

    /// Get all completed tasks.
    pub fn get_completed(&self, task_ids: &[String]) -> Vec<TaskResult> {
        task_ids
            .iter()
            .filter_map(|id| self.completed.get(id).map(|r| r.clone()))
            .collect()
    }
}

impl Default for TaskRegistry {
    fn default() -> Self {
        Self::new()
    }
}

/// Tool that waits for multiple background subagent tasks to complete.
pub struct JoinTool {
    registry: Arc<TaskRegistry>,
}

impl JoinTool {
    pub fn new(registry: Arc<TaskRegistry>) -> Self {
        Self { registry }
    }
}

#[async_trait]
impl ToolExecutor for JoinTool {
    fn name(&self) -> &str {
        "join"
    }

    fn description(&self) -> &str {
        include_str!("instructions/join.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(JoinArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: JoinArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.task_ids.is_empty() {
            return Err("No task_ids provided".to_string());
        }

        let timeout = Duration::from_secs(args.timeout_secs);
        let mut receivers = Vec::new();
        let mut already_completed = Vec::new();

        // Register for all tasks
        for task_id in &args.task_ids {
            match self.registry.register(task_id.clone()) {
                Some(mut rx) => {
                    // Check if already completed (receiver returns immediately)
                    match rx.try_recv() {
                        Ok(result) => already_completed.push(result),
                        Err(oneshot::error::TryRecvError::Empty) => {
                            // Still pending, need to wait
                            receivers.push((task_id.clone(), rx));
                        }
                        Err(oneshot::error::TryRecvError::Closed) => {
                            // Task not found or already consumed
                            already_completed.push(TaskResult {
                                task_id: task_id.clone(),
                                status: "not_found".to_string(),
                                output: None,
                                error: Some("Task not found".to_string()),
                            });
                        }
                    }
                }
                None => {
                    already_completed.push(TaskResult {
                        task_id: task_id.clone(),
                        status: "not_found".to_string(),
                        output: None,
                        error: Some("Task not found".to_string()),
                    });
                }
            }
        }

        // Wait for pending tasks with timeout
        let mut results = already_completed;
        let deadline = tokio::time::Instant::now() + timeout;

        for (task_id, rx) in receivers {
            let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
            
            match tokio::time::timeout(remaining, rx).await {
                Ok(Ok(result)) => results.push(result),
                Ok(Err(_)) => results.push(TaskResult {
                    task_id,
                    status: "failed".to_string(),
                    output: None,
                    error: Some("Receiver dropped".to_string()),
                }),
                Err(_) => results.push(TaskResult {
                    task_id,
                    status: "timeout".to_string(),
                    output: None,
                    error: Some(format!("Timeout after {}s", args.timeout_secs)),
                }),
            }
        }

        // Build final result
        let total = results.len();
        let succeeded = results.iter().filter(|r| r.status == "completed").count();
        let failed = results.iter().filter(|r| r.status == "failed").count();
        let not_found = results.iter().filter(|r| r.status == "not_found").count();

        let join_result = JoinResult {
            all_succeeded: failed == 0 && not_found == 0,
            total,
            succeeded,
            failed,
            not_found,
            completed: results,
        };

        serde_json::to_string_pretty(&join_result)
            .map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_registry_complete_before_register() {
        let registry = TaskRegistry::new();
        
        // Complete a task
        registry.complete("task-1".to_string(), Ok("output".to_string()));
        
        // Register for it - should get immediate result
        let mut rx = registry.register("task-1".to_string()).unwrap();
        let result = rx.try_recv().unwrap();
        
        assert_eq!(result.task_id, "task-1");
        assert_eq!(result.status, "completed");
        assert_eq!(result.output, Some("output".to_string()));
    }

    #[test]
    fn test_registry_multiple_waiters() {
        let registry = Arc::new(TaskRegistry::new());
        
        // Multiple registrations for same task
        let mut rx1 = registry.register("task-1".to_string()).unwrap();
        let mut rx2 = registry.register("task-1".to_string()).unwrap();
        
        // Complete once
        registry.complete("task-1".to_string(), Ok("shared output".to_string()));
        
        // Both should get the result
        let result1 = rx1.try_recv().unwrap();
        let result2 = rx2.try_recv().unwrap();
        
        assert_eq!(result1.output, Some("shared output".to_string()));
        assert_eq!(result2.output, Some("shared output".to_string()));
    }

    #[tokio::test]
    async fn test_join_tool_empty_task_ids() {
        let registry = Arc::new(TaskRegistry::new());
        let tool = JoinTool::new(registry);
        
        let args = serde_json::json!({
            "task_ids": [],
            "timeout_secs": 10
        });
        
        let result = tool.execute(args).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("No task_ids"));
    }

    #[tokio::test]
    async fn test_join_tool_already_completed() {
        let registry = Arc::new(TaskRegistry::new());
        let tool = JoinTool::new(registry.clone());
        
        // Pre-complete a task
        registry.complete("task-1".to_string(), Ok("done".to_string()));
        
        let args = serde_json::json!({
            "task_ids": ["task-1"],
            "timeout_secs": 10
        });
        
        let result = tool.execute(args).await.unwrap();
        let parsed: JoinResult = serde_json::from_str(&result).unwrap();
        
        assert_eq!(parsed.total, 1);
        assert_eq!(parsed.succeeded, 1);
        assert!(parsed.all_succeeded);
        assert_eq!(parsed.completed[0].output, Some("done".to_string()));
    }
}
