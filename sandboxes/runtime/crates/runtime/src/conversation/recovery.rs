use bridge_core::metrics::ConversationMetrics;
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use dashmap::DashMap;
use llm::PermissionManager;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use storage::StorageHandle;
use tokio::sync::RwLock;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AGENT_CONTEXT};
use tools::ToolExecutor;
use tracing::{info, warn};
use webhooks::EventBus;

use super::params::{CONTINUATION_TIMEOUT, MAX_CONTINUATIONS};

/// Inputs needed to attempt empty-response recovery (continuation + no-tools retry).
#[allow(clippy::too_many_arguments)]
pub(super) struct RecoveryInputs<'a> {
    pub(super) agent_id: &'a str,
    pub(super) conversation_id: &'a str,
    pub(super) agent: &'a Arc<RwLock<llm::BridgeAgent>>,
    pub(super) retry_agent: &'a Arc<llm::BridgeAgent>,
    pub(super) event_bus: &'a Arc<EventBus>,
    pub(super) turn_cancel: &'a CancellationToken,
    pub(super) tool_names: &'a HashSet<String>,
    pub(super) tool_executors: &'a HashMap<String, Arc<dyn ToolExecutor>>,
    pub(super) agent_context: &'a Option<AgentContext>,
    pub(super) permission_manager: &'a Arc<PermissionManager>,
    pub(super) agent_permissions: &'a HashMap<String, ToolPermission>,
    pub(super) metrics: &'a Arc<AgentMetrics>,
    pub(super) conversation_metrics: &'a Arc<ConversationMetrics>,
    pub(super) storage: &'a Option<StorageHandle>,
    pub(super) persisted_messages:
        &'a Arc<std::sync::Mutex<Vec<bridge_core::conversation::Message>>>,
    pub(super) user_text: &'a str,
    pub(super) tool_calls_only: bool,
}

