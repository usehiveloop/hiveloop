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
            let it = prompt_response.total_usage.input_tokens;
            let ct = prompt_response.total_usage.cached_input_tokens;
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
                error!(
                    agent_id = ctx.agent_id,
                    conversation_id = ctx.conversation_id,
                    error = %e,
                    error_debug = ?e,
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
                    format!("agent error: {}", e),
                );
                Err(())
            }
        }
    }
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
