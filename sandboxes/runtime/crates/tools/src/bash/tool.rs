use async_trait::async_trait;

use super::args::BashArgs;
use super::runner::run_command;
use crate::agent::{AgentTaskNotification, AGENT_CONTEXT};
use crate::ToolExecutor;

pub struct BashTool;

impl BashTool {
    pub fn new() -> Self {
        Self
    }
}

impl Default for BashTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolExecutor for BashTool {
    fn name(&self) -> &str {
        "bash"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/bash.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(BashArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: BashArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let timeout_ms = args.timeout.unwrap_or(120_000);
        let workdir = args.workdir.as_deref().unwrap_or(".").to_string();
        let command = args.command.clone();
        let description = args.description.clone().unwrap_or_else(|| {
            // Take first line of command, truncated to 80 chars
            let first_line = command.lines().next().unwrap_or(&command);
            if first_line.len() > 80 {
                format!("{}...", &first_line[..77])
            } else {
                first_line.to_string()
            }
        });

        if args.background {
            // Background execution: return immediately, notify on completion.
            // The result is injected into the next user turn by the
            // conversation loop when the notification is received.
            let ctx = AGENT_CONTEXT
                .try_with(|c| c.clone())
                .map_err(|_| "Background bash requires a conversation context".to_string())?;

            let task_id = uuid::Uuid::new_v4().to_string();
            let task_id_clone = task_id.clone();
            let notification_tx = ctx.notification_tx.clone();

            tokio::spawn(async move {
                let result = run_command(&command, &workdir, timeout_ms).await;

                let output = match result {
                    Ok(bash_result) => match serde_json::to_string(&bash_result) {
                        Ok(json) => Ok(json),
                        Err(e) => Err(format!("Failed to serialize result: {e}")),
                    },
                    Err(e) => Err(e),
                };

                let notification = AgentTaskNotification {
                    task_id: task_id_clone,
                    description,
                    output,
                };

                // If the receiver is dropped (conversation ended), silently discard
                let _ = notification_tx.send(notification).await;
            });

            serde_json::to_string(&serde_json::json!({
                "task_id": task_id,
                "status": "running",
                "message": "Background command started. You will be notified when it completes."
            }))
            .map_err(|e| format!("Failed to serialize result: {e}"))
        } else {
            // Foreground execution: block until complete
            let result = run_command(&command, &workdir, timeout_ms).await?;

            serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
        }
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
