use bridge_core::conversation::Message;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::json;
use std::collections::HashMap;
use std::sync::Arc;
use storage::StorageHandle;
use tracing::{info, warn};
use webhooks::EventBus;

use super::convert::convert_from_rig_messages;

/// Snapshot the current todowrite list (if the `todowrite` or `todoread`
/// tool is registered) and format it as plain text suitable for embedding
/// in a chain-handoff message. Returns `None` when the tool isn't in the
/// registry or the list is empty.
async fn snapshot_todos_for_handoff(
    tool_executors: &HashMap<String, Arc<dyn tools::ToolExecutor>>,
) -> Option<String> {
    let state = tool_executors
        .get("todoread")
        .and_then(|t| {
            t.as_ref()
                .as_any()
                .downcast_ref::<tools::todo::TodoReadTool>()
                .map(|tool| tool.state().clone())
        })
        .or_else(|| {
            tool_executors.get("todowrite").and_then(|t| {
                t.as_ref()
                    .as_any()
                    .downcast_ref::<tools::todo::TodoWriteTool>()
                    .map(|tool| tool.state().clone())
            })
        })?;
    let todos = state.get().await;
    if todos.is_empty() {
        return None;
    }
    let mut out = String::new();
    for (i, t) in todos.iter().enumerate() {
        out.push_str(&format!(
            "{}. [{}] ({}) {}\n",
            i + 1,
            t.status,
            t.priority,
            t.content
        ));
    }
    Some(out)
}

