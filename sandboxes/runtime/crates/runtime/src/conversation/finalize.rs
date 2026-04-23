use bridge_core::event::{BridgeEvent, BridgeEventType};
use serde_json::json;
use std::sync::Arc;
use tracing::warn;
use webhooks::EventBus;

use super::convert::extract_tool_names_from_turn;

/// Apply tool-requirement enforcement for a successful turn.
///
/// Emits `AgentError` events for violations and sets `pending_tool_reminder`
/// when any non-Warn enforcement fires.
#[allow(clippy::too_many_arguments)]
pub(super) fn run_enforcement(
    enforcement_state: &mut Option<crate::tool_enforcement::ToolEnforcementState>,
    tool_requirements: &[bridge_core::agent::ToolRequirement],
    enriched_history: &[rig::message::Message],
    baseline_len: usize,
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
    pending_tool_reminder: &mut Option<String>,
) {
    let Some(ef_state) = enforcement_state.as_mut() else {
        return;
    };

    ef_state.advance_turn();
    let turn_calls = extract_tool_names_from_turn(enriched_history, baseline_len);
    let violations =
        crate::tool_enforcement::evaluate_requirements(ef_state, tool_requirements, &turn_calls);
    if violations.is_empty() {
        return;
    }

    for v in &violations {
        event_bus.emit(BridgeEvent::new(
            BridgeEventType::AgentError,
            agent_id,
            conversation_id,
            json!({
                "code": "tool_requirement_violated",
                "tool": v.requirement.tool,
                "reason": format!("{:?}", v.reason),
                "enforcement": format!("{:?}", v.enforcement()),
                "turn": ef_state.turn_count,
            }),
        ));
        warn!(
            agent_id = %agent_id,
            conversation_id = %conversation_id,
            tool = %v.requirement.tool,
            reason = ?v.reason,
            enforcement = ?v.enforcement(),
            turn = ef_state.turn_count,
            "tool_requirement_violated"
        );
    }
    // Aggregate violations into a single reminder block
    // unless every violation is Warn-only.
    use bridge_core::agent::RequirementEnforcement;
    let has_any_nudge = violations
        .iter()
        .any(|v| v.enforcement() != RequirementEnforcement::Warn);
    if has_any_nudge {
        let nudges: Vec<crate::tool_enforcement::Violation> = violations
            .iter()
            .filter(|v| v.enforcement() != RequirementEnforcement::Warn)
            .cloned()
            .collect();
        let block = crate::tool_enforcement::render_reminder_block(&nudges);
        if !block.is_empty() {
            *pending_tool_reminder = Some(block);
        }
        if violations
            .iter()
            .any(|v| v.enforcement() == RequirementEnforcement::Reprompt)
        {
            // Reprompt currently aliased to NextTurnReminder.
            // A true synchronous reprompt requires re-running
            // the agent inline, which is a follow-up PR; this
            // variant lands the reminder on the next turn.
            warn!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                "Reprompt enforcement currently behaves like NextTurnReminder; synchronous reprompt is a follow-up"
            );
        }
    }
}

/// Emit the standard "turn complete" events — `ResponseCompleted`, `Done`,
/// and `TurnCompleted`. Takes ownership of aggregated turn state.
#[allow(clippy::too_many_arguments)]
pub(super) async fn emit_turn_complete_events(
    event_bus: &Arc<EventBus>,
    agent_id: &str,
    conversation_id: &str,
    msg_id: &str,
    response: &str,
    latency_ms: u64,
    initial_input_tokens: u64,
    initial_cached_input_tokens: u64,
    initial_output_tokens: u64,
    conversation_metrics: &Arc<bridge_core::metrics::ConversationMetrics>,
    turn_count: usize,
    history: &[rig::message::Message],
    journal_state: &Option<Arc<tools::journal::JournalState>>,
) {
    // Signal completion
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::ResponseCompleted,
        agent_id,
        conversation_id,
        json!({
            "message_id": msg_id,
            "input_tokens": initial_input_tokens,
            "cached_input_tokens": initial_cached_input_tokens,
            "output_tokens": initial_output_tokens,
            "model": &conversation_metrics.model,
            "timestamp": chrono::Utc::now().to_rfc3339(),
            "full_response": response,
        }),
    ));
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::Done,
        agent_id,
        conversation_id,
        json!({}),
    ));
    let cm = conversation_metrics.snapshot();
    // Live history-token estimate (the same signal the chain check uses).
    // Cheap vs the per-turn input_tokens since tiktoken runs purely local.
    let history_tokens = crate::compaction::estimate_tokens(history);
    // Commit any journal entries staged during this turn. On failure
    // paths above we discard_staged instead — so spurious agent
    // writes during a rolled-back turn never hit storage.
    let committed_journal_count = if let Some(ref js) = journal_state {
        js.commit_staged().await
    } else {
        0
    };
    event_bus.emit(BridgeEvent::new(
        BridgeEventType::TurnCompleted,
        agent_id,
        conversation_id,
        json!({
            "input_tokens": initial_input_tokens,
            "cached_input_tokens": initial_cached_input_tokens,
            "output_tokens": initial_output_tokens,
            "model": &cm.model,
            "timestamp": chrono::Utc::now().to_rfc3339(),
            "turn_number": turn_count,
            "turn_latency_ms": latency_ms,
            "cumulative_input_tokens": cm.input_tokens,
            "cumulative_cached_input_tokens": cm.cached_input_tokens,
            "cumulative_output_tokens": cm.output_tokens,
            "cumulative_cache_hit_ratio": cm.cache_hit_ratio,
            "cumulative_tool_calls": cm.tool_calls,
            "history_tokens_estimate": history_tokens,
            "history_message_count": history.len(),
            "journal_entries_committed": committed_journal_count,
        }),
    ));
}
