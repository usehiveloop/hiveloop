use async_trait::async_trait;
use std::sync::Arc;
use tokio::sync::mpsc;

use super::budget::TaskBudget;

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
