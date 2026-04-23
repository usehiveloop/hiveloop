use bridge_core::conversation::Message;
use bridge_core::metrics::ConversationMetrics;
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use llm::PermissionManager;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use storage::StorageHandle;
use tokio::sync::RwLock;
use tokio_util::sync::CancellationToken;
use tools::agent::AgentContext;
use tools::ToolExecutor;
use tracing::error;
use webhooks::EventBus;

use super::params::AGENT_CHAT_TIMEOUT;
use super::turn_classify::emit_failed_turn_events;
use super::turn_success::handle_got_result;
use crate::token_tracker;

/// Context required to handle the result of a spawned streaming turn.
#[allow(clippy::too_many_arguments)]
pub(super) struct TurnResultCtx<'a> {
    pub(super) agent_id: &'a str,
    pub(super) conversation_id: &'a str,
    pub(super) agent: &'a Arc<RwLock<llm::BridgeAgent>>,
    pub(super) retry_agent: &'a Arc<llm::BridgeAgent>,
    pub(super) event_bus: &'a Arc<EventBus>,
    pub(super) metrics: &'a Arc<AgentMetrics>,
    pub(super) conversation_metrics: &'a Arc<ConversationMetrics>,
    pub(super) turn_cancel: &'a CancellationToken,
    pub(super) tool_names: &'a HashSet<String>,
    pub(super) tool_executors: &'a HashMap<String, Arc<dyn ToolExecutor>>,
    pub(super) agent_context: &'a Option<AgentContext>,
    pub(super) permission_manager: &'a Arc<PermissionManager>,
    pub(super) agent_permissions: &'a HashMap<String, ToolPermission>,
    pub(super) storage: &'a Option<StorageHandle>,
    pub(super) persisted_messages: &'a Arc<std::sync::Mutex<Vec<Message>>>,
    pub(super) journal_state: &'a Option<Arc<tools::journal::JournalState>>,
    pub(super) user_text: &'a str,
    pub(super) tool_calls_only: bool,
    pub(super) msg_id: &'a str,
    pub(super) tool_requirements: &'a [bridge_core::agent::ToolRequirement],
}

/// Outcome of processing a stream-turn result.
pub(super) enum TurnOutcome {
    /// The turn completed successfully (or recovery succeeded). The caller
    /// should advance `turn_count` and move on. The `new_history` is the
    /// enriched history to become the next turn's baseline.
    Completed {
        new_history: Vec<rig::message::Message>,
    },
    /// The turn failed with a genuine fatal error (or timeout/cancellation).
    /// History is restored to the backup; caller should just `turn_count += 1`
    /// and continue the loop.
    FatalRestored,
}

/// Inputs needed only at TurnResultCtx construction time. Grouped to keep
/// the caller's per-loop setup tidy.
#[allow(clippy::too_many_arguments)]
pub(super) fn build_turn_result_ctx<'a>(
    agent_id: &'a str,
    conversation_id: &'a str,
    agent: &'a Arc<RwLock<llm::BridgeAgent>>,
    retry_agent: &'a Arc<llm::BridgeAgent>,
    event_bus: &'a Arc<EventBus>,
    metrics: &'a Arc<AgentMetrics>,
    conversation_metrics: &'a Arc<ConversationMetrics>,
    turn_cancel: &'a CancellationToken,
    tool_names: &'a HashSet<String>,
    tool_executors: &'a HashMap<String, Arc<dyn ToolExecutor>>,
    agent_context: &'a Option<AgentContext>,
    permission_manager: &'a Arc<PermissionManager>,
    agent_permissions: &'a HashMap<String, ToolPermission>,
    storage: &'a Option<StorageHandle>,
    persisted_messages: &'a Arc<std::sync::Mutex<Vec<Message>>>,
    journal_state: &'a Option<Arc<tools::journal::JournalState>>,
    user_text: &'a str,
    tool_calls_only: bool,
    msg_id: &'a str,
    tool_requirements: &'a [bridge_core::agent::ToolRequirement],
) -> TurnResultCtx<'a> {
    TurnResultCtx {
        agent_id,
        conversation_id,
        agent,
        retry_agent,
        event_bus,
        metrics,
        conversation_metrics,
        turn_cancel,
        tool_names,
        tool_executors,
        agent_context,
        permission_manager,
        agent_permissions,
        storage,
        persisted_messages,
        journal_state,
        user_text,
        tool_calls_only,
        msg_id,
        tool_requirements,
    }
}

