use bridge_core::BridgeError;

use crate::state::AppState;

/// Find the agent that owns a conversation by searching all agents.
pub(super) async fn find_agent_for_conversation(
    state: &AppState,
    conv_id: &str,
) -> Result<String, BridgeError> {
    for summary in state.supervisor.list_agents().await {
        if let Some(agent_state) = state.supervisor.get_agent(&summary.id) {
            if agent_state.has_conversation(conv_id) {
                return Ok(summary.id);
            }
        }
    }
    Err(BridgeError::ConversationNotFound(conv_id.to_string()))
}
