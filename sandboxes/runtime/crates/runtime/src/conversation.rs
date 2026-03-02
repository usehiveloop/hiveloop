use bridge_core::conversation::Message;
use bridge_core::AgentMetrics;
use llm::{SseEvent, TokenUsage};
use rig::completion::Prompt;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tracing::{debug, error, info};

/// Timeout for a single agent.chat() call (includes internal tool loops).
const AGENT_CHAT_TIMEOUT: Duration = Duration::from_secs(120);

use crate::token_tracker;

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
    } = params;

    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        "conversation started"
    );

    token_tracker::increment_active_conversations(&metrics);
    token_tracker::increment_total_conversations(&metrics);

    let mut history: Vec<rig::message::Message> = Vec::new();
    let mut turn_count: usize = 0;
    let msg_id = uuid::Uuid::new_v4().to_string();

    loop {
        let message = tokio::select! {
            _ = cancel.cancelled() => {
                debug!(conversation_id = conversation_id, "conversation cancelled");
                break;
            }
            msg = message_rx.recv() => {
                match msg {
                    Some(m) => m,
                    None => {
                        debug!(conversation_id = conversation_id, "message channel closed");
                        break;
                    }
                }
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
                let _ = sse_tx.send(SseEvent::Done).await;
                break;
            }
        }

        // Convert our Message to a rig user message and add to history
        let user_text = extract_text_content(&message);
        history.push(rig::message::Message::user(&user_text));

        // Signal response starting
        let _ = sse_tx
            .send(SseEvent::MessageStart {
                conversation_id: conversation_id.clone(),
                message_id: msg_id.clone(),
            })
            .await;

        let start = std::time::Instant::now();

        // Spawn the agent prompt in a separate task so that tokio::time::timeout
        // is guaranteed to fire even if the future blocks a worker thread.
        // Using prompt().with_hook() instead of chat() so tool calls emit SSE events.
        let agent_clone = agent.clone();
        let user_text_clone = user_text.clone();
        let mut history_clone = history.clone();
        let sse_tx_clone = sse_tx.clone();
        let (result_tx, result_rx) = tokio::sync::oneshot::channel();

        tokio::spawn(async move {
            let emitter = llm::ToolCallEmitter { sse_tx: sse_tx_clone };
            let result = agent_clone
                .prompt(&user_text_clone)
                .with_history(&mut history_clone)
                .with_hook(emitter)
                .await;
            let _ = result_tx.send(result);
        });

        // Wait for the result with a timeout
        let chat_result = tokio::time::timeout(AGENT_CHAT_TIMEOUT, result_rx).await;

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
                let _ = sse_tx.send(SseEvent::Done).await;
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
                let _ = sse_tx.send(SseEvent::Done).await;
            }
            // Got result from agent
            Ok(Ok(result)) => match result {
                Ok(response) => {
                    let latency_ms = start.elapsed().as_millis() as u64;

                    // Send the response as content delta
                    let _ = sse_tx
                        .send(SseEvent::ContentDelta {
                            delta: response.clone(),
                            message_id: msg_id.clone(),
                        })
                        .await;

                    // Add assistant response to history
                    history.push(rig::message::Message::assistant(&response));

                    // Record metrics
                    token_tracker::record_request(&metrics, 0, 0, latency_ms);

                    // Signal completion
                    let _ = sse_tx
                        .send(SseEvent::MessageEnd {
                            message_id: msg_id.clone(),
                            usage: TokenUsage {
                                input_tokens: 0,
                                output_tokens: 0,
                            },
                        })
                        .await;
                    let _ = sse_tx.send(SseEvent::Done).await;
                }
                Err(e) => {
                    error!(
                        conversation_id = conversation_id,
                        error = %e,
                        "agent chat error"
                    );
                    token_tracker::record_error(&metrics);
                    let _ = sse_tx
                        .send(SseEvent::Error {
                            code: "agent_error".to_string(),
                            message: format!("agent error: {}", e),
                        })
                        .await;
                    let _ = sse_tx.send(SseEvent::Done).await;
                }
            },
        }

        turn_count += 1;
    }

    token_tracker::decrement_active_conversations(&metrics);

    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        turns = turn_count,
        "conversation ended"
    );
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
