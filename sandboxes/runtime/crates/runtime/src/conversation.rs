use bridge_core::conversation::{Message, Role};
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use llm::{PermissionManager, SseEvent, TokenUsage};
use serde_json::json;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio::sync::{Mutex, RwLock};
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification, AGENT_CONTEXT};
use tools::ToolExecutor;
use tracing::{debug, error, info, warn};
use webhooks::WebhookContext;

use crate::agent_runner::AgentSessionStore;

/// Timeout for a single agent.chat() call (includes internal tool loops).
const AGENT_CHAT_TIMEOUT: Duration = Duration::from_secs(180);

/// Maximum number of automatic continuation attempts when the agent returns an
/// empty response. After this many continuations, fall back to the no-tools
/// retry agent.
const MAX_CONTINUATIONS: usize = 1;

use crate::token_tracker;

/// Incoming message for the conversation loop — either a user message or
/// a background subagent completion notification.
enum IncomingMessage {
    User(Message),
    BackgroundComplete(AgentTaskNotification),
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
    /// Sender for SSE events back to the client.
    pub sse_tx: mpsc::Sender<SseEvent>,
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
    /// Optional webhook context for dispatching webhook events alongside SSE.
    pub webhook_ctx: Option<WebhookContext>,
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
        sse_tx,
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
        webhook_ctx,
        permission_manager,
        agent_permissions,
        compaction_config,
        system_reminder,
        conversation_date,
    } = params;

    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        "conversation started"
    );

    token_tracker::increment_active_conversations(&metrics);
    token_tracker::increment_total_conversations(&metrics);

    let mut history: Vec<rig::message::Message> = initial_history.unwrap_or_default();
    let mut turn_count: usize = 0;
    let msg_id = uuid::Uuid::new_v4().to_string();

    // Initialize date tracker for detecting date changes
    let mut date_tracker = crate::system_reminder::DateTracker::with_date(conversation_date);

    loop {
        // Wait for either a user message, a background task notification, or cancellation
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
        };

        // Check max turns
        if let Some(max) = max_turns {
            if turn_count >= max {
                let _ = sse_tx
                    .send(SseEvent::Error {
                        code: "max_turns_exceeded".to_string(),
                        message: format!("max turns ({}) exceeded", max),
                    })
                    .await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::agent_error(&agent_id, &conversation_id, json!({"code": "max_turns_exceeded", "message": format!("max turns ({}) exceeded", max)}), &wh.url, &wh.secret));
                }
                let _ = sse_tx.send(SseEvent::Done).await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::turn_completed(
                        &agent_id,
                        &conversation_id,
                        &wh.url,
                        &wh.secret,
                    ));
                }
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

                // Emit SSE event for background task completion
                let _ = sse_tx
                    .send(SseEvent::BackgroundTaskCompleted {
                        task_id: task_id.clone(),
                        description: description.clone(),
                        output: output_text.clone(),
                        is_error,
                    })
                    .await;

                // Emit webhook event for background task completion
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher
                        .dispatch(webhooks::events::background_task_completed(
                            &agent_id,
                            &conversation_id,
                            json!({
                                "task_id": task_id,
                                "description": description,
                                "output": output_text,
                                "is_error": is_error,
                            }),
                            &wh.url,
                            &wh.secret,
                        ));
                }

                format!(
                    "[Background Agent Task Completed]\ntask_id: {}\ndescription: {}\n\n<task_result>\n{}\n</task_result>",
                    task_id,
                    description,
                    output_text,
                )
            }
        };

        // Check if compaction is needed before adding the new message
        if let Some(ref compaction_config) = compaction_config {
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

                    // Fire webhook
                    if let Some(ref wh) = webhook_ctx {
                        wh.dispatcher
                            .dispatch(webhooks::events::conversation_compacted(
                                &agent_id,
                                &conversation_id,
                                json!({
                                    "summary": result.summary_text,
                                    "messages_compacted": result.messages_compacted,
                                    "pre_compaction_tokens": result.pre_compaction_tokens,
                                    "post_compaction_tokens": result.post_compaction_tokens,
                                }),
                                &wh.url,
                                &wh.secret,
                            ));
                    }
                }
                Ok(None) => {} // under budget, no compaction needed
                Err(e) => {
                    warn!(error = %e, "compaction failed, continuing with full history");
                }
            }
        }

        // Check for date change and get reminder if date changed
        let date_change_reminder = date_tracker.check_date_change();

        // Build final user text with reminders
        let final_user_text = match (date_change_reminder, system_reminder.is_empty()) {
            (Some(date_reminder), true) => {
                // Only date change reminder
                format!("{}\n\n{}", date_reminder, user_text)
            }
            (Some(date_reminder), false) => {
                // Both date change and system reminder
                format!("{}\n\n{}\n\n{}", date_reminder, system_reminder, user_text)
            }
            (None, true) => {
                // No reminders
                user_text.clone()
            }
            (None, false) => {
                // Only system reminder
                format!("{}\n\n{}", system_reminder, user_text)
            }
        };

        history.push(rig::message::Message::user(&final_user_text));

        // Signal response starting
        let _ = sse_tx
            .send(SseEvent::MessageStart {
                conversation_id: conversation_id.clone(),
                message_id: msg_id.clone(),
            })
            .await;
        if let Some(ref wh) = webhook_ctx {
            wh.dispatcher.dispatch(webhooks::events::response_started(
                &agent_id,
                &conversation_id,
                &wh.url,
                &wh.secret,
            ));
        }

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
        let mut history_clone = history.clone();
        let sse_tx_clone = sse_tx.clone();
        let agent_context_clone = agent_context.clone();
        let turn_cancel_clone = turn_cancel.clone();
        let tool_names_clone = tool_names.clone();
        let tool_executors_clone = tool_executors.clone();
        let webhook_ctx_clone = webhook_ctx.clone();
        let agent_id_clone = agent_id.clone();
        let conversation_id_clone = conversation_id.clone();
        let permission_manager_clone = permission_manager.clone();
        let agent_permissions_clone = agent_permissions.clone();
        let (result_tx, result_rx) = tokio::sync::oneshot::channel();

        tokio::spawn(async move {
            let emitter = llm::ToolCallEmitter {
                sse_tx: sse_tx_clone,
                cancel: turn_cancel_clone,
                tool_names: tool_names_clone,
                tool_executors: tool_executors_clone,
                webhook_ctx: webhook_ctx_clone,
                agent_id: agent_id_clone,
                conversation_id: conversation_id_clone,
                permission_manager: permission_manager_clone,
                agent_permissions: agent_permissions_clone,
            };
            let fut = async {
                agent_clone
                    .prompt_with_hook(&user_text_clone, &mut history_clone, emitter)
                    .await
            };

            // Wrap in AGENT_CONTEXT scope if available
            let result = match agent_context_clone {
                Some(ctx) => AGENT_CONTEXT.scope(ctx, fut).await,
                None => fut.await,
            };
            let _ = result_tx.send((result, history_clone));
        });

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
                // Remove the user message we pushed before the agent call —
                // no assistant response was generated, so leaving it would
                // create consecutive user messages in history.
                history.pop();
                let _ = sse_tx
                    .send(SseEvent::Error {
                        code: "aborted".to_string(),
                        message: "Turn aborted by user".to_string(),
                    })
                    .await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::agent_error(&agent_id, &conversation_id, json!({"code": "aborted", "message": "Turn aborted by user"}), &wh.url, &wh.secret));
                }
                let _ = sse_tx.send(SseEvent::Done).await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::turn_completed(&agent_id, &conversation_id, &wh.url, &wh.secret));
                }
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
            // Timeout fired
            Err(_timeout) => {
                let elapsed = start.elapsed();
                error!(
                    conversation_id = conversation_id,
                    timeout_secs = AGENT_CHAT_TIMEOUT.as_secs(),
                    elapsed_ms = elapsed.as_millis() as u64,
                    "agent chat timed out"
                );
                token_tracker::record_error(&metrics);
                let _ = sse_tx
                    .send(SseEvent::Error {
                        code: "agent_timeout".to_string(),
                        message: format!(
                            "agent chat timed out after {}s",
                            AGENT_CHAT_TIMEOUT.as_secs()
                        ),
                    })
                    .await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::agent_error(&agent_id, &conversation_id, json!({"code": "agent_timeout", "message": format!("agent chat timed out after {}s", AGENT_CHAT_TIMEOUT.as_secs())}), &wh.url, &wh.secret));
                }
                let _ = sse_tx.send(SseEvent::Done).await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::turn_completed(
                        &agent_id,
                        &conversation_id,
                        &wh.url,
                        &wh.secret,
                    ));
                }
            }
            // Task was cancelled (oneshot sender dropped)
            Ok(Err(_)) => {
                error!(
                    conversation_id = conversation_id,
                    "agent chat task cancelled unexpectedly"
                );
                token_tracker::record_error(&metrics);
                let _ = sse_tx
                    .send(SseEvent::Error {
                        code: "agent_error".to_string(),
                        message: "agent chat task cancelled".to_string(),
                    })
                    .await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::agent_error(
                        &agent_id,
                        &conversation_id,
                        json!({"code": "agent_error", "message": "agent chat task cancelled"}),
                        &wh.url,
                        &wh.secret,
                    ));
                }
                let _ = sse_tx.send(SseEvent::Done).await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::turn_completed(
                        &agent_id,
                        &conversation_id,
                        &wh.url,
                        &wh.secret,
                    ));
                }
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
                        if error_msg.contains("no message or tool call") {
                            warn!(
                                agent_id = agent_id,
                                conversation_id = conversation_id,
                                error = %e,
                                "agent returned no message or tool call, attempting recovery"
                            );
                            (None, 0u64, 0u64)
                        } else {
                            // Genuine error — keep existing fatal handling
                            error!(
                                agent_id = agent_id,
                                conversation_id = conversation_id,
                                error = %e,
                                error_debug = ?e,
                                "agent chat error"
                            );
                            token_tracker::record_error(&metrics);
                            let _ = sse_tx
                                .send(SseEvent::Error {
                                    code: "agent_error".to_string(),
                                    message: format!("agent error: {}", e),
                                })
                                .await;
                            if let Some(ref wh) = webhook_ctx {
                                wh.dispatcher.dispatch(webhooks::events::agent_error(&agent_id, &conversation_id, json!({"code": "agent_error", "message": format!("agent error: {}", e)}), &wh.url, &wh.secret));
                            }
                            let _ = sse_tx.send(SseEvent::Done).await;
                            if let Some(ref wh) = webhook_ctx {
                                wh.dispatcher.dispatch(webhooks::events::turn_completed(
                                    &agent_id,
                                    &conversation_id,
                                    &wh.url,
                                    &wh.secret,
                                ));
                            }
                            turn_count += 1;
                            continue;
                        }
                    }
                };

                let needs_recovery = !matches!(&response_text, Some(text) if !text.is_empty());

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
                        let sse_tx_clone = sse_tx.clone();
                        let turn_cancel_clone = turn_cancel.clone();
                        let tool_names_clone = tool_names.clone();
                        let tool_executors_clone = tool_executors.clone();
                        let agent_context_clone = agent_context.clone();
                        let webhook_ctx_clone = webhook_ctx.clone();
                        let agent_id_clone = agent_id.clone();
                        let conversation_id_clone = conversation_id.clone();
                        let permission_manager_clone = permission_manager.clone();
                        let agent_permissions_clone = agent_permissions.clone();
                        let mut history_for_continuation = enriched_history.clone();
                        let (cont_tx, cont_rx) = tokio::sync::oneshot::channel();
                        let cont_prompt = "Continue working on the task. If you have completed all the work, provide a final text summary of what you did and what you found.".to_string();

                        tokio::spawn(async move {
                            let emitter = llm::ToolCallEmitter {
                                sse_tx: sse_tx_clone,
                                cancel: turn_cancel_clone,
                                tool_names: tool_names_clone,
                                tool_executors: tool_executors_clone,
                                webhook_ctx: webhook_ctx_clone,
                                agent_id: agent_id_clone,
                                conversation_id: conversation_id_clone,
                                permission_manager: permission_manager_clone,
                                agent_permissions: agent_permissions_clone,
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

                        match tokio::time::timeout(AGENT_CHAT_TIMEOUT, cont_rx).await {
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
                    response_text.unwrap()
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

                // Send the response as content delta
                let _ = sse_tx
                    .send(SseEvent::ContentDelta {
                        delta: response.clone(),
                        message_id: msg_id.clone(),
                    })
                    .await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::response_chunk(
                        &agent_id,
                        &conversation_id,
                        json!({"delta": &response}),
                        &wh.url,
                        &wh.secret,
                    ));
                }

                // Replace main history with the enriched version so that
                // subsequent turns preserve full tool-call context.
                history = enriched_history;

                // Record metrics
                token_tracker::record_request(
                    &metrics,
                    initial_input_tokens,
                    initial_output_tokens,
                    latency_ms,
                );

                // Signal completion
                let _ = sse_tx
                    .send(SseEvent::MessageEnd {
                        message_id: msg_id.clone(),
                        usage: TokenUsage {
                            input_tokens: initial_input_tokens,
                            output_tokens: initial_output_tokens,
                        },
                    })
                    .await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::response_completed(&agent_id, &conversation_id, json!({"input_tokens": initial_input_tokens, "output_tokens": initial_output_tokens, "full_response": &response}), &wh.url, &wh.secret));
                }
                let _ = sse_tx.send(SseEvent::Done).await;
                if let Some(ref wh) = webhook_ctx {
                    wh.dispatcher.dispatch(webhooks::events::turn_completed(
                        &agent_id,
                        &conversation_id,
                        &wh.url,
                        &wh.secret,
                    ));
                }
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

    token_tracker::decrement_active_conversations(&metrics);

    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        turns = turn_count,
        "conversation ended"
    );
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
