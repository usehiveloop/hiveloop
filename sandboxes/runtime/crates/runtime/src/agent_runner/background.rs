use bridge_core::event::{BridgeEvent, BridgeEventType};
use llm::ToolCallEmitter;
use std::sync::Arc;
use tools::agent::{AgentContext, AgentTaskHandle, AgentTaskNotification, AGENT_CONTEXT};

use super::ConversationSubAgentRunner;

pub(super) async fn run_background(
    runner: &ConversationSubAgentRunner,
    subagent: &str,
    prompt: &str,
    description: &str,
) -> Result<AgentTaskHandle, String> {
    tracing::debug!(
        subagent = subagent,
        parent_conversation_id = %runner.conversation_id,
        mode = "background",
        "gen_ai.agent.execute"
    );

    // Emit SubAgentStarted event
    runner.event_bus.emit(BridgeEvent::new(
        BridgeEventType::SubAgentStarted,
        &runner.agent_id,
        &runner.conversation_id,
        serde_json::json!({
            "subagent_name": subagent,
            "mode": "background",
            "parent_conversation_id": &runner.conversation_id,
            "depth": runner.depth,
        }),
    ));

    let entry = runner
        .subagents
        .get(subagent)
        .ok_or_else(|| format!("Subagent '{}' not found", subagent))?;

    let agent = entry.agent.clone();
    let background_timeout = entry.background_timeout;
    let task_id = runner.generate_task_id();
    let task_id_clone = task_id.clone();

    let mut history = runner.session_store.get_or_create(&task_id);
    let compaction_config = runner.compaction_config.clone();

    // Compact subagent history if configured
    if let Some(ref config) = compaction_config {
        if let Ok(Some(result)) = crate::compaction::maybe_compact(&history, config).await {
            history = result.compacted_history;
            runner.session_store.save(task_id.clone(), history.clone());
        }
    }

    let session_store = runner.session_store.clone();
    let notification_tx = runner.notification_tx.clone();
    let cancel = runner.cancel.clone();
    let event_bus = runner.event_bus.clone();
    let prompt_owned = prompt.to_string();
    let description_owned = description.to_string();
    let subagents = runner.subagents.clone();
    let conversation_id = runner.conversation_id.clone();
    let depth = runner.depth;
    let max_depth = runner.max_depth;
    let task_budget = runner.task_budget.clone();
    let metrics_clone = runner.metrics.clone();
    let agent_id_clone = runner.agent_id.clone();
    let subagent_name = subagent.to_string();

    tokio::spawn(async move {
        let bg_start = std::time::Instant::now();
        let emitter_conv_id = conversation_id.clone();
        let event_conv_id = conversation_id.clone();
        // Build nested AgentContext for the background task
        let nested_runner = Arc::new(
            ConversationSubAgentRunner::new(
                subagents,
                session_store.clone(),
                notification_tx.clone(),
                cancel.clone(),
                event_bus.clone(),
                conversation_id,
                depth + 1,
                max_depth,
                metrics_clone.clone(),
            )
            .with_task_budget(task_budget.clone()),
        );
        let nested_ctx = AgentContext {
            runner: nested_runner,
            notification_tx: notification_tx.clone(),
            depth: depth + 1,
            max_depth,
            task_budget,
        };

        let mut history = history;
        let emitter = ToolCallEmitter {
            event_bus: event_bus.clone(),
            cancel: cancel.clone(),
            tool_names: std::collections::HashSet::new(),
            tool_executors: std::collections::HashMap::new(),
            agent_id: agent_id_clone.clone(),
            conversation_id: emitter_conv_id,
            permission_manager: std::sync::Arc::new(llm::PermissionManager::new()),
            agent_permissions: std::collections::HashMap::new(),
            metrics: metrics_clone,
            conversation_metrics: None,
            pending_tool_timings: std::sync::Arc::new(dashmap::DashMap::new()),
            storage: None,
            persisted_messages: None,
            pressure_threshold_bytes: None,
            pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
            pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
        };

        let result = AGENT_CONTEXT
            .scope(nested_ctx, async {
                tokio::select! {
                    _ = cancel.cancelled() => {
                        Err("Background subagent cancelled".to_string())
                    }
                    result = async {
                        tokio::time::timeout(
                            background_timeout,
                            agent.prompt_standard_with_hook(&prompt_owned, &mut history, emitter),
                        ).await
                    } => {
                        match result {
                            Err(_) => Err(format!("Background subagent timed out after {}s", background_timeout.as_secs())),
                            Ok(Ok(output)) => Ok(output),
                            Ok(Err(e)) => Err(format!("Background subagent error: {}", e)),
                        }
                    }
                }
            })
            .await;

        // Save history
        session_store.save(task_id_clone.clone(), history);

        // Emit SubAgentCompleted event
        {
            let duration_ms = bg_start.elapsed().as_millis() as u64;
            event_bus.emit(BridgeEvent::new(
                BridgeEventType::SubAgentCompleted,
                &agent_id_clone,
                &event_conv_id,
                serde_json::json!({
                    "subagent_name": &subagent_name,
                    "mode": "background",
                    "task_id": &task_id_clone,
                    "duration_ms": duration_ms,
                    "is_error": result.is_err(),
                }),
            ));
        }

        // Send notification
        let notification = AgentTaskNotification {
            task_id: task_id_clone.clone(),
            description: description_owned,
            output: result,
        };

        if notification_tx.send(notification).await.is_err() {
            tracing::debug!(
                task_id = %task_id_clone,
                "notification channel closed, conversation likely ended"
            );
        }
    });

    Ok(AgentTaskHandle { task_id })
}
