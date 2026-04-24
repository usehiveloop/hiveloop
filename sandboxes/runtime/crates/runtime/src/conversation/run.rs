use tracing::info;

use super::context_mgmt::maybe_run_context_management;
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
        compaction_config,
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
            &compaction_config,
            &journal_state,
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

        let Some(prep) = prepare_turn(
            &abort_token,
            &agent,
            &final_user_text,
            &mut history,
            &immortal_config,
            &llm_semaphore,
        )
        .await
        else {
            break;
        };
        let turn_cancel = prep.turn_cancel;
        let history_backup = prep.history_backup;
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
            pre_turn_len,
            &journal_state,
            &event_bus,
            &agent_id,
            &conversation_id,
        )
        .await
        {
            WaitDisposition::Break => break,
            WaitDisposition::Continue => {
                turn_count += 1;
                continue;
            }
            WaitDisposition::ChatResult(r) => r,
        };

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
