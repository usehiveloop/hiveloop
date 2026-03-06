use bridge_core::permission::{ApprovalDecision, ApprovalRequest, ApprovalStatus};
use dashmap::DashMap;
use serde_json::json;
use tokio::sync::{mpsc, oneshot};
use tracing::{debug, warn};
use webhooks::WebhookContext;

use crate::SseEvent;

/// A pending approval that holds the request metadata and the channel sender
/// to unblock the waiting tool call.
pub struct PendingApproval {
    pub request: ApprovalRequest,
    pub sender: oneshot::Sender<ApprovalDecision>,
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

    /// Create a new approval request, emit SSE + webhook events, and block
    /// until the user resolves it (or the channel is dropped).
    ///
    /// Returns `Ok(decision)` when the user approves/denies, or `Err(())` if
    /// the conversation ended (sender dropped).
    pub async fn request_approval(
        &self,
        agent_id: &str,
        conversation_id: &str,
        tool_name: &str,
        tool_call_id: &str,
        arguments: &serde_json::Value,
        sse_tx: &mpsc::Sender<SseEvent>,
        webhook_ctx: &Option<WebhookContext>,
    ) -> Result<ApprovalDecision, ()> {
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

        // Emit SSE event
        let _ = sse_tx
            .send(SseEvent::ToolApprovalRequired {
                request_id: request_id.clone(),
                tool_name: tool_name.to_string(),
                tool_call_id: tool_call_id.to_string(),
                arguments: arguments.clone(),
            })
            .await;

        // Emit webhook
        if let Some(ref wh) = webhook_ctx {
            wh.dispatcher
                .dispatch(webhooks::events::tool_approval_required(
                    agent_id,
                    conversation_id,
                    json!({
                        "request_id": &request_id,
                        "tool_name": tool_name,
                        "tool_call_id": tool_call_id,
                        "arguments": arguments,
                    }),
                    &wh.url,
                    &wh.secret,
                ));
        }

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
        sse_tx: Option<&mpsc::Sender<SseEvent>>,
        webhook_ctx: &Option<WebhookContext>,
    ) -> bool {
        if let Some((_, pending)) = self.pending.remove(request_id) {
            // Emit SSE event for resolution
            if let Some(tx) = sse_tx {
                let event = SseEvent::ToolApprovalResolved {
                    request_id: request_id.to_string(),
                    decision: match &decision {
                        ApprovalDecision::Approve => "approve".to_string(),
                        ApprovalDecision::Deny => "deny".to_string(),
                    },
                };
                let _ = tx.try_send(event);
            }

            // Emit webhook for resolution
            if let Some(ref wh) = webhook_ctx {
                wh.dispatcher
                    .dispatch(webhooks::events::tool_approval_resolved(
                        &pending.request.agent_id,
                        &pending.request.conversation_id,
                        json!({
                            "request_id": request_id,
                            "decision": match &decision {
                                ApprovalDecision::Approve => "approve",
                                ApprovalDecision::Deny => "deny",
                            },
                        }),
                        &wh.url,
                        &wh.secret,
                    ));
            }

            debug!(request_id = request_id, "approval resolved");

            let _ = pending.sender.send(decision);
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
mod tests {
    use super::*;
    use std::sync::Arc;

    #[tokio::test]
    async fn test_request_and_approve() {
        let manager = Arc::new(PermissionManager::new());
        let (sse_tx, mut sse_rx) = mpsc::channel::<SseEvent>(16);

        let manager_clone = manager.clone();
        let handle = tokio::spawn(async move {
            manager_clone
                .request_approval(
                    "agent1",
                    "conv1",
                    "bash",
                    "call_1",
                    &json!({"command": "ls"}),
                    &sse_tx,
                    &None,
                )
                .await
        });

        let event = sse_rx.recv().await.unwrap();
        match event {
            SseEvent::ToolApprovalRequired { request_id, .. } => {
                assert!(manager.resolve(&request_id, ApprovalDecision::Approve, None, &None));
            }
            _ => panic!("expected ToolApprovalRequired"),
        }

        let result = handle.await.unwrap();
        assert_eq!(result, Ok(ApprovalDecision::Approve));
    }

    #[tokio::test]
    async fn test_request_and_deny() {
        let manager = Arc::new(PermissionManager::new());
        let (sse_tx, mut sse_rx) = mpsc::channel::<SseEvent>(16);

        let manager_clone = manager.clone();
        let handle = tokio::spawn(async move {
            manager_clone
                .request_approval(
                    "agent1",
                    "conv1",
                    "bash",
                    "call_1",
                    &json!({"command": "rm -rf /"}),
                    &sse_tx,
                    &None,
                )
                .await
        });

        let event = sse_rx.recv().await.unwrap();
        match event {
            SseEvent::ToolApprovalRequired { request_id, .. } => {
                assert!(manager.resolve(&request_id, ApprovalDecision::Deny, None, &None));
            }
            _ => panic!("expected ToolApprovalRequired"),
        }

        let result = handle.await.unwrap();
        assert_eq!(result, Ok(ApprovalDecision::Deny));
    }

    #[tokio::test]
    async fn test_cleanup_conversation() {
        let manager = Arc::new(PermissionManager::new());
        let (sse_tx, _sse_rx) = mpsc::channel::<SseEvent>(16);

        let m2 = manager.clone();
        let _handle = tokio::spawn(async move {
            let _ = m2
                .request_approval(
                    "agent1",
                    "conv1",
                    "bash",
                    "call_1",
                    &json!({}),
                    &sse_tx,
                    &None,
                )
                .await;
        });

        // Give it a moment to register
        tokio::task::yield_now().await;

        assert_eq!(manager.list_pending("conv1").len(), 1);
        manager.cleanup_conversation("conv1");
        assert_eq!(manager.list_pending("conv1").len(), 0);
    }

    #[tokio::test]
    async fn test_list_pending_filters_by_conversation() {
        let manager = Arc::new(PermissionManager::new());
        let (sse_tx, _sse_rx) = mpsc::channel::<SseEvent>(16);

        let m2 = manager.clone();
        let sse_tx2 = sse_tx.clone();
        let _h1 = tokio::spawn(async move {
            let _ = m2
                .request_approval("agent1", "conv1", "bash", "call_1", &json!({}), &sse_tx, &None)
                .await;
        });

        let m3 = manager.clone();
        let _h2 = tokio::spawn(async move {
            let _ = m3
                .request_approval(
                    "agent1",
                    "conv2",
                    "bash",
                    "call_2",
                    &json!({}),
                    &sse_tx2,
                    &None,
                )
                .await;
        });

        tokio::task::yield_now().await;

        assert_eq!(manager.list_pending("conv1").len(), 1);
        assert_eq!(manager.list_pending("conv2").len(), 1);
    }

    #[test]
    fn test_resolve_nonexistent() {
        let manager = PermissionManager::new();
        assert!(!manager.resolve("nonexistent", ApprovalDecision::Approve, None, &None));
    }
}
