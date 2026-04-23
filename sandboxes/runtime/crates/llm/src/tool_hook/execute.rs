//! Direct execution path for auto-repaired tool names.

use std::time::Instant;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use rig::agent::ToolCallHookAction;
use serde_json::json;
use tracing::{info, warn};

use super::truncate::{truncate_if_needed, Truncated};
use super::ToolCallEmitter;

impl ToolCallEmitter {
    /// Execute a tool directly after its name was auto-repaired.
    ///
    /// Because rig-core dispatches tools by exact name match, a repaired name
    /// (e.g. `" bash"` → `"bash"`) would not be found by rig-core. We execute
    /// the tool ourselves and return `Skip` with the result.
    pub(super) async fn execute_repaired_tool(
        &self,
        tool_name: &str,
        args: &str,
        sse_id: String,
        call_start: Instant,
    ) -> ToolCallHookAction {
        let executor = match self.tool_executors.get(tool_name) {
            Some(executor) => executor.clone(),
            None => {
                let error = format!(
                    "Tool '{}' resolved but executor not found (internal error)",
                    tool_name
                );
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed(tool_name, true, duration_ms);
                warn!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = tool_name,
                    duration_ms = duration_ms,
                    error = %error,
                    "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": tool_name}),
                ));
                self.persist_tool_interaction(
                    tool_name,
                    &sse_id,
                    &serde_json::Value::Null,
                    &error,
                    true,
                );
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        let args_value: serde_json::Value =
            serde_json::from_str(args).unwrap_or(serde_json::Value::Object(serde_json::Map::new()));

        let (result_str, is_error) = match executor.execute(args_value.clone()).await {
            Ok(output) => (output, false),
            Err(e) => (format!("Toolset error: {}", e), true),
        };

        let duration_ms = call_start.elapsed().as_millis() as u64;
        self.metrics
            .record_tool_call_detailed(tool_name, is_error, duration_ms);
        if is_error {
            warn!(
                agent_id = %self.agent_id,
                conversation_id = %self.conversation_id,
                tool_name = tool_name,
                duration_ms = duration_ms,
                error = %Truncated::new(&result_str, 80),
                "tool_call_failed"
            );
        } else {
            info!(
                agent_id = %self.agent_id,
                conversation_id = %self.conversation_id,
                tool_name = tool_name,
                duration_ms = duration_ms,
                is_error = false,
                result = %Truncated::new(&result_str, 80),
                "tool_call_complete"
            );
        }

        self.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallCompleted,
            &self.agent_id,
            &self.conversation_id,
            json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": tool_name}),
        ));
        self.persist_tool_interaction(tool_name, &sse_id, &args_value, &result_str, is_error);

        ToolCallHookAction::Skip {
            reason: truncate_if_needed(result_str),
        }
    }
}
