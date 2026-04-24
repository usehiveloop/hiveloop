//! Per-agent permission enforcement helper used by `on_tool_call`.

use std::time::Instant;

use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::permission::{ApprovalDecision, ToolPermission};
use rig::agent::ToolCallHookAction;
use serde_json::json;
use tracing::{info, warn};

use super::ToolCallEmitter;

/// Minimal state threaded from `on_tool_call` so we can log + emit errors
/// with the same fields the original inline implementation used.
pub(super) struct PermissionCtx<'a> {
    pub(super) call_start: Instant,
    pub(super) id: &'a str,
    pub(super) arguments: &'a serde_json::Value,
}

impl ToolCallEmitter {
    /// Enforce the per-agent permission policy for `effective_name`.
    /// Returns `Ok(())` to allow the call, or `Skip` with the appropriate
    /// error. Matches the original inline match arms exactly.
    pub(super) async fn enforce_permissions(
        &self,
        effective_name: &str,
        ctx: &PermissionCtx<'_>,
    ) -> Result<(), ToolCallHookAction> {
        match self.agent_permissions.get(effective_name) {
            Some(ToolPermission::Deny) => {
                let error = json!({
                    "error": format!("Tool '{}' is denied by agent permissions", effective_name)
                })
                .to_string();
                let duration_ms = ctx.call_start.elapsed().as_millis() as u64;
                self.metrics
                    .record_tool_call_detailed(effective_name, true, false, duration_ms);
                warn!(
                    agent_id = %self.agent_id,
                    conversation_id = %self.conversation_id,
                    tool_name = %effective_name,
                    duration_ms = duration_ms,
                    error = %error,
                    "tool_call_failed"
                );
                self.event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolCallCompleted,
                    &self.agent_id,
                    &self.conversation_id,
                    json!({"id": ctx.id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": effective_name}),
                ));
                self.persist_tool_interaction(effective_name, ctx.id, ctx.arguments, &error, true);
                Err(ToolCallHookAction::Skip { reason: error })
            }
            Some(ToolPermission::RequireApproval) => {
                // Extract integration metadata from tool name (format: "integration__action")
                let (int_name, int_action) =
                    tools::integration::parse_integration_tool_name(effective_name)
                        .map(|(n, a)| (Some(n.to_string()), Some(a.to_string())))
                        .unwrap_or((None, None));

                let decision = self
                    .permission_manager
                    .request_approval(
                        &self.agent_id,
                        &self.conversation_id,
                        effective_name,
                        ctx.id,
                        ctx.arguments,
                        &self.event_bus,
                        int_name,
                        int_action,
                    )
                    .await;
                match decision {
                    Ok((ApprovalDecision::Deny, reason)) => {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            tool_name = %effective_name,
                            decision = "denied",
                            "permission_decision"
                        );
                        let error_msg = match reason {
                            Some(r) => {
                                format!("Tool '{}' denied by user: {}", effective_name, r)
                            }
                            None => format!("Tool '{}' denied by user", effective_name),
                        };
                        let error = json!({"error": error_msg}).to_string();
                        let duration_ms = ctx.call_start.elapsed().as_millis() as u64;
                        self.metrics.record_tool_call_detailed(
                            effective_name,
                            true,
                            false,
                            duration_ms,
                        );
                        self.event_bus.emit(BridgeEvent::new(
                            BridgeEventType::ToolCallCompleted,
                            &self.agent_id,
                            &self.conversation_id,
                            json!({"id": ctx.id, "result": &error, "is_error": true, "duration_ms": duration_ms, "tool_name": effective_name}),
                        ));
                        self.persist_tool_interaction(
                            effective_name,
                            ctx.id,
                            ctx.arguments,
                            &error,
                            true,
                        );
                        Err(ToolCallHookAction::Skip { reason: error })
                    }
                    Ok((ApprovalDecision::Approve, _)) => {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            tool_name = %effective_name,
                            decision = "approved",
                            "permission_decision"
                        );
                        Ok(())
                    }
                    Err(()) => {
                        info!(
                            agent_id = %self.agent_id,
                            conversation_id = %self.conversation_id,
                            tool_name = %effective_name,
                            decision = "cancelled",
                            "permission_decision"
                        );
                        let error = json!({
                            "error": "Tool approval cancelled — conversation ended"
                        })
                        .to_string();
                        let duration_ms = ctx.call_start.elapsed().as_millis() as u64;
                        self.metrics.record_tool_call_detailed(
                            effective_name,
                            true,
                            false,
                            duration_ms,
                        );
                        self.persist_tool_interaction(
                            effective_name,
                            ctx.id,
                            ctx.arguments,
                            &error,
                            true,
                        );
                        Err(ToolCallHookAction::Skip { reason: error })
                    }
                }
            }
            Some(ToolPermission::Allow) | None => Ok(()),
        }
    }
}
