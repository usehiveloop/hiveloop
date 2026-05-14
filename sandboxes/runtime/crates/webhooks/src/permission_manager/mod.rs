use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::permission::{ApprovalDecision, ApprovalRequest, ApprovalStatus};
use dashmap::DashMap;
use serde_json::json;
use std::sync::Arc;
use tokio::sync::oneshot;
use tracing::{debug, warn};

use crate::EventBus;

/// An approval decision paired with an optional user-provided reason.
pub type ApprovalResult = (ApprovalDecision, Option<String>);

/// A pending approval that holds the request metadata and the channel sender
/// to unblock the waiting tool call.
pub struct PendingApproval {
    pub request: ApprovalRequest,
    pub sender: oneshot::Sender<ApprovalResult>,
}

/// Manages pending tool call approval requests across all conversations.
///
/// Stored in `AppState` and shared via `Arc` with all `ToolCallEmitter` instances.
#[derive(Default)]
pub struct PermissionManager {
    pending: DashMap<String, PendingApproval>,
}

impl PermissionManager {
    pub fn new() -> Self {
        Self {
            pending: DashMap::new(),
        }
    }

    /// Create a new approval request, emit an event via the EventBus, and block
    /// until the user resolves it (or the channel is dropped).
    ///
    /// Returns `Ok(decision)` when the user approves/denies, or `Err(())` if
    /// the conversation ended (sender dropped).
    #[allow(clippy::too_many_arguments)]
    pub async fn request_approval(
        &self,
        agent_id: &str,
        conversation_id: &str,
        tool_name: &str,
        tool_call_id: &str,
        arguments: &serde_json::Value,
        event_bus: &Arc<EventBus>,
        integration_name: Option<String>,
        integration_action: Option<String>,
    ) -> Result<ApprovalResult, ()> {
        let request_id = uuid::Uuid::new_v4().to_string();
        let (tx, rx) = oneshot::channel();

        let request = ApprovalRequest {
            id: request_id.clone(),
            agent_id: agent_id.to_string(),
            conversation_id: conversation_id.to_string(),
            tool_name: tool_name.to_string(),
            tool_call_id: tool_call_id.to_string(),
            arguments: arguments.clone(),
            status: ApprovalStatus::Pending,
            created_at: chrono::Utc::now(),
        };

        // Emit approval required event
        event_bus.emit(BridgeEvent::new(
            BridgeEventType::ToolApprovalRequired,
            agent_id,
            conversation_id,
            json!({
                "request_id": &request_id,
                "tool_name": tool_name,
                "tool_call_id": tool_call_id,
                "arguments": arguments,
                "integration_name": integration_name,
                "integration_action": integration_action,
            }),
        ));

        debug!(
            request_id = %request_id,
            tool_name = tool_name,
            agent_id = agent_id,
            conversation_id = conversation_id,
            "tool approval requested, awaiting decision"
        );

        // Store the pending approval
        self.pending.insert(
            request_id,
            PendingApproval {
                request,
                sender: tx,
            },
        );

        // Block until a decision arrives or the channel is dropped
        rx.await.map_err(|_| ())
    }

    /// Resolve a single pending approval request.
    ///
    /// Returns `true` if the request was found and resolved, `false` otherwise.
    pub fn resolve(
        &self,
        request_id: &str,
        decision: ApprovalDecision,
        reason: Option<String>,
        event_bus: Option<&Arc<EventBus>>,
    ) -> bool {
        if let Some((_, pending)) = self.pending.remove(request_id) {
            // Emit approval resolved event
            if let Some(bus) = event_bus {
                let mut event_data = json!({
                    "request_id": request_id,
                    "decision": match &decision {
                        ApprovalDecision::Approve => "approve",
                        ApprovalDecision::Deny => "deny",
                    },
                });
                if let Some(ref r) = reason {
                    event_data["reason"] = json!(r);
                }
                bus.emit(BridgeEvent::new(
                    BridgeEventType::ToolApprovalResolved,
                    &pending.request.agent_id,
                    &pending.request.conversation_id,
                    event_data,
                ));
            }

            debug!(request_id = request_id, "approval resolved");

            let _ = pending.sender.send((decision, reason));
            true
        } else {
            warn!(request_id = request_id, "approval request not found");
            false
        }
    }

    /// List all pending approval requests for a given conversation.
    pub fn list_pending(&self, conversation_id: &str) -> Vec<ApprovalRequest> {
        self.pending
            .iter()
            .filter(|entry| entry.value().request.conversation_id == conversation_id)
            .map(|entry| entry.value().request.clone())
            .collect()
    }

    /// Clean up all pending approvals for a conversation (e.g., when it ends).
    ///
    /// Drops all oneshot senders, causing the waiting tasks to receive `RecvError`.
    pub fn cleanup_conversation(&self, conversation_id: &str) {
        let keys: Vec<String> = self
            .pending
            .iter()
            .filter(|entry| entry.value().request.conversation_id == conversation_id)
            .map(|entry| entry.key().clone())
            .collect();

        for key in keys {
            self.pending.remove(&key);
        }
    }
}

#[cfg(test)]
mod tests;
