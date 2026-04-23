use bridge_core::event::{BridgeEvent, BridgeEventType};
use llm::ToolCallEmitter;
use tools::agent::AgentTaskResult;

use super::ConversationSubAgentRunner;

pub(super) async fn run_foreground(
    runner: &ConversationSubAgentRunner,
    subagent: &str,
    prompt: &str,
    task_id: Option<&str>,
) -> Result<AgentTaskResult, String> {
    let start = std::time::Instant::now();

    tracing::debug!(
        subagent = subagent,
        parent_conversation_id = %runner.conversation_id,
        mode = "foreground",
        "gen_ai.agent.execute"
    );

    // Emit SubAgentStarted event
    runner.event_bus.emit(BridgeEvent::new(
        BridgeEventType::SubAgentStarted,
        &runner.agent_id,
        &runner.conversation_id,
        serde_json::json!({
            "subagent_name": subagent,
            "mode": "foreground",
            "parent_conversation_id": &runner.conversation_id,
            "depth": runner.depth,
        }),
    ));

    let entry = runner
        .subagents
        .get(subagent)
        .ok_or_else(|| format!("Subagent '{}' not found", subagent))?;

    let agent = entry.agent.clone();
    let foreground_timeout = entry.foreground_timeout;
    let task_id = task_id
        .map(|id| id.to_string())
        .unwrap_or_else(|| runner.generate_task_id());

    let mut history = runner.session_store.get_or_create(&task_id);

    // Compact subagent history if configured
    if let Some(ref config) = runner.compaction_config {
        if let Ok(Some(result)) = crate::compaction::maybe_compact(&history, config).await {
            history = result.compacted_history;
            runner.session_store.save(task_id.clone(), history.clone());
        }
    }

    let cancel = runner.cancel.clone();
    let emitter = ToolCallEmitter {
        event_bus: runner.event_bus.clone(),
        cancel: cancel.clone(),
        tool_names: std::collections::HashSet::new(),
        tool_executors: std::collections::HashMap::new(),
        agent_id: runner.agent_id.clone(),
        conversation_id: runner.conversation_id.clone(),
        permission_manager: std::sync::Arc::new(llm::PermissionManager::new()),
        agent_permissions: std::collections::HashMap::new(),
        metrics: runner.metrics.clone(),
        conversation_metrics: None,
        pending_tool_timings: std::sync::Arc::new(dashmap::DashMap::new()),
        storage: None,
        persisted_messages: None,
        pressure_threshold_bytes: None,
        pressure_counter: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        pressure_warned: std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false)),
    };

    let prompt_owned = prompt.to_string();

    let result = tokio::select! {
        _ = cancel.cancelled() => {
            Err("Subagent cancelled".to_string())
        }
        result = async {
            tokio::time::timeout(
                foreground_timeout,
                agent.prompt_standard_with_hook(&prompt_owned, &mut history, emitter),
            ).await
        } => {
            match result {
                Err(_) => Err(format!("Subagent timed out after {}s", foreground_timeout.as_secs())),
                Ok(Ok(output)) => Ok(output),
                Ok(Err(e)) => Err(format!("Subagent error: {}", e)),
            }
        }
    };

    // Save history regardless of outcome (for resumption)
    runner.session_store.save(task_id.clone(), history);

    let duration_ms = start.elapsed().as_millis() as u64;
    let is_error = result.is_err();

    // Emit SubAgentCompleted event
    runner.event_bus.emit(BridgeEvent::new(
        BridgeEventType::SubAgentCompleted,
        &runner.agent_id,
        &runner.conversation_id,
        serde_json::json!({
            "subagent_name": subagent,
            "mode": "foreground",
            "task_id": &task_id,
            "parent_conversation_id": &runner.conversation_id,
            "duration_ms": duration_ms,
            "is_error": is_error,
        }),
    ));

    match result {
        Ok(output) => Ok(AgentTaskResult { task_id, output }),
        Err(e) => Err(e),
    }
}
