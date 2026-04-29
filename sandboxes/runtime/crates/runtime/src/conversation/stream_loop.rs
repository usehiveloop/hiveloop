use bridge_core::event::{BridgeEvent, BridgeEventType};
use futures::StreamExt;
use llm::{BridgeStreamItem, ToolCallEmitter};
use serde_json::json;
use std::sync::Arc;
use std::time::Duration;
use webhooks::EventBus;

use super::convert::is_retryable_stream_err;

/// Maximum gap between SSE chunks before we declare the upstream stalled
/// and abort the stream. The retry decision below treats a stall the same
/// as any retryable provider error — we bail out, the outer loop backs off,
/// and we send the same prompt again on a fresh connection.
///
/// 5 minutes is generous on purpose: heavy reasoning models routed through
/// busy aggregators (OpenRouter, etc.) can legitimately go silent for
/// several minutes mid-thought before the next chunk arrives. We'd rather
/// tolerate a long pause than abort a real call. A connection that's silent
/// for a full 5 minutes is wedged, not slow.
const STREAM_CHUNK_TIMEOUT: Duration = Duration::from_secs(300);

/// Tighter cap for the **first** chunk: when the upstream silently drops a
/// request (free-tier queues, rate-limit-without-status, etc.), we'd rather
/// know in 60s than 300s. Once the model has emitted at least one chunk the
/// connection is alive — drop back to `STREAM_CHUNK_TIMEOUT` for subsequent
/// chunks so a model that's reasoning between deltas isn't aborted unfairly.
///
/// Tunable via `BRIDGE_FIRST_CHUNK_TIMEOUT_SECS`. Set very high (e.g. 300)
/// to disable and revert to old behavior.
const FIRST_CHUNK_TIMEOUT_DEFAULT: Duration = Duration::from_secs(60);

fn first_chunk_timeout() -> Duration {
    std::env::var("BRIDGE_FIRST_CHUNK_TIMEOUT_SECS")
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .map(Duration::from_secs)
        .unwrap_or(FIRST_CHUNK_TIMEOUT_DEFAULT)
}

