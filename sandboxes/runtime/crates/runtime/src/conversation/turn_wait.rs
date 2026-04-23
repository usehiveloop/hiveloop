use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::permission::ToolPermission;
use serde_json::json;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::oneshot;
use tokio_util::sync::CancellationToken;
use tracing::{debug, info};
use webhooks::EventBus;

use super::params::AGENT_CHAT_TIMEOUT;

/// Outcome of waiting for the spawned streaming task.
#[allow(clippy::type_complexity)]
pub(super) enum ChatWaitOutcome {
    /// Spawned task returned a result (may be Err/Ok internally).
    Got(
        Result<
            Result<
                (
                    Result<llm::PromptResponse, rig::completion::PromptError>,
                    Vec<rig::message::Message>,
                ),
                oneshot::error::RecvError,
            >,
            tokio::time::error::Elapsed,
        >,
    ),
    /// Agent-level shutdown fired — caller should break the loop.
    Shutdown,
    /// Turn-level abort fired — caller should cleanup & continue the loop.
    Abort,
}

#[allow(clippy::too_many_arguments)]
pub(super) async fn wait_for_chat_result(
    cancel: &CancellationToken,
    turn_cancel: &CancellationToken,
    conversation_id: &str,
    result_rx: oneshot::Receiver<(
        Result<llm::PromptResponse, rig::completion::PromptError>,
        Vec<rig::message::Message>,
    )>,
    agent_permissions: &HashMap<String, ToolPermission>,
) -> ChatWaitOutcome {
    tokio::select! {
        _ = cancel.cancelled() => {
            debug!(conversation_id = conversation_id, "conversation cancelled by agent shutdown");
            ChatWaitOutcome::Shutdown
        }
        _ = turn_cancel.cancelled() => {
            info!(conversation_id = conversation_id, "turn aborted by user");
            ChatWaitOutcome::Abort
        }
        result = async {
            let has_approval_tools = agent_permissions.values().any(|p| *p == ToolPermission::RequireApproval);
            if has_approval_tools {
                Ok(result_rx.await)
            } else {
                tokio::time::timeout(AGENT_CHAT_TIMEOUT, result_rx).await
            }
        } => {
            ChatWaitOutcome::Got(result)
        }
    }
}

/// Execute the per-turn "abort" cleanup — restore history, truncate persisted
/// messages, drop any staged journal entries, and emit the AgentError/Done/
/// TurnCompleted event triple.
#[allow(clippy::too_many_arguments)]
pub(super) async fn handle_abort_cleanup(
    history: &mut Vec<rig::message::Message>,
    history_backup: Vec<rig::message::Message>,
    persisted_messages: &Arc<std::sync::Mutex<Vec<bridge_core::conversation::Message>>>,
    pre_turn_len: usize,
    journal_state: &Option<Arc<tools::journal::JournalState>>,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
) {
    *history = history_backup;
    // Remove the user message we pushed before the agent call —
    // no assistant response was generated, so leaving it would
    // create consecutive user messages in history.
    history.pop();
    persisted_messages.lock().unwrap().truncate(pre_turn_len);
    if let Some(ref js) = journal_state {
        js.discard_staged().await;
    }
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::AgentError,
        agent_id,
        conversation_id,
        json!({"code": "aborted", "message": "Turn aborted by user"}),
    ));
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::Done,
        agent_id,
        conversation_id,
        json!({}),
    ));
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::TurnCompleted,
        agent_id,
        conversation_id,
        json!({}),
    ));
}

/// Possible outcomes of the wait-for-streaming-result phase.
#[allow(clippy::type_complexity)]
pub(super) enum WaitDisposition {
    /// Agent-level shutdown fired; loop should break.
    Break,
    /// Turn-level abort fired; loop should `continue`.
    Continue,
    /// Streaming task returned with the given `chat_result`.
    ChatResult(
        Result<
            Result<
                (
                    Result<llm::PromptResponse, rig::completion::PromptError>,
                    Vec<rig::message::Message>,
                ),
                tokio::sync::oneshot::error::RecvError,
            >,
            tokio::time::error::Elapsed,
        >,
    ),
}

