use async_trait::async_trait;
use tools::agent::{AgentTaskHandle, AgentTaskResult, SubAgentRunner};

use super::{background, foreground, ConversationSubAgentRunner};

#[async_trait]
impl SubAgentRunner for ConversationSubAgentRunner {
    fn available_subagents(&self) -> Vec<(String, String)> {
        self.subagents
            .iter()
            .filter(|entry| entry.key() != tools::self_agent::SELF_AGENT_NAME)
            .map(|entry| {
                let e = entry.value();
                (e.name.clone(), e.description.clone())
            })
            .collect()
    }

    async fn run_foreground(
        &self,
        subagent: &str,
        prompt: &str,
        task_id: Option<&str>,
    ) -> Result<AgentTaskResult, String> {
        foreground::run_foreground(self, subagent, prompt, task_id).await
    }

    async fn run_background(
        &self,
        subagent: &str,
        prompt: &str,
        description: &str,
    ) -> Result<AgentTaskHandle, String> {
        background::run_background(self, subagent, prompt, description).await
    }
}