/// Handle the outer timeout/cancellation error from the spawned streaming task.
pub(super) async fn handle_timeout(
    ctx: &TurnResultCtx<'_>,
    history: &mut Vec<rig::message::Message>,
    history_backup: Vec<rig::message::Message>,
    pre_turn_len: usize,
    elapsed: std::time::Duration,
) {
    *history = history_backup;
    ctx.persisted_messages
        .lock()
        .unwrap()
        .truncate(pre_turn_len);
    error!(
        conversation_id = ctx.conversation_id,
        timeout_secs = AGENT_CHAT_TIMEOUT.as_secs(),
        elapsed_ms = elapsed.as_millis() as u64,
        "agent chat timed out"
    );
    token_tracker::record_error(ctx.metrics);
    if let Some(ref js) = ctx.journal_state {
        js.discard_staged().await;
    }
    emit_failed_turn_events(
        ctx.event_bus,
        ctx.agent_id,
        ctx.conversation_id,
        "agent_timeout",
        format!(
            "agent chat timed out after {}s",
            AGENT_CHAT_TIMEOUT.as_secs()
        ),
    );
}

/// Handle the case where the oneshot sender was dropped (task cancelled).
pub(super) async fn handle_task_cancelled(
    ctx: &TurnResultCtx<'_>,
    history: &mut Vec<rig::message::Message>,
    history_backup: Vec<rig::message::Message>,
    pre_turn_len: usize,
) {
    *history = history_backup;
    ctx.persisted_messages
        .lock()
        .unwrap()
        .truncate(pre_turn_len);
    error!(
        conversation_id = ctx.conversation_id,
        "agent chat task cancelled unexpectedly"
    );
    token_tracker::record_error(ctx.metrics);
    if let Some(ref js) = ctx.journal_state {
        js.discard_staged().await;
    }
    emit_failed_turn_events(
        ctx.event_bus,
        ctx.agent_id,
        ctx.conversation_id,
        "agent_error",
        "agent chat task cancelled".to_string(),
    );
}

/// Dispatch the three-way `chat_result` match: Err (timeout), Ok(Err) (task
/// cancelled), or Ok(Ok(...)) (success path that delegates to
/// [`handle_got_result`]).
///
/// On the Ok(Ok) branch returns `Some(new_history)` when the turn completed
/// (caller updates baseline + fingerprint) or `None` on fatal-restored
/// (caller restores from the backup it cloned before this call).
#[allow(clippy::too_many_arguments)]
#[allow(clippy::type_complexity)]
pub(super) async fn dispatch_chat_result(
    ctx: &TurnResultCtx<'_>,
    history: &mut Vec<rig::message::Message>,
    history_backup: Vec<rig::message::Message>,
    pre_turn_len: usize,
    persisted_user_message_clone: Message,
    start: std::time::Instant,
    turn_count: usize,
    enforcement_state: &mut Option<crate::tool_enforcement::ToolEnforcementState>,
    pending_tool_reminder: &mut Option<String>,
    chat_result: Result<
        Result<
            (
                Result<llm::PromptResponse, rig::completion::PromptError>,
                Vec<rig::message::Message>,
            ),
            tokio::sync::oneshot::error::RecvError,
        >,
        tokio::time::error::Elapsed,
    >,
) -> Option<Vec<rig::message::Message>> {
    match chat_result {
        Err(_timeout) => {
            handle_timeout(ctx, history, history_backup, pre_turn_len, start.elapsed()).await;
            None
        }
        Ok(Err(_)) => {
            handle_task_cancelled(ctx, history, history_backup, pre_turn_len).await;
            None
        }
        Ok(Ok((result, enriched_history))) => {
            let backup_for_fatal = history_backup.clone();
            match handle_got_result(
                ctx,
                history_backup,
                pre_turn_len,
                persisted_user_message_clone,
                start,
                turn_count,
                result,
                enriched_history,
                enforcement_state,
                pending_tool_reminder,
            )
            .await
            {
                TurnOutcome::Completed { new_history } => Some(new_history),
                TurnOutcome::FatalRestored => {
                    *history = backup_for_fatal;
                    None
                }
            }
        }
    }
}
