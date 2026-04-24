use bridge_core::metrics::ConversationMetrics;
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use dashmap::DashMap;
use llm::PermissionManager;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use storage::StorageHandle;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AGENT_CONTEXT};
use tools::ToolExecutor;
use webhooks::EventBus;

use super::stream_loop::{attempt_into_result, run_streaming_with_retry};
use tokio::sync::{Mutex, RwLock};
use tracing::{info_span, Instrument};

/// The scalar output of [`prepare_turn`] — the cancellation token used to
/// abort the turn, the backup history used for rollback on error, and the
/// LLM semaphore permit the spawned task must hold.
pub(super) struct TurnPrep {
    pub(super) turn_cancel: tokio_util::sync::CancellationToken,
    pub(super) history_backup: Vec<rig::message::Message>,
    pub(super) llm_permit: tokio::sync::OwnedSemaphorePermit,
    pub(super) stream_prep: StreamTurnPrep,
}

/// Prepare a turn for streaming: install a fresh abort token, snapshot the
/// agent (so API-key rotations apply), take ownership of `history` for the
/// spawned task (keeping a backup for rollback), compute the immortal-mode
/// pressure threshold, and acquire the LLM semaphore permit.
///
/// Returns `None` when the LLM semaphore has been closed (runtime shutting
/// down). The caller should break the loop in that case.
pub(super) async fn prepare_turn(
    abort_token: &Arc<Mutex<tokio_util::sync::CancellationToken>>,
    agent: &Arc<RwLock<llm::BridgeAgent>>,
    user_text: &str,
    history: &mut Vec<rig::message::Message>,
    immortal_config: &Option<bridge_core::agent::ImmortalConfig>,
    llm_semaphore: &Arc<tokio::sync::Semaphore>,
) -> Option<TurnPrep> {
    let turn_cancel = tokio_util::sync::CancellationToken::new();
    {
        let mut guard = abort_token.lock().await;
        *guard = turn_cancel.clone();
    }

    // Clone the agent from behind the RwLock so API key rotations are picked up.
    let agent_clone = { agent.read().await.clone() };
    let user_text_clone = user_text.to_string();
    // Zero-copy: take ownership of history instead of cloning. We get it back
    // via the oneshot channel. Keep a backup only for error recovery paths.
    let history_backup = history.clone();
    let history_for_task = std::mem::take(history);
    let pressure_threshold_bytes_per_turn: Option<usize> = immortal_config
        .as_ref()
        .map(|cfg| ((cfg.token_budget as f32) * 1.5 * 4.0) as usize);
    let llm_permit = match llm_semaphore.clone().acquire_owned().await {
        Ok(p) => p,
        Err(_) => {
            // Semaphore closed — runtime is shutting down
            let _ = history_backup;
            return None;
        }
    };

    Some(TurnPrep {
        turn_cancel,
        history_backup,
        llm_permit,
        stream_prep: StreamTurnPrep {
            agent_clone,
            history_for_task,
            user_text_clone,
            pressure_threshold_bytes_per_turn,
        },
    })
}

/// Spawn the streaming task with the `gen_ai.chat` tracing span, giving it
/// exclusive ownership of the LLM semaphore permit for the duration of the
/// call. The oneshot receiver resolves when the task has finished.
pub(super) fn spawn_streaming_task(
    stream_inputs: StreamTurnInputs,
    llm_permit: tokio::sync::OwnedSemaphorePermit,
    agent_id: &str,
    conversation_id: &str,
    turn_count: usize,
) -> tokio::sync::oneshot::Receiver<(
    Result<llm::PromptResponse, rig::completion::PromptError>,
    Vec<rig::message::Message>,
)> {
    let (result_tx, result_rx) = tokio::sync::oneshot::channel();
    let turn_span = info_span!(
        "gen_ai.chat",
        "gen_ai.operation.name" = "chat",
        "gen_ai.agent.id" = %agent_id,
        "gen_ai.conversation.id" = %conversation_id,
        "turn_number" = turn_count,
    );
    tokio::spawn(
        async move {
            let _llm_permit = llm_permit;
            let result = run_stream_turn(stream_inputs).await;
            let _ = result_tx.send(result);
        }
        .instrument(turn_span),
    );
    result_rx
}

/// Inputs used by [`build_stream_inputs`] when preparing a streaming turn.
/// Captures only the non-cloneable ingredients that need to be set once per
/// turn (agent snapshot, history for task, pressure threshold).
pub(super) struct StreamTurnPrep {
    pub(super) agent_clone: llm::BridgeAgent,
    pub(super) history_for_task: Vec<rig::message::Message>,
    pub(super) user_text_clone: String,
    pub(super) pressure_threshold_bytes_per_turn: Option<usize>,
}

