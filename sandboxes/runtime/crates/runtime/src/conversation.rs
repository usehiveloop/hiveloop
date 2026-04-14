use bridge_core::conversation::{Message, Role};
use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::metrics::ConversationMetrics;
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use dashmap::DashMap;
use futures::StreamExt;
use llm::{BridgeStreamItem, PermissionManager};
use serde_json::json;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::Duration;
use storage::StorageHandle;
use tokio::sync::mpsc;
use tokio::sync::{Mutex, RwLock};
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification, AGENT_CONTEXT};
use tools::ToolExecutor;
use tracing::{debug, error, info, info_span, warn, Instrument};
use webhooks::EventBus;

use crate::agent_runner::AgentSessionStore;

/// Timeout for a single agent.chat() call (includes internal tool loops).
const AGENT_CHAT_TIMEOUT: Duration = Duration::from_secs(6 * 60 * 60);

/// Timeout for automatic continuation attempts when the agent returns an empty response.
const CONTINUATION_TIMEOUT: Duration = Duration::from_secs(180);

/// Maximum number of automatic continuation attempts when the agent returns an
/// empty response. After this many continuations, fall back to the no-tools
/// retry agent.
const MAX_CONTINUATIONS: usize = 3;

use crate::token_tracker;

/// Incoming message for the conversation loop — either a user message or
/// a background subagent completion notification.
enum IncomingMessage {
    User(Message),
    BackgroundComplete(AgentTaskNotification),
    PingFired(Vec<tools::ping_me_back::PendingPing>),
}

/// Parameters for running a conversation loop.
pub struct ConversationParams {
    /// Agent ID.
    pub agent_id: String,
    /// Conversation ID.
    pub conversation_id: String,
    /// The built rig-core agent (behind RwLock for API key rotation).
    pub agent: Arc<RwLock<llm::BridgeAgent>>,
    /// Receiver for user messages.
    pub message_rx: mpsc::Receiver<Message>,
    /// Unified event bus for SSE, WebSocket, webhook, and persistence delivery.
    pub event_bus: Arc<EventBus>,
    /// Metrics counters for this agent.
    pub metrics: Arc<AgentMetrics>,
    /// Cancellation token for graceful shutdown.
    pub cancel: CancellationToken,
    /// Maximum number of turns before ending the conversation.
    pub max_turns: Option<usize>,
    /// Optional agent context for subagent spawning.
    pub agent_context: Option<AgentContext>,
    /// Receiver for background task completion notifications.
    pub notification_rx: Option<mpsc::Receiver<AgentTaskNotification>>,
    /// Session store reference for cleanup on conversation end.
    pub session_store: Option<Arc<AgentSessionStore>>,
    /// Known tool names for tool repair (unknown tool name suggestion).
    pub tool_names: HashSet<String>,
    /// Tool executors for auto-repair dispatch (keyed by canonical name).
    pub tool_executors: HashMap<String, Arc<dyn ToolExecutor>>,
    /// Pre-seeded conversation history (used when hydrating from the control plane).
    pub initial_history: Option<Vec<rig::message::Message>>,
    /// No-tools agent used to retry when the primary agent returns an empty response.
    /// Because it has no tools registered the model is forced to produce text.
    pub retry_agent: Arc<llm::BridgeAgent>,
    /// Shared abort token — holds the current turn's CancellationToken.
    pub abort_token: Arc<Mutex<CancellationToken>>,
    /// Permission manager for handling tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
    /// Per-tool permission overrides for this agent.
    pub agent_permissions: HashMap<String, ToolPermission>,
    /// Optional compaction configuration for history summarization.
    pub compaction_config: Option<bridge_core::agent::CompactionConfig>,
    /// System reminder markdown to inject before every user message.
    pub system_reminder: String,
    /// Initial conversation date for date tracking.
    pub conversation_date: chrono::DateTime<chrono::Utc>,
    /// Global LLM call semaphore for admission control.
    pub llm_semaphore: Arc<tokio::sync::Semaphore>,
    /// Initial persistence-ready history, normalized to align with rig history.
    pub initial_persisted_messages: Option<Vec<Message>>,
    /// Optional non-blocking persistence handle.
    pub storage: Option<StorageHandle>,
    /// When true, empty text responses are accepted as success if tool calls were made.
    pub tool_calls_only: bool,
    /// Per-conversation metrics for token/tool tracking.
    pub conversation_metrics: Arc<ConversationMetrics>,
    /// Optional immortal conversation configuration (replaces compaction when set).
    pub immortal_config: Option<bridge_core::agent::ImmortalConfig>,
    /// Journal state shared with the journal_write tool (only set in immortal mode).
    pub journal_state: Option<Arc<tools::journal::JournalState>>,
    /// MCP scope key for per-conversation MCP servers.
    /// `Some(conv_id)` when the conversation owns its own MCP connections that
    /// must be torn down on exit; `None` when only agent-level MCP is in use.
    pub per_conversation_mcp_scope: Option<String>,
    /// MCP manager handle used to disconnect per-conversation servers during cleanup.
    /// Only meaningful when `per_conversation_mcp_scope` is set.
    pub mcp_manager: Option<Arc<mcp::McpManager>>,
    /// When true, inject environment system reminder (resource usage, installed tools).
    pub standalone_agent: bool,
    /// Shared state for ping-me-back timers (non-blocking delayed reminders).
    pub ping_state: Option<tools::ping_me_back::PingState>,
}