/// Wait for the streaming task and dispatch shutdown/abort side effects.
/// When the outer loop sees `Continue`, it advances `turn_count` and loops;
/// on `Break` it falls through to cleanup. On `ChatResult` it proceeds to
/// dispatch the streaming task's return value.
#[allow(clippy::too_many_arguments)]
pub(super) async fn wait_and_dispatch(
    cancel: &CancellationToken,
    turn_cancel: &CancellationToken,
    result_rx: oneshot::Receiver<(
        Result<llm::PromptResponse, rig::completion::PromptError>,
        Vec<rig::message::Message>,
    )>,
    agent_permissions: &HashMap<String, ToolPermission>,
    history: &mut Vec<rig::message::Message>,
    history_backup: Vec<rig::message::Message>,
    persisted_messages: &Arc<std::sync::Mutex<Vec<bridge_core::conversation::Message>>>,
    pre_turn_len: usize,
    journal_state: &Option<Arc<tools::journal::JournalState>>,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
) -> WaitDisposition {
    let outcome = wait_for_chat_result(
        cancel,
        turn_cancel,
        conversation_id,
        result_rx,
        agent_permissions,
    )
    .await;
    match outcome {
        ChatWaitOutcome::Shutdown => WaitDisposition::Break,
        ChatWaitOutcome::Abort => {
            handle_abort_cleanup(
                history,
                history_backup,
                persisted_messages,
                pre_turn_len,
                journal_state,
                event_bus,
                agent_id,
                conversation_id,
            )
            .await;
            WaitDisposition::Continue
        }
        ChatWaitOutcome::Got(r) => WaitDisposition::ChatResult(r),
    }
}

/// Perform end-of-conversation cleanup: drop pending approvals, clear the
/// session store, disconnect per-conversation MCP servers, decrement live
/// conversation count, and log a summary.
#[allow(clippy::too_many_arguments)]
pub(super) async fn run_conversation_cleanup(
    permission_manager: &Arc<llm::PermissionManager>,
    session_store: Option<Arc<crate::agent_runner::AgentSessionStore>>,
    per_conversation_mcp_scope: &Option<String>,
    mcp_manager: &Option<Arc<mcp::McpManager>>,
    metrics: &Arc<bridge_core::AgentMetrics>,
    conversation_metrics: &Arc<bridge_core::metrics::ConversationMetrics>,
    agent_id: &str,
    conversation_id: &str,
    turn_count: usize,
) {
    permission_manager.cleanup_conversation(conversation_id);

    if let Some(store) = session_store {
        store.remove_by_prefix(conversation_id);
    }

    // Disconnect any per-conversation MCP servers attached at creation time.
    if let (Some(scope), Some(manager)) = (per_conversation_mcp_scope, mcp_manager) {
        manager.disconnect_agent(scope).await;
    }

    crate::token_tracker::decrement_active_conversations(metrics);

    let cm = conversation_metrics.snapshot();
    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        turns = turn_count,
        input_tokens = cm.input_tokens,
        output_tokens = cm.output_tokens,
        model = %cm.model,
        duration_ms = cm.duration_ms,
        "conversation ended"
    );
}

/// Emit the "max_turns exceeded" event triple at the top of a turn.
pub(super) fn emit_max_turns_events(
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
    max: usize,
) {
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::AgentError,
        agent_id,
        conversation_id,
        json!({"code": "max_turns_exceeded", "message": format!("max turns ({}) exceeded", max)}),
    ));
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::Done,
        agent_id,
        conversation_id,
        json!({}),
    ));
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::TurnCompleted,
        agent_id,
        conversation_id,
        json!({}),
    ));
}
