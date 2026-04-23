use async_trait::async_trait;
use std::sync::Arc;
use tokio::sync::mpsc;

use crate::agent::{AgentContext, AgentTaskHandle, AgentTaskResult, SubAgentRunner, TaskBudget};

pub(super) struct MockRunner {
    pub subagents: Vec<(String, String)>,
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

pub(super) fn make_context(subagents: Vec<(String, String)>) -> AgentContext {
    let (tx, _rx) = mpsc::channel(16);
    AgentContext {
        runner: Arc::new(MockRunner { subagents }),
        notification_tx: tx,
        depth: 0,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    }
}