/// Combine per-turn prep with conversation-wide clones to produce the full
/// [`StreamTurnInputs`] struct, avoiding repetition of all 18 fields in `run.rs`.
#[allow(clippy::too_many_arguments)]
pub(super) fn build_stream_inputs(
    prep: StreamTurnPrep,
    event_bus: &Arc<EventBus>,
    agent_context: &Option<AgentContext>,
    turn_cancel: &CancellationToken,
    tool_names: &HashSet<String>,
    tool_executors: &HashMap<String, Arc<dyn ToolExecutor>>,
    agent_id: &str,
    conversation_id: &str,
    permission_manager: &Arc<PermissionManager>,
    agent_permissions: &HashMap<String, ToolPermission>,
    metrics: &Arc<AgentMetrics>,
    conversation_metrics: &Arc<ConversationMetrics>,
    msg_id: &str,
    storage: &Option<StorageHandle>,
    persisted_messages: &Arc<std::sync::Mutex<Vec<bridge_core::conversation::Message>>>,
    repeat_guard: &Arc<std::sync::Mutex<llm::RepeatGuardState>>,
) -> StreamTurnInputs {
    StreamTurnInputs {
        agent_clone: prep.agent_clone,
        history_for_task: prep.history_for_task,
        user_text_clone: prep.user_text_clone,
        event_bus_clone: event_bus.clone(),
        agent_context_clone: agent_context.clone(),
        turn_cancel_clone: turn_cancel.clone(),
        tool_names_clone: tool_names.clone(),
        tool_executors_clone: tool_executors.clone(),
        agent_id_clone: agent_id.to_string(),
        conversation_id_clone: conversation_id.to_string(),
        permission_manager_clone: permission_manager.clone(),
        agent_permissions_clone: agent_permissions.clone(),
        metrics_for_task: metrics.clone(),
        conversation_metrics_for_task: conversation_metrics.clone(),
        msg_id_clone: msg_id.to_string(),
        pressure_threshold_bytes_per_turn: prep.pressure_threshold_bytes_per_turn,
        storage_for_emitter: storage.clone(),
        persisted_messages_for_emitter: persisted_messages.clone(),
        repeat_guard_for_emitter: repeat_guard.clone(),
    }
}

/// Inputs for [`run_stream_turn`].
#[allow(clippy::too_many_arguments)]
pub(super) struct StreamTurnInputs {
    pub(super) agent_clone: llm::BridgeAgent,
    pub(super) history_for_task: Vec<rig::message::Message>,
    pub(super) user_text_clone: String,
    pub(super) event_bus_clone: Arc<EventBus>,
    pub(super) agent_context_clone: Option<AgentContext>,
    pub(super) turn_cancel_clone: CancellationToken,
    pub(super) tool_names_clone: HashSet<String>,
    pub(super) tool_executors_clone: HashMap<String, Arc<dyn ToolExecutor>>,
    pub(super) agent_id_clone: String,
    pub(super) conversation_id_clone: String,
    pub(super) permission_manager_clone: Arc<PermissionManager>,
    pub(super) agent_permissions_clone: HashMap<String, ToolPermission>,
    pub(super) metrics_for_task: Arc<AgentMetrics>,
    pub(super) conversation_metrics_for_task: Arc<ConversationMetrics>,
    pub(super) msg_id_clone: String,
    pub(super) pressure_threshold_bytes_per_turn: Option<usize>,
    pub(super) storage_for_emitter: Option<StorageHandle>,
    pub(super) persisted_messages_for_emitter:
        Arc<std::sync::Mutex<Vec<bridge_core::conversation::Message>>>,
    pub(super) repeat_guard_for_emitter: Arc<std::sync::Mutex<llm::RepeatGuardState>>,
}

/// Run the stream prompt loop — pre-stream retry on retryable errors,
/// emitting SSE text-delta events incrementally. Returns the final rig
/// response result and the enriched history.
pub(super) async fn run_stream_turn(
    inputs: StreamTurnInputs,
) -> (
    Result<llm::PromptResponse, rig::completion::PromptError>,
    Vec<rig::message::Message>,
) {
    let StreamTurnInputs {
        agent_clone,
        history_for_task,
        user_text_clone,
        event_bus_clone,
        agent_context_clone,
        turn_cancel_clone,
        tool_names_clone,
        tool_executors_clone,
        agent_id_clone,
        conversation_id_clone,
        permission_manager_clone,
        agent_permissions_clone,
        metrics_for_task,
        conversation_metrics_for_task,
        msg_id_clone,
        pressure_threshold_bytes_per_turn,
        storage_for_emitter,
        persisted_messages_for_emitter,
        repeat_guard_for_emitter,
    } = inputs;

    // Extra clones for streaming text delta emission (the originals
    // are moved into the ToolCallEmitter for tool-call SSE events).
    let event_bus_for_text = event_bus_clone.clone();
    let agent_id_for_text = agent_id_clone.clone();
    let conversation_id_for_text = conversation_id_clone.clone();

    let emitter = llm::ToolCallEmitter {
        event_bus: event_bus_clone,
        cancel: turn_cancel_clone,
        tool_names: tool_names_clone,
        tool_executors: tool_executors_clone,
        agent_id: agent_id_clone,
        conversation_id: conversation_id_clone,
        permission_manager: permission_manager_clone,
        agent_permissions: agent_permissions_clone,
        metrics: metrics_for_task,
        conversation_metrics: Some(conversation_metrics_for_task),
        pending_tool_timings: Arc::new(DashMap::new()),
        storage: storage_for_emitter.clone(),
        persisted_messages: Some(persisted_messages_for_emitter.clone()),
        // Mid-turn pressure warning: fire once per turn when tool-output
        // bytes exceed ~1.5× the immortal token budget (rough heuristic
        // of 4 bytes per token). Disabled when immortal mode is off.
        pressure_threshold_bytes: pressure_threshold_bytes_per_turn,
        pressure_counter: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        pressure_warned: Arc::new(std::sync::atomic::AtomicBool::new(false)),
        repeat_guard: repeat_guard_for_emitter,
    };

    let fut = async move {
        let attempt = run_streaming_with_retry(
            &agent_clone,
            &user_text_clone,
            &history_for_task,
            emitter,
            &event_bus_for_text,
            &agent_id_for_text,
            &conversation_id_for_text,
            &msg_id_clone,
        )
        .await;
        attempt_into_result(attempt)
    };

    // Wrap in AGENT_CONTEXT scope if available
    match agent_context_clone {
        Some(ctx) => AGENT_CONTEXT.scope(ctx, fut).await,
        None => fut.await,
    }
}