/// Run either an immortal chain handoff or a compaction pass at the top of
/// a turn, depending on which (if any) is configured. Refreshes `history_fp`
/// when either path rewrites `history`.
#[allow(clippy::too_many_arguments)]
pub(super) async fn maybe_run_context_management(
    history: &mut Vec<rig::message::Message>,
    history_fp: &mut crate::history_guard::HistoryFingerprint,
    persisted_messages: &Arc<std::sync::Mutex<Vec<Message>>>,
    immortal_config: &Option<bridge_core::agent::ImmortalConfig>,
    immortal_state: &mut Option<crate::immortal::ImmortalState>,
    compaction_config: &Option<bridge_core::agent::CompactionConfig>,
    journal_state: &Option<Arc<tools::journal::JournalState>>,
    tool_executors: &HashMap<String, Arc<dyn tools::ToolExecutor>>,
    storage: &Option<StorageHandle>,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
) {
    if let (Some(ref immortal_cfg), Some(ref mut imm_state)) =
        (immortal_config, immortal_state.as_mut())
    {
        // Journal state is optional. When the agent opted out of journal
        // tools (`expose_journal_tools: false`) `journal_state` is None;
        // the immortal engine then carries the current todowrite list as
        // the cross-chain memory instead of journal entries.
        let js_opt: Option<&tools::journal::JournalState> =
            journal_state.as_ref().map(|a| a.as_ref());
        // Step 1: cheap probe — do we even need a handoff?
        if let Some(trigger) = crate::immortal::chain_needed(history, immortal_cfg) {
            // Snapshot the todos list so the handoff carries it across chains.
            let todos_snapshot = snapshot_todos_for_handoff(tool_executors).await;
            let pending_chain_index = imm_state.current_chain_index + 1;
            let chain_start_instant = std::time::Instant::now();
            let pre_chain_tokens = trigger.pre_chain_tokens;

            // Emit ChainStarted BEFORE the expensive checkpoint LLM call so
            // consumers can render progress during the ~30-75s extraction.
            event_bus.emit(BridgeEvent::new(
                BridgeEventType::ChainStarted,
                agent_id,
                conversation_id,
                json!({
                    "chain_index": pending_chain_index,
                    "reason": "token_budget_exceeded",
                    "token_count": pre_chain_tokens,
                    "budget": immortal_cfg.token_budget,
                    "verify_enabled": immortal_cfg.verify_checkpoint,
                }),
            ));

            // Step 2: run the extraction.
            match crate::immortal::execute_chain_handoff(
                history,
                immortal_cfg,
                imm_state,
                js_opt,
                todos_snapshot,
                trigger,
            )
            .await
            {
                Ok(result) => {
                    info!(
                        conversation_id = conversation_id,
                        chain_index = result.chain_index,
                        pre_tokens = result.pre_chain_tokens,
                        carry_forward = result.carry_forward_count,
                        carry_forward_tokens = result.carry_forward_tokens,
                        verified = result.verified,
                        "conversation chain handoff"
                    );

                    // Save checkpoint as a journal entry — skipped when the
                    // journal was disabled for this agent.
                    if let Some(js) = js_opt {
                        let checkpoint_entry = tools::journal::JournalEntry {
                            id: uuid::Uuid::new_v4().to_string(),
                            chain_index: imm_state.current_chain_index,
                            entry_type: "checkpoint".to_string(),
                            content: result.checkpoint_text.clone(),
                            category: None,
                            timestamp: chrono::Utc::now(),
                        };
                        js.append(checkpoint_entry).await;
                    }

                    // Reset in-memory history
                    *history = result.new_history;
                    // Immortal chain reset is an expected cache-bust
                    // event. Refresh the fingerprint so the next
                    // turn's append-only check compares against the
                    // reset baseline, not the pre-reset history.
                    *history_fp = crate::history_guard::HistoryFingerprint::take(history);

                    // Rebuild persisted messages from the new rig history
                    {
                        let mut guard = persisted_messages.lock().unwrap();
                        *guard = convert_from_rig_messages(history);
                    }

                    // Update immortal state + journal chain pointer
                    imm_state.current_chain_index = result.chain_index;
                    if let Some(js) = js_opt {
                        js.set_chain_index(result.chain_index);
                    }

                    // Persist: replace messages + save chain link
                    if let Some(storage) = storage {
                        storage.replace_messages(
                            conversation_id.to_string(),
                            persisted_messages.lock().unwrap().clone(),
                        );
                        storage.save_chain_link(
                            conversation_id.to_string(),
                            result.chain_index,
                            chrono::Utc::now(),
                            Some(result.pre_chain_tokens),
                            Some(result.checkpoint_text.clone()),
                        );
                    }

                    let journal_entry_count = match js_opt {
                        Some(js) => js.entries().await.len(),
                        None => 0,
                    };
                    event_bus.emit(BridgeEvent::new(
                        BridgeEventType::ChainCompleted,
                        agent_id,
                        conversation_id,
                        json!({
                            "chain_index": result.chain_index,
                            "journal_entry_count": journal_entry_count,
                            "carry_forward_messages": result.carry_forward_count,
                            "carry_forward_tokens": result.carry_forward_tokens,
                            "checkpoint_bytes": result.checkpoint_text.len(),
                            "verified": result.verified,
                            "duration_ms": chain_start_instant.elapsed().as_millis() as u64,
                        }),
                    ));
                }
                Err(e) => {
                    warn!(
                        error = %e,
                        chain_index = pending_chain_index,
                        "chain handoff failed, continuing with full history"
                    );
                    event_bus.emit(BridgeEvent::new(
                        BridgeEventType::ChainFailed,
                        agent_id,
                        conversation_id,
                        json!({
                            "chain_index": pending_chain_index,
                            "reason": format!("{}", e),
                            "token_count": pre_chain_tokens,
                            "duration_ms": chain_start_instant.elapsed().as_millis() as u64,
                        }),
                    ));
                }
            }
        }
    } else if let Some(ref compaction_config) = compaction_config {
        match crate::compaction::maybe_compact(history, compaction_config).await {
            Ok(Some(result)) => {
                info!(
                    conversation_id = conversation_id,
                    pre_tokens = result.pre_compaction_tokens,
                    post_tokens = result.post_compaction_tokens,
                    messages_compacted = result.messages_compacted,
                    "conversation compacted"
                );

                // Replace in-memory history
                *history = result.compacted_history;
                // Compaction rewrites head messages — an expected
                // cache-bust. Refresh the fingerprint baseline.
                *history_fp = crate::history_guard::HistoryFingerprint::take(history);

                {
                    let mut guard = persisted_messages.lock().unwrap();
                    super::convert::apply_compaction_to_persisted_history(
                        &mut guard,
                        &result.summary_text,
                        result.messages_compacted,
                    );
                }

                if let Some(storage) = storage {
                    storage.replace_messages(
                        conversation_id.to_string(),
                        persisted_messages.lock().unwrap().clone(),
                    );
                }

                // Fire compaction event
                event_bus.emit(BridgeEvent::new(
                    BridgeEventType::ConversationCompacted,
                    agent_id,
                    conversation_id,
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
}
