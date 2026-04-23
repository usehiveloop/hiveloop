use async_trait::async_trait;

use super::context::AGENT_CONTEXT;
use super::params::SubAgentToolParams;
use crate::ToolExecutor;

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
    pub(super) fn build_description(agents: &[(String, String)]) -> String {
        let template = include_str!("../instructions/sub_agent.txt");
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
            include_str!("../instructions/sub_agent.txt")
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
