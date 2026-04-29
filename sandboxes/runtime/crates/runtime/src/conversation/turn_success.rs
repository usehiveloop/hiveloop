use bridge_core::conversation::Message;
use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::json;
use tracing::info;

use super::convert::{convert_from_rig_messages, history_contains_tool_calls};
use super::finalize::{emit_turn_complete_events, run_enforcement};
use super::recovery::{attempt_empty_response_recovery, RecoveryInputs};
use super::turn_classify::classify_turn_result;
use super::turn_result::{TurnOutcome, TurnResultCtx};
use crate::token_tracker;

/// Handle the success path where the spawned task returned. Covers both
/// successful responses and the recovery paths for empty/parse-error responses.
#[allow(clippy::too_many_arguments)]
pub(super) async fn handle_got_result(
    ctx: &TurnResultCtx<'_>,
    history_backup: Vec<rig::message::Message>,
    pre_turn_len: usize,
    persisted_user_message_clone: Message,
    start: std::time::Instant,
    turn_count: usize,
    result: Result<llm::PromptResponse, rig::completion::PromptError>,
    mut enriched_history: Vec<rig::message::Message>,
    enforcement_state: &mut Option<crate::tool_enforcement::ToolEnforcementState>,
    pending_tool_reminder: &mut Option<String>,
) -> TurnOutcome {
    let latency_ms = start.elapsed().as_millis() as u64;

    let (response_text, initial_input_tokens, initial_cached_input_tokens, initial_output_tokens) =
        match classify_turn_result(ctx, result, pre_turn_len).await {
            Ok(tuple) => tuple,
            Err(()) => {
                let _ = history_backup; // caller will restore via FatalRestored
                return TurnOutcome::FatalRestored;
            }
        };

    let has_text = matches!(&response_text, Some(text) if !text.is_empty());
    let had_tool_calls = history_contains_tool_calls(&enriched_history, history_backup.len());

    // Detect leaked tool calls — model emitted its native tool-call template
    // (Hermes XML, MiMo function tag, Gemma special tokens, Tencent template,
    // bare-JSON Qwen) as plain text instead of structured `tool_calls`. The
    // turn looks complete (`has_text=true`, `had_tool_calls=false`) but the
    // model was actually trying to call a tool the inference server failed
    // to translate. Treat as a recoverable empty response so the agent gets
    // one more shot to re-emit using the proper schema.
    let leaked_tool_call = response_text
        .as_deref()
        .map(llm::providers::tool_call_recovery::detect_leak)
        .unwrap_or(false);
    if leaked_tool_call {
        tracing::warn!(
            agent_id = ctx.agent_id,
            conversation_id = ctx.conversation_id,
            response_preview = %response_text.as_deref().unwrap_or("").chars().take(200).collect::<String>(),
            "tool_call_leak_detected — model emitted native tool-call template as text; routing through empty-response recovery"
        );
    }

    let needs_recovery = if ctx.tool_calls_only && had_tool_calls {
        // Agent is configured to complete with tool calls only — no text needed.
        false
    } else {
        !has_text || leaked_tool_call
    };

    let response = if needs_recovery {
        let recovery_inputs = RecoveryInputs {
            agent_id: ctx.agent_id,
            conversation_id: ctx.conversation_id,
            agent: ctx.agent,
            retry_agent: ctx.retry_agent,
            event_bus: ctx.event_bus,
            turn_cancel: ctx.turn_cancel,
            tool_names: ctx.tool_names,
            tool_executors: ctx.tool_executors,
            agent_context: ctx.agent_context,
            permission_manager: ctx.permission_manager,
            agent_permissions: ctx.agent_permissions,
            metrics: ctx.metrics,
            conversation_metrics: ctx.conversation_metrics,
            storage: ctx.storage,
            persisted_messages: ctx.persisted_messages,
            user_text: ctx.user_text,
            tool_calls_only: ctx.tool_calls_only,
        };
        attempt_empty_response_recovery(&recovery_inputs, &mut enriched_history).await
    } else {
        response_text.unwrap_or_default()
    };

    info!(
        agent_id = ctx.agent_id,
        conversation_id = ctx.conversation_id,
        response_len = response.len(),
        response_preview = %response.chars().take(500).collect::<String>(),
        latency_ms = latency_ms,
        input_tokens = initial_input_tokens,
        cached_input_tokens = initial_cached_input_tokens,
        cache_hit_ratio = bridge_core::metrics::cache_hit_ratio(
            initial_input_tokens,
            initial_cached_input_tokens
        ),
        output_tokens = initial_output_tokens,
        "agent response finalized"
    );

    // Send the response as content delta only for recovery responses.
    // In the normal streaming path, text was already sent incrementally
    // via ContentDelta events from the spawned task.
    if !response.is_empty() && needs_recovery {
        ctx.event_bus.emit(BridgeEvent::new(
            BridgeEventType::ResponseChunk,
            ctx.agent_id,
            ctx.conversation_id,
            json!({
                "delta": &response,
                "message_id": ctx.msg_id,
            }),
        ));
    }

    // Tool-requirement enforcement.
    run_enforcement(
        enforcement_state,
        ctx.tool_requirements,
        &enriched_history,
        history_backup.len(),
        ctx.event_bus,
        ctx.agent_id,
        ctx.conversation_id,
        pending_tool_reminder,
    );

    // Authoritative rebuild: discard incremental tool messages added
    // during the turn and replace with the canonical rig history.
    let new_persisted_messages =
        convert_from_rig_messages(&enriched_history[history_backup.len()..]);
    {
        let mut guard = ctx.persisted_messages.lock().unwrap();
        guard.truncate(pre_turn_len);
        guard.push(persisted_user_message_clone);
        guard.extend(new_persisted_messages);
    }

    if let Some(storage) = ctx.storage {
        storage.replace_messages(
            ctx.conversation_id.to_string(),
            ctx.persisted_messages.lock().unwrap().clone(),
        );
    }

    // Record metrics (dual-write to agent + conversation)
    token_tracker::record_request(
        ctx.metrics,
        Some(ctx.conversation_metrics),
        initial_input_tokens,
        initial_cached_input_tokens,
        initial_output_tokens,
        latency_ms,
    );

    // Verifier gating: ask the classifier whether this turn is genuinely
    // done. On `needs_work + high` (within cap), append a synthetic user
    // message and ask the runtime to resume the same turn. On any other
    // verdict — or any verifier error — proceed as today.
    let verifier_action = super::verifier::run_if_enabled(
        ctx.verifier_config,
        ctx.verifier_client,
        ctx.agent_system_prompt,
        &enriched_history,
        history_backup.len(),
        ctx.reprompt_count,
        ctx.event_bus,
        ctx.agent_id,
        ctx.conversation_id,
    )
    .await;

    if let super::verifier::VerifierAction::Reprompt { synthetic_prompt } = verifier_action {
        enriched_history.push(rig::message::Message::user(&synthetic_prompt));
        let synthetic_persisted = bridge_core::conversation::Message {
            role: bridge_core::conversation::Role::User,
            content: vec![bridge_core::conversation::ContentBlock::Text {
                text: synthetic_prompt.clone(),
            }],
            timestamp: chrono::Utc::now(),
            system_reminder: None,
        };
        {
            let mut guard = ctx.persisted_messages.lock().unwrap();
            guard.push(synthetic_persisted);
        }
        if let Some(storage) = ctx.storage {
            storage.replace_messages(
                ctx.conversation_id.to_string(),
                ctx.persisted_messages.lock().unwrap().clone(),
            );
        }
        return TurnOutcome::ResumeSameTurn {
            new_history: enriched_history,
            synthetic_prompt,
        };
    }

    emit_turn_complete_events(
        ctx.event_bus,
        ctx.agent_id,
        ctx.conversation_id,
        ctx.msg_id,
        &response,
        latency_ms,
        initial_input_tokens,
        initial_cached_input_tokens,
        initial_output_tokens,
        ctx.conversation_metrics,
        turn_count,
        &enriched_history,
        ctx.journal_state,
    )
    .await;

    TurnOutcome::Completed {
        new_history: enriched_history,
    }
}
