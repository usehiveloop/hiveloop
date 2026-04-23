use bridge_core::event::{BridgeEvent, BridgeEventType};
use futures::StreamExt;
use llm::{BridgeStreamItem, ToolCallEmitter};
use serde_json::json;
use std::sync::Arc;
use webhooks::EventBus;

use super::convert::is_retryable_stream_err;

/// Accumulator for a single attempt of the streaming LLM call.
pub(super) struct StreamAttempt {
    pub(super) accumulated_text: String,
    pub(super) final_usage: rig::completion::Usage,
    pub(super) final_history: Option<Vec<rig::message::Message>>,
    pub(super) had_error: Option<String>,
    pub(super) any_progress: bool,
}

/// Run the inner retry loop that stream-prompts the agent, emitting SSE
/// text/reasoning deltas as they arrive. Returns the final [`StreamAttempt`]
/// after a successful attempt or the allowed retry budget is exhausted.
#[allow(clippy::too_many_arguments)]
pub(super) async fn run_streaming_with_retry(
    agent_clone: &llm::BridgeAgent,
    user_text: &str,
    history_for_task: &[rig::message::Message],
    emitter: ToolCallEmitter,
    event_bus_for_text: &Arc<EventBus>,
    agent_id_for_text: &str,
    conversation_id_for_text: &str,
    msg_id_clone: &str,
) -> StreamAttempt {
    const MAX_STREAM_PREFLIGHT_RETRIES: usize = 3;
    let mut attempt_no: usize = 0;
    let mut out;
    loop {
        let history_for_attempt = history_for_task.to_vec();
        let emitter_for_attempt = emitter.clone();

        let mut stream = agent_clone
            .stream_prompt_with_hook(user_text, history_for_attempt, emitter_for_attempt)
            .await;

        out = StreamAttempt {
            accumulated_text: String::new(),
            final_usage: rig::completion::Usage::new(),
            final_history: None,
            had_error: None,
            any_progress: false,
        };

        while let Some(item) = stream.next().await {
            match item {
                BridgeStreamItem::TextDelta(delta) => {
                    out.any_progress = true;
                    out.accumulated_text.push_str(&delta);
                    event_bus_for_text.emit(BridgeEvent::new(
                        BridgeEventType::ResponseChunk,
                        agent_id_for_text,
                        conversation_id_for_text,
                        json!({
                            "delta": &delta,
                            "message_id": msg_id_clone,
                        }),
                    ));
                }
                BridgeStreamItem::ReasoningDelta(delta) => {
                    out.any_progress = true;
                    event_bus_for_text.emit(BridgeEvent::new(
                        BridgeEventType::ReasoningDelta,
                        agent_id_for_text,
                        conversation_id_for_text,
                        json!({
                            "delta": &delta,
                            "message_id": msg_id_clone,
                        }),
                    ));
                }
                BridgeStreamItem::StreamFinished {
                    response,
                    usage,
                    history,
                } => {
                    out.accumulated_text = response;
                    out.final_usage = usage;
                    out.final_history = history;
                }
                BridgeStreamItem::StreamError(err) => {
                    out.had_error = Some(err);
                    break;
                }
            }
        }

        // Decide whether to retry.
        let should_retry = match (&out.had_error, out.any_progress) {
            (Some(err_msg), false) if attempt_no < MAX_STREAM_PREFLIGHT_RETRIES => {
                is_retryable_stream_err(err_msg)
            }
            _ => false,
        };

        if !should_retry {
            break;
        }

        attempt_no += 1;
        // 1s → 2s → 4s exponential, capped at 30s
        let backoff_ms: u64 = std::cmp::min(
            1_000u64.saturating_mul(1u64 << (attempt_no - 1) as u32),
            30_000,
        );
        tracing::warn!(
            attempt = attempt_no,
            backoff_ms = backoff_ms,
            error = out.had_error.as_deref().unwrap_or(""),
            "pre-stream LLM error — retrying"
        );
        tokio::time::sleep(std::time::Duration::from_millis(backoff_ms)).await;
    }
    out
}

/// Convert a [`StreamAttempt`] into the final `(Result, history)` tuple
/// returned by the streaming wrapper. Recognises the parse-error recovery
/// pattern and flattens stream errors into `PromptError::ProviderError`.
pub(super) fn attempt_into_result(
    attempt: StreamAttempt,
) -> (
    Result<llm::PromptResponse, rig::completion::PromptError>,
    Vec<rig::message::Message>,
) {
    let enriched_history = attempt.final_history.unwrap_or_default();

    if let Some(err_msg) = attempt.had_error {
        // Check if it's a parse error that allows recovery
        if err_msg.contains("no message or tool call")
            || err_msg.contains("did not match any variant of untagged enum")
        {
            // Treat as recoverable: return accumulated text (may be empty)
            (
                Ok(llm::PromptResponse {
                    output: attempt.accumulated_text,
                    total_usage: attempt.final_usage,
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
                output: attempt.accumulated_text,
                total_usage: attempt.final_usage,
            }),
            enriched_history,
        )
    }
}
