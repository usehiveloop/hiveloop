//! Background-bash interception — spawns the command asynchronously and
//! sends a notification via the conversation's `notification_tx` when
//! complete.

use std::time::Instant;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use rig::agent::ToolCallHookAction;
use serde_json::json;
use tools::agent::{AgentTaskNotification, AGENT_CONTEXT};
use tools::bash::{run_command, BashArgs};
use tracing::{info, warn};

use super::truncate::truncate_if_needed;
use super::ToolCallEmitter;

impl ToolCallEmitter {
    /// Handle a bash tool call with `background: true`.
    ///
    /// Spawns the command asynchronously and sends a notification via the
    /// conversation's `notification_tx` when complete. Returns `Skip` with
    /// a JSON result containing the task_id so the tool server does not
    /// execute the bash tool itself.
    pub(super) async fn handle_background_bash(
        &self,
        args: BashArgs,
        sse_id: String,
        call_start: Instant,
    ) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                let error = "Background bash requires a conversation context".to_string();
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed("bash", true, duration_ms);
                warn!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = "bash",
                    duration_ms = duration_ms,
                    error = %error,
                    "tool_call_failed"
                );
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        let task_id = uuid::Uuid::new_v4().to_string();
        let task_id_clone = task_id.clone();
        let notification_tx = ctx.notification_tx.clone();

        let command = args.command.clone();
        let timeout_ms = args.timeout.unwrap_or(120_000);
        let workdir = args.workdir.unwrap_or_else(|| ".".to_string());
        let description = args
            .description
            .unwrap_or_else(|| command.chars().take(80).collect());
        let description_clone = description.clone();

        let result_json = serde_json::json!({
            "task_id": task_id,
            "status": "running",
            "message": "Background command started. You will be notified when it completes."
        })
        .to_string();

        // Record metrics for the background bash spawn (not the actual execution)
        let duration_ms = call_start.elapsed().as_millis() as u64;
        self.metrics
            .record_tool_call_detailed("bash", false, duration_ms);
        info!(
            agent_id = %self.agent_id,
            conversation_id = %self.conversation_id,
            tool_name = "bash",
            duration_ms = duration_ms,
            is_error = false,
            task_id = %task_id_clone,
            "tool_call_complete"
        );

        // Emit the tool result SSE event for the immediate response
        let result_json_clone = result_json.clone();
        let cancel = self.cancel.clone();
        tokio::spawn(async move {
            let result = tokio::select! {
                _ = cancel.cancelled() => {
                    Err("Background command cancelled".to_string())
                }
                result = run_command(&command, &workdir, timeout_ms) => result,
            };

            let output = match result {
                Ok(bash_result) => match serde_json::to_string(&bash_result) {
                    Ok(json) => Ok(json),
                    Err(e) => Err(format!("Failed to serialize result: {e}")),
                },
                Err(e) => Err(e),
            };

            let notification = AgentTaskNotification {
                task_id: task_id_clone,
                description: description_clone,
                output,
            };

            // If the receiver is dropped (conversation ended), silently discard
            let _ = notification_tx.send(notification).await;
        });

        // Emit tool_call_result so the client sees the immediate response
        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallCompleted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &sse_id, "result": &result_json_clone, "is_error": false, "duration_ms": duration_ms, "tool_name": "bash"}),
        ));
        self.persist_tool_interaction(
            "bash",
            &sse_id,
            &serde_json::Value::Null,
            &result_json_clone,
            false,
        );

        ToolCallHookAction::Skip {
            reason: truncate_if_needed(result_json),
        }
    }
}
