use bridge_core::{AgentDefinition, BridgeError};
use tracing::info;

use crate::state::AppState;

pub(super) async fn restore_stored_conversations_for_agent(
    state: &AppState,
    agent_id: &str,
) -> Result<(), BridgeError> {
    let Some(storage_backend) = state.storage_backend.as_ref() else {
        return Ok(());
    };

    let Some(agent) = state.supervisor.get_agent(agent_id) else {
        return Ok(());
    };

    if agent.active_conversation_count() > 0 {
        return Ok(());
    }

    let records = storage_backend
        .load_conversations(agent_id)
        .await
        .map_err(|e| {
            BridgeError::Internal(format!(
                "failed to load stored conversations for {}: {}",
                agent_id, e
            ))
        })?;

    if records.is_empty() {
        return Ok(());
    }

    let restored = records.len();
    let sse_receivers = state
        .supervisor
        .hydrate_conversations(agent_id, records)
        .await;
    for (conv_id, sse_rx) in sse_receivers {
        state.sse_streams.insert(conv_id, sse_rx);
    }

    info!(
        agent_id = agent_id,
        count = restored,
        "restored conversations after agent load"
    );
    Ok(())
}

pub(super) fn definitions_equivalent(
    existing: &AgentDefinition,
    incoming: &AgentDefinition,
) -> bool {
    match (&existing.version, &incoming.version) {
        (Some(existing_version), Some(incoming_version)) => existing_version == incoming_version,
        _ => existing == incoming,
    }
}
