use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::json;
use std::sync::Arc;
use tracing::{error, warn};
use webhooks::EventBus;

use super::turn_result::TurnResultCtx;
use crate::token_tracker;

/// Tuple of (response_text, input_tokens, cached_input_tokens, output_tokens).
/// `response_text == None` signals a recoverable empty response; `Err` signals
/// a fatal error that caused the turn to abort.
pub(super) type ClassifiedResponse = (Option<String>, u64, u64, u64);

/// Classify the spawned task's result into a usable response, a recoverable
/// empty response (`Ok((None, _, _, _))`), or a fatal error (`Err(())`).
///
/// On the fatal path this function also emits the standard `AgentError /
/// Done / TurnCompleted` event triple, truncates persisted messages, and
/// discards staged journal entries. The caller should restore history from
/// the backup it owns and continue the loop.
pub(super) async fn classify_turn_result(
    ctx: &TurnResultCtx<'_>,
    result: Result<llm::PromptResponse, rig::completion::PromptError>,
    pre_turn_len: usize,
) -> Result<ClassifiedResponse, ()> {
    match result {
        Ok(prompt_response) => {
            // rig's `Usage::input_tokens` is the TOTAL prompt token count
            // (cached + uncached). `metrics::AgentMetrics::input_tokens`
            // is documented as the non-cached portion only — subtract here
            // so the agent-level metric and the cache_hit_ratio it computes
            // are correct (otherwise cached tokens get double-counted).
            let total_in = prompt_response.total_usage.input_tokens;
            let ct = prompt_response.total_usage.cached_input_tokens;
            let it = total_in.saturating_sub(ct);
            let ot = prompt_response.total_usage.output_tokens;
            Ok((Some(prompt_response.output), it, ct, ot))
        }
        Err(e) => {
            let error_msg = format!("{}", e);
            if error_msg.contains("no message or tool call")
                || error_msg.contains("did not match any variant of untagged enum")
            {
                warn!(
                    agent_id = ctx.agent_id,
                    conversation_id = ctx.conversation_id,
                    error = %e,
                    "agent response could not be parsed, attempting recovery"
                );
                Ok((None, 0u64, 0u64, 0u64))
            } else {
                ctx.persisted_messages
                    .lock()
                    .unwrap()
                    .truncate(pre_turn_len);

                // Pull any partial usage that `attempt_into_result` smuggled
                // into the error message. This covers conversations that
                // completed N successful sub-calls inside rig's multi-turn
                // loop before the (N+1)th errored — without this, those N
                // calls' tokens wouldn't show up anywhere in bridge metrics.
                let (partial_in, partial_cached, partial_out, clean_msg) =
                    extract_partial_usage(&error_msg);
                if partial_in > 0 || partial_cached > 0 || partial_out > 0 {
                    let it = partial_in.saturating_sub(partial_cached);
                    token_tracker::record_request(
                        ctx.metrics,
                        Some(ctx.conversation_metrics),
                        it,
                        partial_cached,
                        partial_out,
                        0,
                    );
                }

                error!(
                    agent_id = ctx.agent_id,
                    conversation_id = ctx.conversation_id,
                    error = %clean_msg,
                    "agent chat error"
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
                    format!("agent error: {}", clean_msg),
                );
                Err(())
            }
        }
    }
}

/// Parse the `__bridge_partial_usage__{json} <real msg>` envelope that
/// `attempt_into_result` uses to smuggle accumulated per-HTTP-call usage
/// through rig's `PromptError` (which has no usage field). Returns the
/// extracted (input_tokens_total, cached_input_tokens, output_tokens) and
/// the underlying error message with the marker stripped. If no marker is
/// present, returns zeros and the original message unchanged.
fn extract_partial_usage(msg: &str) -> (u64, u64, u64, String) {
    const MARKER: &str = "__bridge_partial_usage__";
    let Some(start) = msg.find(MARKER) else {
        return (0, 0, 0, msg.to_string());
    };
    let after = &msg[start + MARKER.len()..];
    let Some(end) = after.find('}') else {
        return (0, 0, 0, msg.to_string());
    };
    let json_part = &after[..=end];
    #[derive(serde::Deserialize)]
    struct Partial {
        #[serde(rename = "in")]
        input: u64,
        cached: u64,
        out: u64,
    }
    let Ok(p) = serde_json::from_str::<Partial>(json_part) else {
        return (0, 0, 0, msg.to_string());
    };
    let rest = after[end + 1..].trim_start().to_string();
    let cleaned = if msg[..start].is_empty() {
        rest
    } else {
        format!("{}{}", &msg[..start], rest)
    };
    (p.input, p.cached, p.out, cleaned)
}

/// Emit the standard "failed turn" event triple (AgentError+Done+TurnCompleted).
pub(super) fn emit_failed_turn_events(
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
    err_code: &str,
    err_message: String,
) {
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::AgentError,
        agent_id,
        conversation_id,
        json!({"code": err_code, "message": err_message}),
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