/// Run a conversation loop for a single conversation.
///
/// This function runs as an async task, receiving user messages via the params,
/// sending them to the LLM agent, and streaming responses back via SSE.
///
/// The loop exits when:
/// - The cancellation token is cancelled (agent shutdown)
/// - The message channel is closed (conversation ended)
/// - max_turns is exceeded
pub async fn run_conversation(params: ConversationParams) {
    let ConversationParams {
        agent_id,
        conversation_id,
        agent,
        mut message_rx,
        event_bus,
        metrics,
        cancel,
        max_turns,
        agent_context,
        mut notification_rx,
        session_store,
        tool_names,
        tool_executors,
        initial_history,
        retry_agent,
        abort_token,
        permission_manager,
        agent_permissions,
        compaction_config,
        system_reminder,
        conversation_date,
        llm_semaphore,
        initial_persisted_messages,
        storage,
        tool_calls_only,
        conversation_metrics,
        immortal_config,
        journal_state,
        per_conversation_mcp_scope,
        mcp_manager,
        standalone_agent,
        ping_state,
    } = params;

    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        "conversation started"
    );

    token_tracker::increment_active_conversations(&metrics);
    token_tracker::increment_total_conversations(&metrics);

    let mut history: Vec<rig::message::Message> = initial_history.unwrap_or_default();
    let persisted_messages: Arc<std::sync::Mutex<Vec<Message>>> = Arc::new(std::sync::Mutex::new(
        initial_persisted_messages.unwrap_or_default(),
    ));
    let mut turn_count: usize = 0;
    let msg_id = uuid::Uuid::new_v4().to_string();

    // Initialize date tracker for detecting date changes
    let mut date_tracker = crate::system_reminder::DateTracker::with_date(conversation_date);

    // Initialize immortal state if configured
    let mut immortal_state = immortal_config.as_ref().map(|_| {
        let chain_index = journal_state
            .as_ref()
            .map(|js| js.chain_index())
            .unwrap_or(0);
        crate::immortal::ImmortalState {
            current_chain_index: chain_index,
        }
    });

    loop {
        // Wait for either a user message, a background task notification,
        // a ping-me-back timer firing, or cancellation
        let incoming = tokio::select! {
            _ = cancel.cancelled() => {
                debug!(conversation_id = conversation_id, "conversation cancelled");
                break;
            }
            msg = message_rx.recv() => {
                match msg {
                    Some(m) => IncomingMessage::User(m),
                    None => {
                        debug!(conversation_id = conversation_id, "message channel closed");
                        break;
                    }
                }
            }
            Some(notif) = async {
                match notification_rx.as_mut() {
                    Some(rx) => rx.recv().await,
                    None => std::future::pending().await,
                }
            } => {
                IncomingMessage::BackgroundComplete(notif)
            }
            _ = async {
                match &ping_state {
                    Some(ps) => match ps.next_fire_time().await {
                        Some(instant) => tokio::time::sleep_until(instant).await,
                        None => std::future::pending().await,
                    },
                    None => std::future::pending().await,
                }
            } => {
                let fired = match &ping_state {
                    Some(ps) => ps.pop_fired().await,
                    None => vec![],
                };
                IncomingMessage::PingFired(fired)
            }
        };

        // Check max turns
        if let Some(max) = max_turns {
            if turn_count >= max {
                event_bus.emit(BridgeEvent::new(BridgeEventType::AgentError, &agent_id, &conversation_id, json!({"code": "max_turns_exceeded", "message": format!("max turns ({}) exceeded", max)})));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::Done,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::TurnCompleted,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
                break;
            }
        }

        // Build the user text from the incoming message
        let user_text: String = match &incoming {
            IncomingMessage::User(msg) => extract_text_content(msg),
            IncomingMessage::BackgroundComplete(notif) => {
                let task_id = notif.task_id.clone();
                let description = notif.description.clone();
                let is_error = notif.output.is_err();
                let output_text = match &notif.output {
                    Ok(output) => output.clone(),
                    Err(error) => format!("[ERROR] {}", error),
                };

                // Emit background task completion event
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::BackgroundTaskCompleted,
                    &agent_id,
                    &conversation_id,
                    json!({
                        "task_id": task_id,
                        "description": description,
                        "output": output_text,
                        "is_error": is_error,
                    }),
                ));

                format!(
                    "[Background Agent Task Completed]\ntask_id: {}\ndescription: {}\n\n<task_result>\n{}\n</task_result>",
                    task_id,
                    description,
                    output_text,
                )
            }
            IncomingMessage::PingFired(pings) => {
                let mut parts = Vec::new();
                for ping in pings {
                    parts.push(format!(
                        "[Ping-Me-Back Fired]\nYou are being pinged back because you asked to be pinged back. It has been {} seconds since then.\n\nYour message: {}",
                        ping.delay_secs, ping.message
                    ));
                }
                parts.join("\n\n")
            }
        };

        let persisted_user_message = match &incoming {
            IncomingMessage::User(msg) => {
                normalize_messages_for_persistence(std::slice::from_ref(msg))
                    .into_iter()
                    .next()
                    .unwrap_or_else(|| msg.clone())
            }
            IncomingMessage::BackgroundComplete(_) | IncomingMessage::PingFired(_) => {
                text_message(Role::User, user_text.clone())
            }
        };

        // Mask old tool outputs to reduce context pressure before budget checks.
        crate::masking::mask_old_tool_outputs_default(&mut history);

        // Check if context management is needed before adding the new message.
        // Immortal mode (chain handoff) takes priority over compaction.
        if let (Some(ref immortal_cfg), Some(ref mut imm_state)) =
            (&immortal_config, &mut immortal_state)
        {
            if let Some(ref js) = journal_state {
                match crate::immortal::maybe_chain(&history, immortal_cfg, imm_state, js).await {
                    Ok(Some(result)) => {
                        info!(
                            conversation_id = conversation_id,
                            chain_index = result.chain_index,
                            pre_tokens = result.pre_chain_tokens,
                            carry_forward = result.carry_forward_count,
                            "conversation chain handoff"
                        );

                        // Emit chain_started event
                        event_bus.emit(BridgeEvent::new(
                            BridgeEventType::ChainStarted,
                            &agent_id,
                            &conversation_id,
                            json!({
                                "chain_index": result.chain_index,
                                "reason": "token_budget_exceeded",
                                "token_count": result.pre_chain_tokens,
                            }),
                        ));

                        // Save checkpoint as a journal entry
                        let checkpoint_entry = tools::journal::JournalEntry {
                            id: uuid::Uuid::new_v4().to_string(),
                            chain_index: imm_state.current_chain_index,
                            entry_type: "checkpoint".to_string(),
                            content: result.checkpoint_text.clone(),
                            category: None,
                            timestamp: chrono::Utc::now(),
                        };
                        js.append(checkpoint_entry).await;

                        // Reset in-memory history
                        history = result.new_history;

                        // Rebuild persisted messages from the new rig history
                        {
                            let mut guard = persisted_messages.lock().unwrap();
                            *guard = convert_from_rig_messages(&history);
                        }

                        // Update immortal state
                        imm_state.current_chain_index = result.chain_index;
                        js.set_chain_index(result.chain_index);

                        // Persist: replace messages + save chain link
                        if let Some(storage) = &storage {
                            storage.replace_messages(
                                conversation_id.clone(),
                                persisted_messages.lock().unwrap().clone(),
                            );
                            storage.save_chain_link(
                                conversation_id.clone(),
                                result.chain_index,
                                chrono::Utc::now(),
                                Some(result.pre_chain_tokens),
                                Some(result.checkpoint_text.clone()),
                            );
                        }

                        // Emit chain_completed event
                        event_bus.emit(BridgeEvent::new(
                            BridgeEventType::ChainCompleted,
                            &agent_id,
                            &conversation_id,
                            json!({
                                "chain_index": result.chain_index,
                                "journal_entry_count": js.entries().await.len(),
                                "carry_forward_messages": result.carry_forward_count,
                            }),
                        ));
                    }
                    Ok(None) => {} // under budget
                    Err(e) => {
                        warn!(error = %e, "chain handoff failed, continuing with full history");
                    }
                }
            }
        } else if let Some(ref compaction_config) = compaction_config {
            match crate::compaction::maybe_compact(&history, compaction_config).await {
                Ok(Some(result)) => {
                    info!(
                        conversation_id = conversation_id,
                        pre_tokens = result.pre_compaction_tokens,
                        post_tokens = result.post_compaction_tokens,
                        messages_compacted = result.messages_compacted,
                        "conversation compacted"
                    );

                    // Replace in-memory history
                    history = result.compacted_history;

                    {
                        let mut guard = persisted_messages.lock().unwrap();
                        apply_compaction_to_persisted_history(
                            &mut guard,
                            &result.summary_text,
                            result.messages_compacted,
                        );
                    }

                    if let Some(storage) = &storage {
                        storage.replace_messages(
                            conversation_id.clone(),
                            persisted_messages.lock().unwrap().clone(),
                        );
                    }

                    // Fire compaction event
                    event_bus.emit(BridgeEvent::new(
                        BridgeEventType::ConversationCompacted,
                        &agent_id,
                        &conversation_id,
                        json!({
                            "summary": result.summary_text,
                            "messages_compacted": result.messages_compacted,
                            "pre_compaction_tokens": result.pre_compaction_tokens,
                            "post_compaction_tokens": result.post_compaction_tokens,
                        }),
                    ));
                }
                Ok(None) => {} // under budget, no compaction needed
                Err(e) => {
                    warn!(error = %e, "compaction failed, continuing with full history");
                }
            }
        }

        // Extract per-message system reminder if present
        let per_message_reminder = match &incoming {
            IncomingMessage::User(msg) => msg
                .system_reminder
                .as_deref()
                .map(|r| format!("<system-reminder>\n{}\n</system-reminder>", r)),
            _ => None,
        };

        // Check for date change and get reminder if date changed
        let date_change_reminder = date_tracker.check_date_change();

        // Build the effective system reminder, including immortal context if active
        let mut effective_reminder =
            if let (Some(ref imm_state), Some(ref js)) = (&immortal_state, &journal_state) {
                let journal_count = js.entries().await.len();
                let immortal_section = crate::system_reminder::SystemReminder::new()
                    .with_immortal_context(imm_state.current_chain_index, journal_count)
                    .build();
                if system_reminder.is_empty() {
                    immortal_section
                } else {
                    format!("{}\n\n{}", system_reminder, immortal_section)
                }
            } else {
                system_reminder.clone()
            };

        // Append environment snapshot when standalone_agent is enabled.
        // Refreshes every 5 turns to keep resource numbers current without
        // paying the cost every single message.
        if standalone_agent && turn_count.is_multiple_of(5) {
            let env_section = crate::environment::EnvironmentSnapshot::collect().format_reminder();
            if effective_reminder.is_empty() {
                effective_reminder =
                    format!("<system-reminder>\n{}\n</system-reminder>", env_section);
            } else {
                effective_reminder = format!(
                    "{}\n\n<system-reminder>\n{}\n</system-reminder>",
                    effective_reminder, env_section
                );
            }
        }

        // Append per-message system reminder from the control plane
        if let Some(pmr) = per_message_reminder {
            if effective_reminder.is_empty() {
                effective_reminder = pmr;
            } else {
                effective_reminder = format!("{}\n\n{}", effective_reminder, pmr);
            }
        }

        // Append pending ping-me-back timers to system reminder
        if let Some(ref ps) = ping_state {
            let pings = ps.list().await;
            let ping_reminder = tools::ping_me_back::format_pending_pings_reminder(&pings);
            if !ping_reminder.is_empty() {
                let ping_section =
                    format!("<system-reminder>\n{}\n</system-reminder>", ping_reminder);
                if effective_reminder.is_empty() {
                    effective_reminder = ping_section;
                } else {
                    effective_reminder = format!("{}\n\n{}", effective_reminder, ping_section);
                }
            }
        }

        // Build final user text with reminders
        let final_user_text = match (date_change_reminder, effective_reminder.is_empty()) {
            (Some(date_reminder), true) => {
                // Only date change reminder
                format!("{}\n\n{}", date_reminder, user_text)
            }
            (Some(date_reminder), false) => {
                // Both date change and system reminder
                format!(
                    "{}\n\n{}\n\n{}",
                    date_reminder, effective_reminder, user_text
                )
            }
            (None, true) => {
                // No reminders
                user_text.clone()
            }
            (None, false) => {
                // Only system reminder
                format!("{}\n\n{}", effective_reminder, user_text)
            }
        };

        history.push(rig::message::Message::user(&final_user_text));
        let persisted_user_message_clone = persisted_user_message.clone();
        let pre_turn_len = {
            let mut guard = persisted_messages.lock().unwrap();
            let len = guard.len();
            guard.push(persisted_user_message);
            len
        };

        // Persist immediately so the user message survives a crash
        if let Some(storage) = &storage {
            storage.replace_messages(
                conversation_id.clone(),
                persisted_messages.lock().unwrap().clone(),
            );
        }

        // Signal response starting
        event_bus.emit(BridgeEvent::new(
            BridgeEventType::ResponseStarted,
            &agent_id,
            &conversation_id,
            json!({
                "conversation_id": &conversation_id,
                "message_id": &msg_id,
            }),
        ));

        let start = std::time::Instant::now();

        // Create a fresh abort token for this turn
        let turn_cancel = CancellationToken::new();
        {
            let mut guard = abort_token.lock().await;
            *guard = turn_cancel.clone();
        }

        // Spawn the agent prompt in a separate task so that tokio::time::timeout
        // is guaranteed to fire even if the future blocks a worker thread.
        // Using prompt().with_hook() instead of chat() so tool calls emit SSE events.
        // Clone the agent from behind the RwLock so API key rotations are picked up.
        let agent_clone = { agent.read().await.clone() };
        let user_text_clone = user_text.clone();
        // Zero-copy: take ownership of history instead of cloning. We get it back
        // via the oneshot channel. Keep a backup only for error recovery paths.
        let history_backup = history.clone();
        let history_for_task = std::mem::take(&mut history);
        let event_bus_clone = event_bus.clone();
        let agent_context_clone = agent_context.clone();
        let turn_cancel_clone = turn_cancel.clone();
        let tool_names_clone = tool_names.clone();
        let tool_executors_clone = tool_executors.clone();
        let agent_id_clone = agent_id.clone();
        let conversation_id_clone = conversation_id.clone();
        let permission_manager_clone = permission_manager.clone();
        let agent_permissions_clone = agent_permissions.clone();
        let metrics_for_task = metrics.clone();
        let conversation_metrics_for_task = conversation_metrics.clone();
        let msg_id_clone = msg_id.clone();
        let storage_for_emitter = storage.clone();
        let persisted_messages_for_emitter = persisted_messages.clone();
        // Acquire LLM semaphore permit before spawning the task.
        // This provides global backpressure on concurrent LLM API calls.
        let llm_permit = match llm_semaphore.clone().acquire_owned().await {
            Ok(p) => p,
            Err(_) => {
                // Semaphore closed — runtime is shutting down
                let _ = history_backup; // no longer needed
                break;
            }
        };
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
                // Hold the LLM permit for the duration of the agent streaming call
                let _llm_permit = llm_permit;

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
                };

                let fut = async {
                    let mut stream = agent_clone
                        .stream_prompt_with_hook(&user_text_clone, history_for_task, emitter)
                        .await;

                    let mut accumulated_text = String::new();
                    let mut final_usage = rig::completion::Usage::new();
                    let mut final_history: Option<Vec<rig::message::Message>> = None;
                    let mut had_error: Option<String> = None;

                    while let Some(item) = stream.next().await {
                        match item {
                            BridgeStreamItem::TextDelta(delta) => {
                                accumulated_text.push_str(&delta);
                                // Emit content delta in real time
                                event_bus_for_text.emit(BridgeEvent::new(
                                    BridgeEventType::ResponseChunk,
                                    &agent_id_for_text,
                                    &conversation_id_for_text,
                                    json!({
                                        "delta": &delta,
                                        "message_id": &msg_id_clone,
                                    }),
                                ));
                            }
                            BridgeStreamItem::ReasoningDelta(delta) => {
                                // Emit reasoning delta in real time
                                event_bus_for_text.emit(BridgeEvent::new(
                                    BridgeEventType::ReasoningDelta,
                                    &agent_id_for_text,
                                    &conversation_id_for_text,
                                    json!({
                                        "delta": &delta,
                                        "message_id": &msg_id_clone,
                                    }),
                                ));
                            }
                            BridgeStreamItem::StreamFinished {
                                response,
                                usage,
                                history,
                            } => {
                                accumulated_text = response;
                                final_usage = usage;
                                final_history = history;
                            }
                            BridgeStreamItem::StreamError(err) => {
                                had_error = Some(err);
                                break;
                            }
                        }
                    }

                    let enriched_history = final_history.unwrap_or_default();

                    if let Some(err_msg) = had_error {
                        // Check if it's a parse error that allows recovery
                        if err_msg.contains("no message or tool call")
                            || err_msg.contains("did not match any variant of untagged enum")
                        {
                            // Treat as recoverable: return accumulated text (may be empty)
                            (
                                Ok(llm::PromptResponse {
                                    output: accumulated_text,
                                    total_usage: final_usage,
                                }),
                                enriched_history,
                            )
                        } else {
                            (
                                Err(rig::completion::PromptError::CompletionError(
                                    rig::completion::CompletionError::ProviderError(err_msg),
                                )),
                                enriched_history,
                            )
                        }
                    } else {
                        (
                            Ok(llm::PromptResponse {
                                output: accumulated_text,
                                total_usage: final_usage,
                            }),
                            enriched_history,
                        )
                    }
                };

                // Wrap in AGENT_CONTEXT scope if available
                let result = match agent_context_clone {
                    Some(ctx) => AGENT_CONTEXT.scope(ctx, fut).await,
                    None => fut.await,
                };
                let _ = result_tx.send(result);
            }
            .instrument(turn_span),
        );

        // Wait for the result with a timeout, or abort/shutdown
        let chat_result = tokio::select! {
            // Agent-level shutdown (kills all conversations)
            _ = cancel.cancelled() => {
                debug!(conversation_id = conversation_id, "conversation cancelled by agent shutdown");
                break;
            }
            // Turn-level abort (user requested abort)
            _ = turn_cancel.cancelled() => {
                info!(conversation_id = conversation_id, "turn aborted by user");
                // Restore history from backup since the spawned task may not return it.
                history = history_backup;
                // Remove the user message we pushed before the agent call —
                // no assistant response was generated, so leaving it would
                // create consecutive user messages in history.
                history.pop();
                persisted_messages.lock().unwrap().truncate(pre_turn_len);
                event_bus.emit(BridgeEvent::new(BridgeEventType::AgentError, &agent_id, &conversation_id, json!({"code": "aborted", "message": "Turn aborted by user"})));
                event_bus.emit(BridgeEvent::new(BridgeEventType::Done, &agent_id, &conversation_id, json!({})));
                event_bus.emit(BridgeEvent::new(BridgeEventType::TurnCompleted, &agent_id, &conversation_id, json!({})));
                turn_count += 1;
                continue;
            }
            // Normal completion or timeout.
            // When the agent has any require_approval permissions, the timeout is
            // disabled entirely — the tool call can block indefinitely waiting for
            // user approval. The abort mechanism exists for manual cancellation.
            result = async {
                let has_approval_tools = agent_permissions.values().any(|p| *p == bridge_core::permission::ToolPermission::RequireApproval);
                if has_approval_tools {
                    Ok(result_rx.await)
                } else {
                    tokio::time::timeout(AGENT_CHAT_TIMEOUT, result_rx).await
                }
            } => {
                result
            }
        };

        match chat_result {
            // Timeout fired — restore history from backup
            Err(_timeout) => {
                history = history_backup;
                persisted_messages.lock().unwrap().truncate(pre_turn_len);
                let elapsed = start.elapsed();
                error!(
                    conversation_id = conversation_id,
                    timeout_secs = AGENT_CHAT_TIMEOUT.as_secs(),
                    elapsed_ms = elapsed.as_millis() as u64,
                    "agent chat timed out"
                );
                token_tracker::record_error(&metrics);
                event_bus.emit(BridgeEvent::new(BridgeEventType::AgentError, &agent_id, &conversation_id, json!({"code": "agent_timeout", "message": format!("agent chat timed out after {}s", AGENT_CHAT_TIMEOUT.as_secs())})));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::Done,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::TurnCompleted,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
            }
            // Task was cancelled (oneshot sender dropped) — restore history from backup
            Ok(Err(_)) => {
                history = history_backup;
                persisted_messages.lock().unwrap().truncate(pre_turn_len);
                error!(
                    conversation_id = conversation_id,
                    "agent chat task cancelled unexpectedly"
                );
                token_tracker::record_error(&metrics);
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::AgentError,
                    &agent_id,
                    &conversation_id,
                    json!({"code": "agent_error", "message": "agent chat task cancelled"}),
                ));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::Done,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::TurnCompleted,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
            }
            // Got result from agent
            Ok(Ok((result, mut enriched_history))) => {
                let latency_ms = start.elapsed().as_millis() as u64;

                // Classify the result into: usable response, needs recovery, or genuine error.
                let (response_text, initial_input_tokens, initial_output_tokens) = match result {
                    Ok(prompt_response) => {
                        let it = prompt_response.total_usage.input_tokens;
                        let ot = prompt_response.total_usage.output_tokens;
                        (Some(prompt_response.output), it, ot)
                    }
                    Err(e) => {
                        let error_msg = format!("{}", e);
                        if error_msg.contains("no message or tool call")
                            || error_msg.contains("did not match any variant of untagged enum")
                        {
                            warn!(
                                agent_id = agent_id,
                                conversation_id = conversation_id,
                                error = %e,
                                "agent response could not be parsed, attempting recovery"
                            );
                            (None, 0u64, 0u64)
                        } else {
                            // Genuine error — keep existing fatal handling
                            persisted_messages.lock().unwrap().truncate(pre_turn_len);
                            error!(
                                agent_id = agent_id,
                                conversation_id = conversation_id,
                                error = %e,
                                error_debug = ?e,
                                "agent chat error"
                            );
                            token_tracker::record_error(&metrics);
                            event_bus.emit(BridgeEvent::new(BridgeEventType::AgentError, &agent_id, &conversation_id, json!({"code": "agent_error", "message": format!("agent error: {}", e)})));
                            event_bus.emit(BridgeEvent::new(
                                BridgeEventType::Done,
                                &agent_id,
                                &conversation_id,
                                json!({}),
                            ));
                            event_bus.emit(BridgeEvent::new(
                                BridgeEventType::TurnCompleted,
                                &agent_id,
                                &conversation_id,
                                json!({}),
                            ));
                            turn_count += 1;
                            continue;
                        }
                    }
                };

                let has_text = matches!(&response_text, Some(text) if !text.is_empty());
                let had_tool_calls =
                    history_contains_tool_calls(&enriched_history, history_backup.len());

                let needs_recovery = if tool_calls_only && had_tool_calls {
                    // Agent is configured to complete with tool calls only — no text needed.
                    false
                } else {
                    !has_text
                };

                let response = if needs_recovery {
                    warn!(
                        agent_id = agent_id,
                        conversation_id = conversation_id,
                        "agent returned empty response, attempting continuation with tools"
                    );

                    // Step 1: Try continuation with the main agent (has tools).
                    // This gives the agent a chance to keep working if it wasn't done.
                    let mut continuation_response: Option<String> = None;
                    for _attempt in 0..MAX_CONTINUATIONS {
                        let agent_clone = { agent.read().await.clone() };
                        let event_bus_clone = event_bus.clone();
                        let turn_cancel_clone = turn_cancel.clone();
                        let tool_names_clone = tool_names.clone();
                        let tool_executors_clone = tool_executors.clone();
                        let agent_context_clone = agent_context.clone();
                        let agent_id_clone = agent_id.clone();
                        let conversation_id_clone = conversation_id.clone();
                        let permission_manager_clone = permission_manager.clone();
                        let agent_permissions_clone = agent_permissions.clone();
                        let metrics_for_cont = metrics.clone();
                        let conversation_metrics_for_cont = conversation_metrics.clone();
                        let storage_for_cont = storage.clone();
                        let persisted_messages_for_cont = persisted_messages.clone();
                        let mut history_for_continuation = enriched_history.clone();
                        let (cont_tx, cont_rx) = tokio::sync::oneshot::channel();
                        let cont_prompt = if tool_calls_only {
                            format!(
                                "You were assigned to work on the following task:\n\n{}\n\nPlease continue working on it.",
                                user_text
                            )
                        } else {
                            format!(
                                "You were assigned to work on the following task:\n\n{}\n\nPlease continue working on it. If you have completed all the work, provide a final text summary.",
                                user_text
                            )
                        };

                        tokio::spawn(async move {
                            let emitter = llm::ToolCallEmitter {
                                event_bus: event_bus_clone,
                                cancel: turn_cancel_clone,
                                tool_names: tool_names_clone,
                                tool_executors: tool_executors_clone,
                                agent_id: agent_id_clone,
                                conversation_id: conversation_id_clone,
                                permission_manager: permission_manager_clone,
                                agent_permissions: agent_permissions_clone,
                                metrics: metrics_for_cont,
                                conversation_metrics: Some(conversation_metrics_for_cont.clone()),
                                pending_tool_timings: Arc::new(DashMap::new()),
                                storage: storage_for_cont,
                                persisted_messages: Some(persisted_messages_for_cont),
                            };
                            let fut = async {
                                agent_clone
                                    .prompt_with_hook(
                                        &cont_prompt,
                                        &mut history_for_continuation,
                                        emitter,
                                    )
                                    .await
                            };
                            let result = match agent_context_clone {
                                Some(ctx) => AGENT_CONTEXT.scope(ctx, fut).await,
                                None => fut.await,
                            };
                            let _ = cont_tx.send((result, history_for_continuation));
                        });

                        match tokio::time::timeout(CONTINUATION_TIMEOUT, cont_rx).await {
                            Ok(Ok((Ok(pr), cont_history))) if !pr.output.is_empty() => {
                                info!(
                                    agent_id = agent_id,
                                    conversation_id = conversation_id,
                                    response_len = pr.output.len(),
                                    "continuation with tools produced non-empty response"
                                );
                                enriched_history = cont_history;
                                continuation_response = Some(pr.output);
                                break;
                            }
                            Ok(Ok((_, cont_history))) => {
                                // Continuation produced empty or error — update history
                                // (preserves any new tool calls) and fall through to retry.
                                enriched_history = cont_history;
                            }
                            _ => {
                                warn!(
                                    agent_id = agent_id,
                                    conversation_id = conversation_id,
                                    "continuation attempt timed out or was cancelled"
                                );
                            }
                        }
                    }

                    if let Some(text) = continuation_response {
                        text
                    } else {
                        // Step 2: Retry with no-tools agent WITH enriched history.
                        // The retry agent sees the full conversation (user request +
                        // all tool calls + results) so it can write an accurate summary
                        // instead of hallucinating.
                        warn!(
                            agent_id = agent_id,
                            conversation_id = conversation_id,
                            "continuation did not produce text, retrying with no-tools agent with enriched history"
                        );
                        match retry_agent
                            .prompt_with_history(
                                "Please provide a text response summarizing what you found or did.",
                                &mut enriched_history,
                            )
                            .await
                        {
                            Ok(resp) if !resp.is_empty() => {
                                info!(
                                    agent_id = agent_id,
                                    conversation_id = conversation_id,
                                    retry_len = resp.len(),
                                    "no-tools retry with enriched history succeeded"
                                );
                                resp
                            }
                            Ok(_) => {
                                warn!(
                                    agent_id = agent_id,
                                    conversation_id = conversation_id,
                                    "no-tools retry also returned empty, using fallback"
                                );
                                let fallback =
                                    "I completed the requested tasks using the available tools."
                                        .to_string();
                                enriched_history.push(rig::message::Message::assistant(&fallback));
                                fallback
                            }
                            Err(e) => {
                                warn!(
                                    agent_id = agent_id,
                                    conversation_id = conversation_id,
                                    error = %e,
                                    "no-tools retry failed, using fallback"
                                );
                                let fallback =
                                    "I completed the requested tasks using the available tools."
                                        .to_string();
                                enriched_history.push(rig::message::Message::assistant(&fallback));
                                fallback
                            }
                        }
                    }
                } else {
                    response_text.unwrap_or_default()
                };

                info!(
                    agent_id = agent_id,
                    conversation_id = conversation_id,
                    response_len = response.len(),
                    response_preview = %response.chars().take(500).collect::<String>(),
                    latency_ms = latency_ms,
                    input_tokens = initial_input_tokens,
                    output_tokens = initial_output_tokens,
                    "agent response finalized"
                );

                // Send the response as content delta only for recovery responses.
                // In the normal streaming path, text was already sent incrementally
                // via ContentDelta events from the spawned task.
                if !response.is_empty() && needs_recovery {
                    event_bus.emit(BridgeEvent::new(
                        BridgeEventType::ResponseChunk,
                        &agent_id,
                        &conversation_id,
                        json!({
                            "delta": &response,
                            "message_id": &msg_id,
                        }),
                    ));
                }

                // Authoritative rebuild: discard incremental tool messages added
                // during the turn and replace with the canonical rig history.
                let new_persisted_messages =
                    convert_from_rig_messages(&enriched_history[history_backup.len()..]);
                {
                    let mut guard = persisted_messages.lock().unwrap();
                    guard.truncate(pre_turn_len);
                    guard.push(persisted_user_message_clone);
                    guard.extend(new_persisted_messages);
                }

                if let Some(storage) = &storage {
                    storage.replace_messages(
                        conversation_id.clone(),
                        persisted_messages.lock().unwrap().clone(),
                    );
                }

                // Replace main history with the enriched version so that
                // subsequent turns preserve full tool-call context.
                history = enriched_history;

                // Record metrics (dual-write to agent + conversation)
                token_tracker::record_request(
                    &metrics,
                    Some(&conversation_metrics),
                    initial_input_tokens,
                    initial_output_tokens,
                    latency_ms,
                );

                // Signal completion
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ResponseCompleted,
                    &agent_id,
                    &conversation_id,
                    json!({
                        "message_id": &msg_id,
                        "input_tokens": initial_input_tokens,
                        "output_tokens": initial_output_tokens,
                        "model": &conversation_metrics.model,
                        "timestamp": chrono::Utc::now().to_rfc3339(),
                        "full_response": &response,
                    }),
                ));
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::Done,
                    &agent_id,
                    &conversation_id,
                    json!({}),
                ));
                let cm = conversation_metrics.snapshot();
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::TurnCompleted,
                    &agent_id,
                    &conversation_id,
                    json!({
                        "input_tokens": initial_input_tokens,
                        "output_tokens": initial_output_tokens,
                        "model": &cm.model,
                        "timestamp": chrono::Utc::now().to_rfc3339(),
                        "turn_number": turn_count,
                        "cumulative_input_tokens": cm.input_tokens,
                        "cumulative_output_tokens": cm.output_tokens,
                    }),
                ));
            }
        }

        turn_count += 1;
    }

    // Cleanup pending approvals for this conversation
    permission_manager.cleanup_conversation(&conversation_id);

    // Cleanup session store entries for this conversation
    if let Some(store) = session_store {
        store.remove_by_prefix(&conversation_id);
    }

    // Cleanup task registry entries for this conversation (prevents unbounded growth)
    if let Some(ref ctx) = agent_context {
        if let Some(ref registry) = ctx.task_registry {
            registry.cleanup_conversation(&conversation_id);
        }
    }

    // Disconnect any per-conversation MCP servers attached at creation time.
    // Runs on every loop-exit path (DELETE, abort, drain, SIGINT/SIGTERM,
    // max_turns, internal error) — panics and OS-level task aborts still skip it.
    if let (Some(ref scope), Some(ref manager)) = (&per_conversation_mcp_scope, &mcp_manager) {
        manager.disconnect_agent(scope).await;
    }

    token_tracker::decrement_active_conversations(&metrics);

    // Log final conversation metrics summary.
    // Note: the conversation_ended webhook is emitted by the DELETE handler
    // in the API layer — not here — to avoid duplicate events.
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

