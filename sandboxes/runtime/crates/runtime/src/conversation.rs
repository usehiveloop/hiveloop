use bridge_core::conversation::{Message, Role};
use bridge_core::AgentMetrics;
use llm::{SseEvent, TokenUsage};
use rig::completion::Prompt;
use serde_json::json;
use std::collections::{HashMap, HashSet};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::sync::mpsc;
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
    /// The built rig-core agent.
    pub agent: Arc<llm::BridgeAgent>,
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
        let user_text = match incoming {
            IncomingMessage::User(ref msg) => extract_text_content(msg),
            IncomingMessage::BackgroundComplete(notif) => {
                let output_text = match notif.output {
                    Ok(output) => output,
                    Err(error) => format!("[ERROR] {}", error),
                };
                format!(
                    "[Background Agent Task Completed]\ntask_id: {}\ndescription: {}\n\n<task_result>\n{}\n</task_result>",
                    notif.task_id,
                    notif.description,
                    output_text,
                )
            }
        };

        history.push(rig::message::Message::user(&user_text));

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
            let mut guard = abort_token.lock().unwrap();
            *guard = turn_cancel.clone();
        }

        // Spawn the agent prompt in a separate task so that tokio::time::timeout
        // is guaranteed to fire even if the future blocks a worker thread.
        // Using prompt().with_hook() instead of chat() so tool calls emit SSE events.
        let agent_clone = agent.clone();
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
            };
            let fut = async {
                agent_clone
                    .prompt(&user_text_clone)
                    .extended_details()
                    .with_history(&mut history_clone)
                    .with_hook(emitter)
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
            // Normal completion or timeout
            result = tokio::time::timeout(AGENT_CHAT_TIMEOUT, result_rx) => {
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
                        let agent_clone = agent.clone();
                        let sse_tx_clone = sse_tx.clone();
                        let turn_cancel_clone = turn_cancel.clone();
                        let tool_names_clone = tool_names.clone();
                        let tool_executors_clone = tool_executors.clone();
                        let agent_context_clone = agent_context.clone();
                        let webhook_ctx_clone = webhook_ctx.clone();
                        let agent_id_clone = agent_id.clone();
                        let conversation_id_clone = conversation_id.clone();
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
                            };
                            let fut = async {
                                agent_clone
                                    .prompt(&cont_prompt)
                                    .extended_details()
                                    .with_history(&mut history_for_continuation)
                                    .with_hook(emitter)
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
                            .prompt(
                                "Please provide a text response summarizing what you found or did.",
                            )
                            .with_history(&mut enriched_history)
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
                    wh.dispatcher.dispatch(webhooks::events::response_completed(&agent_id, &conversation_id, json!({"input_tokens": initial_input_tokens, "output_tokens": initial_output_tokens}), &wh.url, &wh.secret));
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
/// Returns `None` for roles that don't map directly (e.g. System, Tool).
fn convert_to_rig_message(msg: &Message) -> Option<rig::message::Message> {
    let text = msg
        .content
        .iter()
        .filter_map(|block| match block {
            bridge_core::conversation::ContentBlock::Text { text } => Some(text.as_str()),
            _ => None,
        })
        .collect::<Vec<_>>()
        .join("\n");

    if text.is_empty() {
        return None;
    }

    match msg.role {
        Role::User => Some(rig::message::Message::user(&text)),
        Role::Assistant => Some(rig::message::Message::assistant(&text)),
        _ => None,
    }
}

/// Convert a slice of bridge_core Messages into rig messages for history seeding.
pub fn convert_messages(messages: &[Message]) -> Vec<rig::message::Message> {
    messages.iter().filter_map(convert_to_rig_message).collect()
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
