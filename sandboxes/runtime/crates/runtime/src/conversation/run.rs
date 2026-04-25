use tracing::info;

use super::context_mgmt::{maybe_run_context_management, run_chain_handoff};

/// Wrap-around marker the immortal hook (`hook_impl::on_completion_call`)
/// uses when it returns `HookAction::Terminate` from inside rig's loop.
/// `run.rs` looks for this exact reason on a `PromptCancelled` to decide
/// whether to run a chain handoff and re-spawn streaming, vs. surface the
/// cancellation to the regular dispatch path.
const IMMORTAL_CANCEL_REASON: &str = "bridge:immortal";

#[allow(clippy::type_complexity)]
fn is_immortal_cancellation(
    chat_result: &Result<
        Result<
            (
                Result<llm::PromptResponse, rig::completion::PromptError>,
                Vec<rig::message::Message>,
            ),
            tokio::sync::oneshot::error::RecvError,
        >,
        tokio::time::error::Elapsed,
    >,
) -> bool {
    matches!(
        chat_result,
        Ok(Ok((Err(rig::completion::PromptError::PromptCancelled { reason, .. }), _)))
            if reason == IMMORTAL_CANCEL_REASON
    )
}

#[allow(clippy::type_complexity)]
fn take_cancellation_history(
    chat_result: &Result<
        Result<
            (
                Result<llm::PromptResponse, rig::completion::PromptError>,
                Vec<rig::message::Message>,
            ),
            tokio::sync::oneshot::error::RecvError,
        >,
        tokio::time::error::Elapsed,
    >,
) -> Option<Vec<rig::message::Message>> {
    match chat_result {
        Ok(Ok((Err(rig::completion::PromptError::PromptCancelled { chat_history, .. }), _))) => {
            Some(chat_history.clone())
        }
        _ => None,
    }
}
use super::init::LoopState;
use super::params::ConversationParams;
use super::receive::{
    build_persisted_user_message, build_user_text_with_pending, commit_user_turn, receive_incoming,
    ReceiveOutcome,
};
use super::streaming::{build_stream_inputs, prepare_turn, spawn_streaming_task};
use super::turn_result::{build_turn_result_ctx, dispatch_chat_result};
use super::turn_wait::{
    emit_max_turns_events, run_conversation_cleanup, wait_and_dispatch, WaitDisposition,
};
use crate::token_tracker;

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
        history_strip_config,
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
        system_reminder_refresh_turns,
        ping_state,
        tool_requirements,
    } = params;

    info!(
        agent_id = agent_id,
        conversation_id = conversation_id,
        "conversation started"
    );

    token_tracker::increment_active_conversations(&metrics);
    token_tracker::increment_total_conversations(&metrics);

    // One shared repeat-call guard for the whole conversation. Handed to
    // every per-turn ToolCallEmitter so identical consecutive calls are
    // detected across turns, not just within one turn.
    let repeat_guard = std::sync::Arc::new(std::sync::Mutex::new(llm::RepeatGuardState::default()));

    let history_strip_config = history_strip_config.unwrap_or_default();
    let LoopState {
        mut history,
        persisted_messages,
        mut turn_count,
        mut history_fp,
        msg_id,
        mut date_tracker,
        mut immortal_state,
        mut enforcement_state,
        mut pending_tool_reminder,
    } = LoopState::new(
        initial_history,
        initial_persisted_messages,
        conversation_date,
        &immortal_config,
        &journal_state,
        &tool_requirements,
    );

    loop {
        let incoming = match receive_incoming(
            &cancel,
            &conversation_id,
            &mut message_rx,
            &mut notification_rx,
            &ping_state,
        )
        .await
        {
            ReceiveOutcome::Got(m) => m,
            ReceiveOutcome::Break => break,
        };

        if let Some(max) = max_turns {
            if turn_count >= max {
                emit_max_turns_events(&event_bus, &agent_id, &conversation_id, max);
                break;
            }
        }

        let user_text = build_user_text_with_pending(
            &incoming,
            pending_tool_reminder.take(),
            &event_bus,
            &agent_id,
            &conversation_id,
        );

        let persisted_user_message = build_persisted_user_message(&incoming, &user_text);

        // Append-only invariant check (P1.5).
        history_fp.verify_and_log(&history, &agent_id, &conversation_id);

        // Strip old tool-result bodies before budget checks.
        crate::masking::strip_old_tool_outputs(&mut history, &history_strip_config);
        history_fp = crate::history_guard::HistoryFingerprint::take(&history);

        maybe_run_context_management(
            &mut history,
            &mut history_fp,
            &persisted_messages,
            &immortal_config,
            &mut immortal_state,
            &journal_state,
            &tool_executors,
            &storage,
            &event_bus,
            &agent_id,
            &conversation_id,
        )
        .await;

        let final_user_text = super::volatile::build_layout_text(
            &incoming,
            &mut date_tracker,
            &immortal_state,
            &journal_state,
            standalone_agent,
            turn_count,
            &ping_state,
            &tool_executors,
            &system_reminder,
            &user_text,
            system_reminder_refresh_turns,
        )
        .await;

        let persisted_user_message_clone = persisted_user_message.clone();
        let pre_turn_len = commit_user_turn(
            &mut history,
            &persisted_messages,
            &final_user_text,
            persisted_user_message,
            &storage,
            &conversation_id,
            &event_bus,
            &agent_id,
            &msg_id,
        );

        let start = std::time::Instant::now();

        // Inner loop: streaming may be cut short mid-rig-loop by the
        // immortal hook (`PromptError::PromptCancelled` with reason
        // `"bridge:immortal"`) when the in-flight history exceeds the
        // immortal token budget. When that happens we promote the
        // cancellation history into bridge's working history, run the
        // chain handoff, and re-spawn streaming with a brief continuation
        // prompt so the model picks up where it stopped — same
        // conversation, same turn from the user's perspective. The agent's
        // most recent tool batch (already executed before the hook fired)
        // is preserved in the carry-forward window of the new chain.
        let mut resume_user_text: Option<String> = None;
        let mut current_pre_turn_len = pre_turn_len;
        let mut original_history_backup: Option<Vec<rig::message::Message>> = None;
        let mut last_turn_cancel: Option<tokio_util::sync::CancellationToken> = None;
        let chat_result = 'resume: loop {
            let prompt_for_attempt: &str = resume_user_text
                .as_deref()
                .unwrap_or(final_user_text.as_str());

            let Some(prep) = prepare_turn(
                &abort_token,
                &agent,
                prompt_for_attempt,
                &mut history,
                &immortal_config,
                &llm_semaphore,
            )
            .await
            else {
                break 'resume None;
            };
            let turn_cancel = prep.turn_cancel;
            last_turn_cancel = Some(turn_cancel.clone());
            let history_backup = prep.history_backup;
            if original_history_backup.is_none() {
                original_history_backup = Some(history_backup.clone());
            }
            let stream_inputs = build_stream_inputs(
                prep.stream_prep,
                &event_bus,
                &agent_context,
                &turn_cancel,
                &tool_names,
                &tool_executors,
                &agent_id,
                &conversation_id,
                &permission_manager,
                &agent_permissions,
                &metrics,
                &conversation_metrics,
                &msg_id,
                &storage,
                &persisted_messages,
                &repeat_guard,
            );

            let result_rx = spawn_streaming_task(
                stream_inputs,
                prep.llm_permit,
                &agent_id,
                &conversation_id,
                turn_count,
            );

            let history_backup_for_wait = history_backup.clone();
            let chat_result = match wait_and_dispatch(
                &cancel,
                &turn_cancel,
                result_rx,
                &agent_permissions,
                &mut history,
                history_backup_for_wait,
                &persisted_messages,
                current_pre_turn_len,
                &journal_state,
                &event_bus,
                &agent_id,
                &conversation_id,
            )
            .await
            {
                WaitDisposition::Break => break 'resume None,
                WaitDisposition::Continue => break 'resume Some(WaitDisposition::Continue),
                WaitDisposition::ChatResult(r) => r,
            };

            if is_immortal_cancellation(&chat_result) {
                // Take the cancellation's history out of the result and
                // promote it as bridge's working history.
                let cancel_history = take_cancellation_history(&chat_result);
                if let Some(new_hist) = cancel_history {
                    history = new_hist;
                    // Strip-then-handoff. The strip pass replaces old
                    // tool-result bodies with markers (the bodies are
                    // already on disk via the spill pipeline; the agent
                    // can `RipGrep` them if needed). On bench scenarios
                    // where the entire conversation is a single bridge
                    // turn, this is the ONLY chance the strip pass gets
                    // to run — the top-of-turn call at run.rs:181 fires
                    // exactly once when history is empty. Without this
                    // call, `history_strip` is silently a no-op.
                    crate::masking::strip_old_tool_outputs(&mut history, &history_strip_config);
                    {
                        let mut g = persisted_messages.lock().unwrap();
                        *g = super::convert::convert_from_rig_messages(&history);
                    }
                    history_fp = crate::history_guard::HistoryFingerprint::take(&history);
                    // Force-handoff: bypass `chain_needed` to avoid an
                    // estimator mismatch (the hook uses JSON bytes/4 while
                    // `chain_needed` uses precise tiktoken — they can
                    // disagree near the boundary, which would deadlock the
                    // resume loop). The hook has already decided we need
                    // to handoff; trust it.
                    let pre_chain_tokens = crate::compaction::estimate_tokens(&history);
                    let trigger = crate::immortal::ChainTrigger { pre_chain_tokens };
                    run_chain_handoff(
                        &mut history,
                        &mut history_fp,
                        &persisted_messages,
                        &immortal_config,
                        &mut immortal_state,
                        &storage,
                        &event_bus,
                        &agent_id,
                        &conversation_id,
                        trigger,
                        "hook_threshold",
                    )
                    .await;
                    current_pre_turn_len = persisted_messages.lock().unwrap().len();
                    // Forgecode-style: no continuation prompt. The
                    // summary frame's footer ("Proceed with implementation
                    // based on this context.") is the only directive. Pass
                    // a single space as the rig prompt because rig's API
                    // requires a non-empty `&str` — but we DON'T inject a
                    // user-visible "Continue" message into history.
                    resume_user_text = Some(" ".to_string());
                    continue 'resume;
                }
            }

            break 'resume Some(WaitDisposition::ChatResult(chat_result));
        };
        let chat_result = match chat_result {
            None => break,
            Some(WaitDisposition::Continue) => {
                turn_count += 1;
                continue;
            }
            Some(WaitDisposition::ChatResult(r)) => r,
            Some(WaitDisposition::Break) => break,
        };
        let history_backup = original_history_backup.unwrap_or_else(|| history.clone());
        let pre_turn_len = current_pre_turn_len;
        let turn_cancel = last_turn_cancel.unwrap_or_default();

        let turn_ctx = build_turn_result_ctx(
            &agent_id,
            &conversation_id,
            &agent,
            &retry_agent,
            &event_bus,
            &metrics,
            &conversation_metrics,
            &turn_cancel,
            &tool_names,
            &tool_executors,
            &agent_context,
            &permission_manager,
            &agent_permissions,
            &storage,
            &persisted_messages,
            &journal_state,
            &user_text,
            tool_calls_only,
            &msg_id,
            &tool_requirements,
        );

        if let Some(new_history) = dispatch_chat_result(
            &turn_ctx,
            &mut history,
            history_backup,
            pre_turn_len,
            persisted_user_message_clone,
            start,
            turn_count,
            &mut enforcement_state,
            &mut pending_tool_reminder,
            chat_result,
        )
        .await
        {
            history = new_history;
            history_fp = crate::history_guard::HistoryFingerprint::take(&history);
        }

        turn_count += 1;
    }

    run_conversation_cleanup(
        &permission_manager,
        session_store,
        &per_conversation_mcp_scope,
        &mcp_manager,
        &metrics,
        &conversation_metrics,
        &agent_id,
        &conversation_id,
        turn_count,
    )
    .await;
}