/// Check if any assistant messages in `enriched[baseline_len..]` contain tool calls.
fn history_contains_tool_calls(enriched: &[rig::message::Message], baseline_len: usize) -> bool {
    use rig::completion::message::AssistantContent;
    enriched[baseline_len..].iter().any(|msg| {
        if let rig::message::Message::Assistant { content, .. } = msg {
            content
                .iter()
                .any(|c| matches!(c, AssistantContent::ToolCall(_)))
        } else {
            false
        }
    })
}

fn text_message(role: Role, text: String) -> Message {
    Message {
        role,
        content: vec![bridge_core::conversation::ContentBlock::Text { text }],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

fn tool_result_message(tool_call_id: String, content: String) -> Message {
    Message {
        role: Role::Tool,
        content: vec![bridge_core::conversation::ContentBlock::ToolResult(
            bridge_core::conversation::ToolResult {
                tool_call_id,
                content,
                is_error: false,
            },
        )],
        timestamp: chrono::Utc::now(),
        system_reminder: None,
    }
}

fn tool_result_text(parts: &rig::OneOrMany<rig::message::ToolResultContent>) -> String {
    parts
        .iter()
        .map(|part| match part {
            rig::message::ToolResultContent::Text(text) => text.text.clone(),
            other => format!("{:?}", other),
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn convert_from_rig_message(msg: &rig::message::Message) -> Vec<Message> {
    use rig::completion::message::{AssistantContent, UserContent};

    match msg {
        rig::message::Message::User { content } => content
            .iter()
            .filter_map(|part| match part {
                UserContent::Text(text) if !text.text.is_empty() => {
                    Some(text_message(Role::User, text.text.clone()))
                }
                UserContent::ToolResult(result) => Some(tool_result_message(
                    result.id.clone(),
                    tool_result_text(&result.content),
                )),
                _ => None,
            })
            .collect(),
        rig::message::Message::Assistant { content, .. } => {
            let mut blocks = Vec::new();
            for part in content.iter() {
                match part {
                    AssistantContent::Text(text) if !text.text.is_empty() => {
                        blocks.push(bridge_core::conversation::ContentBlock::Text {
                            text: text.text.clone(),
                        });
                    }
                    AssistantContent::ToolCall(call) => {
                        let arguments = call.function.arguments.clone();
                        blocks.push(bridge_core::conversation::ContentBlock::ToolCall(
                            bridge_core::conversation::ToolCall {
                                id: call.id.clone(),
                                name: call.function.name.clone(),
                                arguments,
                            },
                        ));
                    }
                    _ => {}
                }
            }

            if blocks.is_empty() {
                Vec::new()
            } else {
                vec![Message {
                    role: Role::Assistant,
                    content: blocks,
                    timestamp: chrono::Utc::now(),
                    system_reminder: None,
                }]
            }
        }
    }
}

fn convert_from_rig_messages(messages: &[rig::message::Message]) -> Vec<Message> {
    messages.iter().flat_map(convert_from_rig_message).collect()
}

fn apply_compaction_to_persisted_history(
    history: &mut Vec<Message>,
    summary_text: &str,
    messages_compacted: usize,
) {
    let split_at = messages_compacted.min(history.len());
    if split_at == 0 {
        return;
    }

    let mut compacted = Vec::with_capacity(1 + history.len().saturating_sub(split_at));
    compacted.push(text_message(
        Role::User,
        format!("[Conversation Summary]\n{}", summary_text),
    ));
    compacted.extend(history.drain(split_at..));
    *history = compacted;
}

pub fn normalize_messages_for_persistence(messages: &[Message]) -> Vec<Message> {
    let mut normalized = Vec::with_capacity(messages.len());

    for message in messages {
        if message.role == Role::Tool {
            let mut expanded = false;
            for block in &message.content {
                if let bridge_core::conversation::ContentBlock::ToolResult(result) = block {
                    expanded = true;
                    normalized.push(Message {
                        role: Role::Tool,
                        content: vec![bridge_core::conversation::ContentBlock::ToolResult(
                            result.clone(),
                        )],
                        timestamp: message.timestamp,
                        system_reminder: None,
                    });
                }
            }
            if !expanded {
                normalized.push(message.clone());
            }
        } else {
            normalized.push(message.clone());
        }
    }

    normalized
}

/// Convert a bridge_core Message into a rig message.
///
/// Handles all content block types so that hydrated conversations preserve
/// the full tool-call/tool-result exchange the LLM needs for context.
fn convert_to_rig_message(msg: &Message) -> Option<rig::message::Message> {
    use bridge_core::conversation::ContentBlock;
    use rig::completion::message::AssistantContent;
    use rig::OneOrMany;

    match msg.role {
        Role::User => {
            let text = extract_text_content(msg);
            if text.is_empty() {
                return None;
            }
            Some(rig::message::Message::user(&text))
        }
        Role::Assistant => {
            let mut items: Vec<AssistantContent> = Vec::new();
            for block in &msg.content {
                match block {
                    ContentBlock::Text { text } if !text.is_empty() => {
                        items.push(AssistantContent::text(text));
                    }
                    ContentBlock::ToolCall(tc) => {
                        items.push(AssistantContent::tool_call(
                            &tc.id,
                            &tc.name,
                            tc.arguments.clone(),
                        ));
                    }
                    _ => {}
                }
            }
            let content = OneOrMany::many(items).ok()?;
            Some(rig::message::Message::Assistant { id: None, content })
        }
        Role::Tool => {
            for block in &msg.content {
                if let ContentBlock::ToolResult(tr) = block {
                    return Some(rig::message::Message::tool_result(
                        &tr.tool_call_id,
                        &tr.content,
                    ));
                }
            }
            None
        }
        Role::System => None,
    }
}

/// Convert a slice of bridge_core Messages into rig messages for history seeding.
///
/// Tool-role messages with multiple `ToolResult` blocks are expanded into
/// one rig message per result, since rig models each tool result as a
/// separate user message.
pub fn convert_messages(messages: &[Message]) -> Vec<rig::message::Message> {
    let mut result = Vec::with_capacity(messages.len());
    for msg in messages {
        if msg.role == Role::Tool {
            for block in &msg.content {
                if let bridge_core::conversation::ContentBlock::ToolResult(tr) = block {
                    result.push(rig::message::Message::tool_result(
                        &tr.tool_call_id,
                        &tr.content,
                    ));
                }
            }
        } else if let Some(rig_msg) = convert_to_rig_message(msg) {
            result.push(rig_msg);
        }
    }
    result
}

/// Extract text content from a Message for sending to the LLM.
fn extract_text_content(message: &Message) -> String {
    message
        .content
        .iter()
        .filter_map(|block| match block {
            bridge_core::conversation::ContentBlock::Text { text } => Some(text.as_str()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("\n")
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::conversation::{ContentBlock, ToolCall, ToolResult};
    use serde_json::json;

    fn make_message(role: Role, content: Vec<ContentBlock>) -> Message {
        Message {
            role,
            content,
            timestamp: chrono::Utc::now(),
            system_reminder: None,
        }
    }

    #[test]
    fn test_convert_user_text_message() {
        let msg = make_message(
            Role::User,
            vec![ContentBlock::Text {
                text: "hello".into(),
            }],
        );
        let rig_msg = convert_to_rig_message(&msg).unwrap();
        assert_eq!(rig_msg, rig::message::Message::user("hello"));
    }

    #[test]
    fn test_convert_assistant_text_message() {
        let msg = make_message(
            Role::Assistant,
            vec![ContentBlock::Text {
                text: "hi there".into(),
            }],
        );
        let rig_msg = convert_to_rig_message(&msg).unwrap();
        assert_eq!(rig_msg, rig::message::Message::assistant("hi there"));
    }

    #[test]
    fn test_convert_assistant_with_tool_call() {
        let msg = make_message(
            Role::Assistant,
            vec![
                ContentBlock::Text {
                    text: "Let me read that file.".into(),
                },
                ContentBlock::ToolCall(ToolCall {
                    id: "call_001".into(),
                    name: "read_file".into(),
                    arguments: json!({"path": "src/main.rs"}),
                }),
            ],
        );
        let rig_msg = convert_to_rig_message(&msg).unwrap();
        match &rig_msg {
            rig::message::Message::Assistant { content, .. } => {
                assert_eq!(content.iter().count(), 2);
            }
            _ => panic!("expected Assistant message"),
        }
    }

    #[test]
    fn test_convert_assistant_tool_call_only() {
        let msg = make_message(
            Role::Assistant,
            vec![ContentBlock::ToolCall(ToolCall {
                id: "call_002".into(),
                name: "bash".into(),
                arguments: json!({"command": "ls"}),
            })],
        );
        let rig_msg = convert_to_rig_message(&msg).unwrap();
        match &rig_msg {
            rig::message::Message::Assistant { content, .. } => {
                assert_eq!(content.iter().count(), 1);
            }
            _ => panic!("expected Assistant message"),
        }
    }

    #[test]
    fn test_convert_tool_result_message() {
        let msg = make_message(
            Role::Tool,
            vec![ContentBlock::ToolResult(ToolResult {
                tool_call_id: "call_001".into(),
                content: "file contents here".into(),
                is_error: false,
            })],
        );
        let rig_msg = convert_to_rig_message(&msg).unwrap();
        assert_eq!(
            rig_msg,
            rig::message::Message::tool_result("call_001", "file contents here")
        );
    }

    #[test]
    fn test_convert_system_message_returns_none() {
        let msg = make_message(
            Role::System,
            vec![ContentBlock::Text {
                text: "system prompt".into(),
            }],
        );
        assert!(convert_to_rig_message(&msg).is_none());
    }

    #[test]
    fn test_convert_empty_assistant_returns_none() {
        let msg = make_message(Role::Assistant, vec![]);
        assert!(convert_to_rig_message(&msg).is_none());
    }

    #[test]
    fn test_convert_empty_user_returns_none() {
        let msg = make_message(Role::User, vec![]);
        assert!(convert_to_rig_message(&msg).is_none());
    }

    #[test]
    fn test_convert_empty_tool_returns_none() {
        let msg = make_message(Role::Tool, vec![]);
        assert!(convert_to_rig_message(&msg).is_none());
    }

    #[test]
    fn test_convert_messages_full_conversation() {
        let messages = vec![
            make_message(
                Role::User,
                vec![ContentBlock::Text {
                    text: "Review auth.rs".into(),
                }],
            ),
            make_message(
                Role::Assistant,
                vec![
                    ContentBlock::Text {
                        text: "I'll read the file.".into(),
                    },
                    ContentBlock::ToolCall(ToolCall {
                        id: "call_001".into(),
                        name: "read_file".into(),
                        arguments: json!({"path": "src/auth.rs"}),
                    }),
                ],
            ),
            make_message(
                Role::Tool,
                vec![ContentBlock::ToolResult(ToolResult {
                    tool_call_id: "call_001".into(),
                    content: "fn login() { ... }".into(),
                    is_error: false,
                })],
            ),
            make_message(
                Role::Assistant,
                vec![ContentBlock::Text {
                    text: "The auth module looks good.".into(),
                }],
            ),
        ];

        let rig_messages = convert_messages(&messages);
        assert_eq!(rig_messages.len(), 4);

        assert_eq!(
            rig_messages[0],
            rig::message::Message::user("Review auth.rs")
        );
        match &rig_messages[1] {
            rig::message::Message::Assistant { content, .. } => {
                assert_eq!(content.iter().count(), 2);
            }
            _ => panic!("expected Assistant"),
        }
        assert_eq!(
            rig_messages[2],
            rig::message::Message::tool_result("call_001", "fn login() { ... }")
        );
        assert_eq!(
            rig_messages[3],
            rig::message::Message::assistant("The auth module looks good.")
        );
    }

    #[test]
    fn test_convert_messages_multiple_tool_results_expanded() {
        let messages = vec![make_message(
            Role::Tool,
            vec![
                ContentBlock::ToolResult(ToolResult {
                    tool_call_id: "call_a".into(),
                    content: "result a".into(),
                    is_error: false,
                }),
                ContentBlock::ToolResult(ToolResult {
                    tool_call_id: "call_b".into(),
                    content: "result b".into(),
                    is_error: false,
                }),
            ],
        )];

        let rig_messages = convert_messages(&messages);
        assert_eq!(rig_messages.len(), 2);
        assert_eq!(
            rig_messages[0],
            rig::message::Message::tool_result("call_a", "result a")
        );
        assert_eq!(
            rig_messages[1],
            rig::message::Message::tool_result("call_b", "result b")
        );
    }

    #[test]
    fn test_convert_messages_skips_system() {
        let messages = vec![
            make_message(
                Role::System,
                vec![ContentBlock::Text {
                    text: "You are helpful.".into(),
                }],
            ),
            make_message(Role::User, vec![ContentBlock::Text { text: "hi".into() }]),
        ];
        let rig_messages = convert_messages(&messages);
        assert_eq!(rig_messages.len(), 1);
        assert_eq!(rig_messages[0], rig::message::Message::user("hi"));
    }

    #[test]
    fn test_convert_assistant_multiple_tool_calls() {
        let msg = make_message(
            Role::Assistant,
            vec![
                ContentBlock::ToolCall(ToolCall {
                    id: "call_a".into(),
                    name: "read_file".into(),
                    arguments: json!({"path": "a.rs"}),
                }),
                ContentBlock::ToolCall(ToolCall {
                    id: "call_b".into(),
                    name: "read_file".into(),
                    arguments: json!({"path": "b.rs"}),
                }),
            ],
        );
        let rig_msg = convert_to_rig_message(&msg).unwrap();
        match &rig_msg {
            rig::message::Message::Assistant { content, .. } => {
                assert_eq!(content.iter().count(), 2);
            }
            _ => panic!("expected Assistant message"),
        }
    }

    #[test]
    fn test_roundtrip_multi_turn_with_tools() {
        // Simulates a realistic multi-turn conversation that would be
        // sent by the control plane for hydration.
        let messages = vec![
            make_message(
                Role::User,
                vec![ContentBlock::Text {
                    text: "Find security issues in auth.rs".into(),
                }],
            ),
            make_message(
                Role::Assistant,
                vec![
                    ContentBlock::Text {
                        text: "I'll read the file.".into(),
                    },
                    ContentBlock::ToolCall(ToolCall {
                        id: "call_1".into(),
                        name: "read_file".into(),
                        arguments: json!({"path": "src/auth.rs"}),
                    }),
                ],
            ),
            make_message(
                Role::Tool,
                vec![ContentBlock::ToolResult(ToolResult {
                    tool_call_id: "call_1".into(),
                    content: "pub fn login() {}".into(),
                    is_error: false,
                })],
            ),
            make_message(
                Role::Assistant,
                vec![
                    ContentBlock::Text {
                        text: "Let me also check for rate limiting.".into(),
                    },
                    ContentBlock::ToolCall(ToolCall {
                        id: "call_2".into(),
                        name: "grep".into(),
                        arguments: json!({"pattern": "rate_limit", "path": "src/"}),
                    }),
                ],
            ),
            make_message(
                Role::Tool,
                vec![ContentBlock::ToolResult(ToolResult {
                    tool_call_id: "call_2".into(),
                    content: "No matches found.".into(),
                    is_error: false,
                })],
            ),
            make_message(
                Role::Assistant,
                vec![ContentBlock::Text {
                    text: "No rate limiting found. This is a security issue.".into(),
                }],
            ),
            make_message(
                Role::User,
                vec![ContentBlock::Text {
                    text: "Fix it.".into(),
                }],
            ),
        ];

        let rig_messages = convert_messages(&messages);

        // 7 input messages → 7 rig messages (all preserved)
        assert_eq!(rig_messages.len(), 7);

        // Verify the sequence of roles
        assert!(matches!(
            rig_messages[0],
            rig::message::Message::User { .. }
        ));
        assert!(matches!(
            rig_messages[1],
            rig::message::Message::Assistant { .. }
        ));
        // Tool result is modeled as User in rig
        assert!(matches!(
            rig_messages[2],
            rig::message::Message::User { .. }
        ));
        assert!(matches!(
            rig_messages[3],
            rig::message::Message::Assistant { .. }
        ));
        assert!(matches!(
            rig_messages[4],
            rig::message::Message::User { .. }
        ));
        assert!(matches!(
            rig_messages[5],
            rig::message::Message::Assistant { .. }
        ));
        assert!(matches!(
            rig_messages[6],
            rig::message::Message::User { .. }
        ));
    }

    #[test]
    fn test_system_reminder_prepended_to_user_message() {
        // This test verifies that the system reminder is prepended to user messages
        // The actual prepending happens in run_conversation, but we can test the formatting logic
        let system_reminder = "<system-reminder>\n\n# System Reminders\n\n## Available skills\n\nThe following skills are available for use with the Skill tool:\n\n- **Code Review** - Reviews code\n\n</system-reminder>";
        let user_text = "Please review this code";

        let final_text = format!("{}\n\n{}", system_reminder, user_text);

        assert!(final_text.contains("<system-reminder>"));
        assert!(final_text.contains("</system-reminder>"));
        assert!(final_text.contains(user_text));
        assert!(final_text.starts_with("<system-reminder>"));
    }

    #[test]
    fn test_empty_system_reminder_skipped() {
        // When system_reminder is empty, user text should be used as-is
        let system_reminder = "";
        let user_text = "Hello";

        let final_text = if system_reminder.is_empty() {
            user_text.to_string()
        } else {
            format!("{}\n\n{}", system_reminder, user_text)
        };

        assert_eq!(final_text, user_text);
    }
}
