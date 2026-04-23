//! Self-delegation `agent` tool interception (targets `__self__`).

use std::time::Instant;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use rig::agent::ToolCallHookAction;
use serde_json::json;
use tools::agent::AGENT_CONTEXT;
use tools::self_agent::AgentToolParams;
use tracing::{info, warn};

use super::truncate::{truncate_if_needed, Truncated};
use super::ToolCallEmitter;

impl ToolCallEmitter {
    /// Handle a self-delegation agent tool call by executing it here where
    /// AGENT_CONTEXT is available, then returning `Skip` so rig-core does not
    /// dispatch to a spawned task (where the task_local would be lost).
    pub(super) async fn handle_self_agent_tool(
        &self,
        params: AgentToolParams,
        sse_id: String,
        call_start: Instant,
    ) -> ToolCallHookAction {
        let ctx = match AGENT_CONTEXT.try_with(|c| c.clone()) {
            Ok(ctx) => ctx,
            Err(_) => {
                let error = "Agent tool requires a conversation context".to_string();
                let duration_ms = call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed("agent", true, duration_ms);
                warn!(
                    agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                    tool_name = "agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "agent"}),
                ));
                self.persist_tool_interaction(
                    "agent",
                    &sse_id,
                    &serde_json::Value::Null,
                    &error,
                    true,
                );
                return ToolCallHookAction::Skip { reason: error };
            }
        };

        // Check depth limit
        if ctx.depth >= ctx.max_depth {
            let error = format!("Maximum agent depth ({}) reached", ctx.max_depth);
            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", true, duration_ms);
            warn!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, error = %error, "tool_call_failed"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction("agent", &sse_id, &serde_json::Value::Null, &error, true);
            return ToolCallHookAction::Skip { reason: error };
        }

        // Check task budget
        if let Err(e) = ctx.task_budget.try_acquire() {
            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", true, duration_ms);
            warn!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, error = %e, "tool_call_failed"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &e, "is_error": true, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction("agent", &sse_id, &serde_json::Value::Null, &e, true);
            return ToolCallHookAction::Skip { reason: e };
        }

        // Self-delegation always targets "__self__"
        let subagent_name = "__self__";

        if params.run_in_background {
            let result = ctx
                .runner
                .run_background(subagent_name, &params.prompt, &params.description)
                .await;

            let (result_str, is_error) = match result {
                Ok(handle) => {
                    let json = serde_json::json!({
                        "task_id": handle.task_id,
                        "status": "running",
                        "message": "Background agent started. Its final output will appear in your next user turn — do not poll or wait."
                    })
                    .to_string();
                    (json, false)
                }
                Err(e) => (e, true),
            };

            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", is_error, duration_ms);
            info!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, is_error = is_error,
                result = %Truncated::new(&result_str, 80), "tool_call_complete"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction(
                "agent",
                &sse_id,
                &serde_json::Value::Null,
                &result_str,
                is_error,
            );
            ToolCallHookAction::Skip {
                reason: truncate_if_needed(result_str),
            }
        } else {
            let result = ctx
                .runner
                .run_foreground(subagent_name, &params.prompt, params.task_id.as_deref())
                .await;

            let (result_str, is_error) = match result {
                Ok(task_result) => {
                    let output = format!(
                        "task_id: {} (for resuming)\n\n<task_result>\n{}\n</task_result>",
                        task_result.task_id, task_result.output
                    );
                    (output, false)
                }
                Err(e) => (e, true),
            };

            let duration_ms = call_start.elapsed().as_millis() as u64;
            self.metrics
                .record_tool_call_detailed("agent", is_error, duration_ms);
            info!(
                agent_id = %self.agent_id, conversation_id = %self.conversation_id,
                tool_name = "agent", duration_ms = duration_ms, is_error = is_error,
                result = %Truncated::new(&result_str, 80), "tool_call_complete"
            );
            self.event_bus.emit(BridgeEvent::new(
                BridgeEventType::ToolCallCompleted,
                &self.agent_id,
                &self.conversation_id,
                json!({"id": &sse_id, "result": &result_str, "is_error": is_error, "duration_ms": duration_ms, "tool_name": "agent"}),
            ));
            self.persist_tool_interaction(
                "agent",
                &sse_id,
                &serde_json::Value::Null,
                &result_str,
                is_error,
            );
            ToolCallHookAction::Skip {
                reason: truncate_if_needed(result_str),
            }
        }
    }
}