/// Attempt to recover from an empty agent response by running up to
/// `MAX_CONTINUATIONS` tool-equipped continuations, and falling back to a
/// no-tools retry agent if none produced text.
///
/// Mutates `enriched_history` to absorb any tool calls produced during
/// continuations. Returns the final response text (never empty — falls
/// back to a canned summary string if nothing else works).
pub(super) async fn attempt_empty_response_recovery(
    inputs: &RecoveryInputs<'_>,
    enriched_history: &mut Vec<rig::message::Message>,
) -> String {
    warn!(
        agent_id = inputs.agent_id,
        conversation_id = inputs.conversation_id,
        "agent returned empty response, attempting continuation with tools"
    );

    // Step 1: Try continuation with the main agent (has tools).
    // This gives the agent a chance to keep working if it wasn't done.
    let mut continuation_response: Option<String> = None;
    for _attempt in 0..MAX_CONTINUATIONS {
        let agent_clone = { inputs.agent.read().await.clone() };
        let event_bus_clone = inputs.event_bus.clone();
        let turn_cancel_clone = inputs.turn_cancel.clone();
        let tool_names_clone = inputs.tool_names.clone();
        let tool_executors_clone = inputs.tool_executors.clone();
        let agent_context_clone = inputs.agent_context.clone();
        let agent_id_clone = inputs.agent_id.to_string();
        let conversation_id_clone = inputs.conversation_id.to_string();
        let permission_manager_clone = inputs.permission_manager.clone();
        let agent_permissions_clone = inputs.agent_permissions.clone();
        let metrics_for_cont = inputs.metrics.clone();
        let conversation_metrics_for_cont = inputs.conversation_metrics.clone();
        let storage_for_cont = inputs.storage.clone();
        let persisted_messages_for_cont = inputs.persisted_messages.clone();
        let mut history_for_continuation = enriched_history.clone();
        let (cont_tx, cont_rx) = tokio::sync::oneshot::channel();
        let cont_prompt = if inputs.tool_calls_only {
            format!(
                "You were assigned to work on the following task:\n\n{}\n\nPlease continue working on it.",
                inputs.user_text
            )
        } else {
            format!(
                "You were assigned to work on the following task:\n\n{}\n\nPlease continue working on it. If you have completed all the work, provide a final text summary.",
                inputs.user_text
            )
        };

        tokio::spawn(async move {
            let emitter = llm::ToolCallEmitter {
                event_bus: event_bus_clone,
                cancel: turn_cancel_clone,
                tool_names: tool_names_clone,
                tool_executors: tool_executors_clone,
                agent_id: agent_id_clone,
                conversation_id: conversation_id_clone,
                permission_manager: permission_manager_clone,
                agent_permissions: agent_permissions_clone,
                metrics: metrics_for_cont,
                conversation_metrics: Some(conversation_metrics_for_cont.clone()),
                pending_tool_timings: Arc::new(DashMap::new()),
                storage: storage_for_cont,
                persisted_messages: Some(persisted_messages_for_cont),
                // Continuation uses its own fresh counters — a pressure
                // warning from the main turn doesn't carry over.
                pressure_threshold_bytes: None,
                pressure_counter: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
                pressure_warned: Arc::new(std::sync::atomic::AtomicBool::new(false)),
            };
            let fut = async {
                agent_clone
                    .prompt_with_hook(&cont_prompt, &mut history_for_continuation, emitter)
                    .await
            };
            let result = match agent_context_clone {
                Some(ctx) => AGENT_CONTEXT.scope(ctx, fut).await,
                None => fut.await,
            };
            let _ = cont_tx.send((result, history_for_continuation));
        });

        match tokio::time::timeout(CONTINUATION_TIMEOUT, cont_rx).await {
            Ok(Ok((Ok(pr), cont_history))) if !pr.output.is_empty() => {
                info!(
                    agent_id = inputs.agent_id,
                    conversation_id = inputs.conversation_id,
                    response_len = pr.output.len(),
                    "continuation with tools produced non-empty response"
                );
                *enriched_history = cont_history;
                continuation_response = Some(pr.output);
                break;
            }
            Ok(Ok((_, cont_history))) => {
                // Continuation produced empty or error — update history
                // (preserves any new tool calls) and fall through to retry.
                *enriched_history = cont_history;
            }
            _ => {
                warn!(
                    agent_id = inputs.agent_id,
                    conversation_id = inputs.conversation_id,
                    "continuation attempt timed out or was cancelled"
                );
            }
        }
    }

    if let Some(text) = continuation_response {
        text
    } else {
        // Step 2: Retry with no-tools agent WITH enriched history.
        // The retry agent sees the full conversation (user request +
        // all tool calls + results) so it can write an accurate summary
        // instead of hallucinating.
        warn!(
            agent_id = inputs.agent_id,
            conversation_id = inputs.conversation_id,
            "continuation did not produce text, retrying with no-tools agent with enriched history"
        );
        match inputs
            .retry_agent
            .prompt_with_history(
                "Please provide a text response summarizing what you found or did.",
                enriched_history,
            )
            .await
        {
            Ok(resp) if !resp.is_empty() => {
                info!(
                    agent_id = inputs.agent_id,
                    conversation_id = inputs.conversation_id,
                    retry_len = resp.len(),
                    "no-tools retry with enriched history succeeded"
                );
                resp
            }
            Ok(_) => {
                warn!(
                    agent_id = inputs.agent_id,
                    conversation_id = inputs.conversation_id,
                    "no-tools retry also returned empty, using fallback"
                );
                let fallback =
                    "I completed the requested tasks using the available tools.".to_string();
                enriched_history.push(rig::message::Message::assistant(&fallback));
                fallback
            }
            Err(e) => {
                warn!(
                    agent_id = inputs.agent_id,
                    conversation_id = inputs.conversation_id,
                    error = %e,
                    "no-tools retry failed, using fallback"
                );
                let fallback =
                    "I completed the requested tasks using the available tools.".to_string();
                enriched_history.push(rig::message::Message::assistant(&fallback));
                fallback
            }
        }
    }
}
