use bridge_core::conversation::Message;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::json;
use std::collections::HashMap;
use std::sync::Arc;
use storage::StorageHandle;
use tracing::{info, warn};
use webhooks::EventBus;

use super::convert::convert_from_rig_messages;

/// Top-of-turn check. Runs `chain_needed` against the current history,
/// and if it triggers, runs one in-place compaction pass via
/// `run_chain_handoff`. Refreshes `history_fp` on success.
#[allow(clippy::too_many_arguments)]
pub(super) async fn maybe_run_context_management(
    history: &mut Vec<rig::message::Message>,
    history_fp: &mut crate::history_guard::HistoryFingerprint,
    persisted_messages: &Arc<std::sync::Mutex<Vec<Message>>>,
    immortal_config: &Option<bridge_core::agent::ImmortalConfig>,
    immortal_state: &mut Option<crate::immortal::ImmortalState>,
    _journal_state: &Option<Arc<tools::journal::JournalState>>,
    _tool_executors: &HashMap<String, Arc<dyn tools::ToolExecutor>>,
    storage: &Option<StorageHandle>,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
) {
    if let (Some(ref immortal_cfg), Some(_)) = (immortal_config, immortal_state.as_ref()) {
        if let Some(trigger) = crate::immortal::chain_needed(history, immortal_cfg) {
            run_chain_handoff(
                history,
                history_fp,
                persisted_messages,
                immortal_config,
                immortal_state,
                storage,
                event_bus,
                agent_id,
                conversation_id,
                trigger,
                "token_budget_exceeded",
            )
            .await;
        }
    }
}

/// Run one compaction pass and install the result. Used both by the
/// top-of-turn check and by the mid-rig-loop resume path in `run.rs`
/// (where `chain_needed` is bypassed because the hook has already
/// decided a handoff is needed and we don't want estimator disagreement
/// to deadlock the resume).
///
/// Forgecode-style: replaces the eligible head of `history` in place
/// with one summary user message, refreshes persisted state, emits the
/// `ChainStarted` / `ChainCompleted` event pair.
#[allow(clippy::too_many_arguments)]
pub(super) async fn run_chain_handoff(
    history: &mut Vec<rig::message::Message>,
    history_fp: &mut crate::history_guard::HistoryFingerprint,
    persisted_messages: &Arc<std::sync::Mutex<Vec<Message>>>,
    immortal_config: &Option<bridge_core::agent::ImmortalConfig>,
    immortal_state: &mut Option<crate::immortal::ImmortalState>,
    storage: &Option<StorageHandle>,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
    trigger: crate::immortal::ChainTrigger,
    trigger_reason: &str,
) {
    let (Some(ref immortal_cfg), Some(ref mut imm_state)) =
        (immortal_config, immortal_state.as_mut())
    else {
        return;
    };

    let pending_chain_index = imm_state.current_chain_index + 1;
    let chain_start_instant = std::time::Instant::now();

    event_bus.emit(BridgeEvent::new(
        BridgeEventType::ChainStarted,
        agent_id,
        conversation_id,
        json!({
            "chain_index": pending_chain_index,
            "reason": trigger_reason,
            "token_count": trigger.pre_chain_tokens,
            "budget": immortal_cfg.token_budget,
        }),
    ));

    match crate::immortal::execute_chain_handoff(
        history,
        immortal_cfg,
        imm_state,
        None,
        None,
        trigger,
    )
    .await
    {
        Ok(result) => {
            info!(
                conversation_id,
                chain_index = result.chain_index,
                pre_tokens = result.pre_chain_tokens,
                messages_compacted = result.messages_compacted,
                messages_after = result.messages_after,
                summary_bytes = result.summary_text.len(),
                "compaction handoff installed"
            );

            *history = result.new_history;
            *history_fp = crate::history_guard::HistoryFingerprint::take(history);

            {
                let mut guard = persisted_messages.lock().unwrap();
                *guard = convert_from_rig_messages(history);
            }
            imm_state.current_chain_index = result.chain_index;

            if let Some(storage) = storage {
                storage.replace_messages(
                    conversation_id.to_string(),
                    persisted_messages.lock().unwrap().clone(),
                );
            }

            event_bus.emit(BridgeEvent::new(
                BridgeEventType::ChainCompleted,
                agent_id,
                conversation_id,
                json!({
                    "chain_index": result.chain_index,
                    "messages_compacted": result.messages_compacted,
                    "messages_after": result.messages_after,
                    "summary_bytes": result.summary_text.len(),
                    "duration_ms": chain_start_instant.elapsed().as_millis() as u64,
                }),
            ));
        }
        Err(e) => {
            warn!(
                error = %e,
                chain_index = pending_chain_index,
                "compaction handoff failed; continuing with oversized history"
            );
            event_bus.emit(BridgeEvent::new(
                BridgeEventType::ChainFailed,
                agent_id,
                conversation_id,
                json!({
                    "chain_index": pending_chain_index,
                    "reason": format!("{}", e),
                    "token_count": trigger.pre_chain_tokens,
                    "duration_ms": chain_start_instant.elapsed().as_millis() as u64,
                }),
            ));
        }
    }
}