/// Accumulator for a single attempt of the streaming LLM call.
pub(super) struct StreamAttempt {
    pub(super) accumulated_text: String,
    pub(super) final_usage: rig::completion::Usage,
    pub(super) final_history: Option<Vec<rig::message::Message>>,
    pub(super) had_error: Option<String>,
    pub(super) any_progress: bool,
    /// Set when the agent loop was terminated by a `PromptHook` returning
    /// `HookAction::Terminate` (e.g. mid-rig-loop immortal trigger). The
    /// caller is expected to inspect the reason, run the corresponding
    /// recovery (chain handoff, etc.), and re-invoke the streaming call
    /// with the post-recovery history.
    pub(super) hook_cancellation: Option<(Vec<rig::message::Message>, String)>,
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
            hook_cancellation: None,
        };

        let first_timeout = first_chunk_timeout();
        loop {
            // Use the tighter `first_chunk` timeout until any progress
            // has been observed; once the upstream has emitted at least
            // one chunk, the connection is alive — fall back to the
            // longer chunk-gap timeout for the rest of the stream.
            let timeout = if out.any_progress {
                STREAM_CHUNK_TIMEOUT
            } else {
                first_timeout
            };
            let item = match tokio::time::timeout(timeout, stream.next()).await {
                Ok(Some(item)) => item,
                Ok(None) => break,
                Err(_elapsed) => {
                    out.had_error = Some(format!(
                        "stream stalled: no chunk received for {}s (timed out)",
                        timeout.as_secs()
                    ));
                    break;
                }
            };
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
                BridgeStreamItem::IntermediateUsage(usage) => {
                    // Per-HTTP-call usage from rig's multi-turn loop. Note:
                    // rig only yields these for sub-calls that emitted visible
                    // text (`saw_text_this_turn` is true) — tool-only sub-calls
                    // are accumulated into rig's internal `aggregated_usage`
                    // but never surfaced as `Final` items. So this counter is
                    // a *lower bound* on real usage: useful on the error path
                    // (where `StreamFinished` never fires) but always
                    // overwritten by the authoritative aggregated value when
                    // the multi-turn completes successfully.
                    out.final_usage += usage;
                }
                BridgeStreamItem::StreamFinished {
                    response,
                    usage,
                    history,
                } => {
                    out.accumulated_text = response;
                    // Authoritative on success: rig's `aggregated_usage`
                    // includes tool-only sub-call usage that the per-call
                    // `IntermediateUsage` events miss (see comment above).
                    out.final_usage = usage;
                    out.final_history = history;
                }
                BridgeStreamItem::StreamError(err) => {
                    out.had_error = Some(err);
                    break;
                }
                BridgeStreamItem::HookCancelled { history, reason } => {
                    // Hook-driven cancellation. Capture history + reason so
                    // the caller can run the corresponding recovery (e.g.
                    // mid-rig-loop immortal handoff) and re-invoke streaming
                    // with the post-recovery history. Treated as success
                    // from the retry policy's perspective.
                    out.hook_cancellation = Some((history, reason));
                    break;
                }
            }
        }

        // Decide whether to retry. Two paths:
        // 1. Pre-progress retryable error (HTTP 429/5xx, connect reset before
        //    any chunk arrived): retry the call as-is.
        // 2. Mid-stream stall (our 60s no-chunk guard fired even though some
        //    chunks already arrived): retry too. The upstream connection is
        //    wedged and the partial state is unusable — `final_history` is
        //    `None` because no `StreamFinished` arrived. Re-prompting with the
        //    *original* history is the right move; any tool calls that already
        //    executed during the failed attempt will get re-issued by the
        //    model on the retry, which is acceptable for the idempotent reads
        //    that dominate a normal turn (Read, LS, todowrite). Risk for
        //    non-idempotent writes (bash side-effects) is real but bounded —
        //    we'd rather re-do a step than abandon the conversation.
        let stream_stalled = out
            .had_error
            .as_deref()
            .is_some_and(|m| m.starts_with("stream stalled"));
        let should_retry = if out.hook_cancellation.is_some() {
            // Hook-driven cancellation is not a transport error; the caller
            // owns the recovery + resume.
            false
        } else if attempt_no >= MAX_STREAM_PREFLIGHT_RETRIES {
            false
        } else if stream_stalled {
            true
        } else {
            match (&out.had_error, out.any_progress) {
                (Some(err_msg), false) => is_retryable_stream_err(err_msg),
                _ => false,
            }
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
///
/// Hook-driven cancellations (immortal mid-rig-loop trigger) are surfaced
/// as `PromptError::PromptCancelled` carrying the cancellation history and
/// the hook's reason marker, so the outer loop can run the appropriate
/// recovery (chain handoff) and re-invoke streaming with the new history.
///
/// `history_for_task` and `user_text` are the inputs that were sent to rig.
/// When the stream terminates without a `StreamFinished` event (stall, parse
/// error, transient transport error), `attempt.final_history` is `None` —
/// we synthesize a fallback history (`history_for_task + user message`) so
/// the empty-response recovery path receives the full conversation context
/// instead of an empty Vec.
pub(super) fn attempt_into_result(
    attempt: StreamAttempt,
    history_for_task: &[rig::message::Message],
    user_text: &str,
) -> (
    Result<llm::PromptResponse, rig::completion::PromptError>,
    Vec<rig::message::Message>,
) {
    if let Some((history, reason)) = attempt.hook_cancellation {
        return (
            Err(rig::completion::PromptError::PromptCancelled {
                chat_history: history.clone(),
                reason,
            }),
            history,
        );
    }
    let enriched_history = attempt.final_history.unwrap_or_else(|| {
        let mut h = history_for_task.to_vec();
        h.push(rig::message::Message::user(user_text));
        h
    });

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
            // Fatal error path: stash any partial usage we accumulated from
            // successful sub-calls inside rig's multi-turn loop. The wrapper
            // error type here is a workaround — rig's `PromptError` doesn't
            // carry usage, but bridge's error classifier (in `turn_classify`)
            // looks for the `__bridge_partial_usage__` prefix and extracts
            // the JSON-encoded usage so the metrics path can record it.
            let usage = &attempt.final_usage;
            let wrapped = if usage.input_tokens > 0
                || usage.cached_input_tokens > 0
                || usage.output_tokens > 0
            {
                format!(
                    "__bridge_partial_usage__{{\"in\":{},\"cached\":{},\"out\":{}}} {}",
                    usage.input_tokens, usage.cached_input_tokens, usage.output_tokens, err_msg,
                )
            } else {
                err_msg
            };
            (
                Err(rig::completion::PromptError::CompletionError(
                    rig::completion::CompletionError::ProviderError(wrapped),
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
