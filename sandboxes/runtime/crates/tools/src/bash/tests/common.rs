use async_trait::async_trait;
use std::sync::Arc;
use tokio::sync::mpsc;

use crate::agent::{
    AgentContext, AgentTaskHandle, AgentTaskNotification, AgentTaskResult, SubAgentRunner,
    TaskBudget,
};

/// Mock SubAgentRunner needed to construct an AgentContext for background tests.
pub(super) struct MockRunner;

#[async_trait]
impl SubAgentRunner for MockRunner {
    fn available_subagents(&self) -> Vec<(String, String)> {
        vec![]
    }

    async fn run_foreground(
        &self,
        _subagent: &str,
        _prompt: &str,
        _task_id: Option<&str>,
    ) -> Result<AgentTaskResult, String> {
        Err("not implemented".to_string())
    }

    async fn run_background(
        &self,
        _subagent: &str,
        _prompt: &str,
        _description: &str,
    ) -> Result<AgentTaskHandle, String> {
        Err("not implemented".to_string())
    }
}

pub(super) fn make_context() -> (AgentContext, mpsc::Receiver<AgentTaskNotification>) {
    let (tx, rx) = mpsc::channel(16);
    let ctx = AgentContext {
        runner: Arc::new(MockRunner),
        notification_tx: tx,
        depth: 0,
        max_depth: 3,
        task_budget: Arc::new(TaskBudget::new(50)),
    };
    (ctx, rx)
}
