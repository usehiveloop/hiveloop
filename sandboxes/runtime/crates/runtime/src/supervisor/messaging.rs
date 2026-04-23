use bridge_core::conversation::{ContentBlock, Message, Role};
use bridge_core::{BridgeError, MetricsSnapshot};
use tracing::info;

use super::AgentSupervisor;

impl AgentSupervisor {
    /// Send a message to an active conversation.
    pub async fn send_message(
        &self,
        agent_id: &str,
        conversation_id: &str,
        content: String,
        system_reminder: Option<String>,
    ) -> Result<(), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        let handle = state
            .conversations
            .get(conversation_id)
            .ok_or_else(|| BridgeError::ConversationNotFound(conversation_id.to_string()))?;

        let message = Message {
            role: Role::User,
            content: vec![ContentBlock::Text { text: content }],
            timestamp: chrono::Utc::now(),
            system_reminder,
        };

        handle
            .message_tx
            .send(message)
            .await
            .map_err(|_| BridgeError::ConversationEnded(conversation_id.to_string()))?;

        Ok(())
    }

    /// Return the set of tool names currently registered for an agent.
    /// Includes built-in, MCP, integration, and custom tools — matches what
    /// the LLM sees on prompts. Returns `None` if the agent is unknown.
    ///
    /// Used by the API layer (e.g. message-attachment reminder generation)
    /// to tailor system hints to tools the agent actually has.
    pub fn agent_tool_names(&self, agent_id: &str) -> Option<std::collections::HashSet<String>> {
        let state = self.agent_map.get(agent_id)?;
        Some(state.tool_registry.tool_names().into_iter().collect())
    }

    /// End an active conversation.
    pub fn end_conversation(
        &self,
        agent_id: &str,
        conversation_id: &str,
    ) -> Result<(), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        state
            .conversations
            .remove(conversation_id)
            .ok_or_else(|| BridgeError::ConversationNotFound(conversation_id.to_string()))?;

        if let Some(storage) = &self.storage {
            storage.delete_conversation(conversation_id.to_string());
        }

        info!(
            agent_id = agent_id,
            conversation_id = conversation_id,
            "conversation ended"
        );

        // Dropping the handle closes the message_tx sender, which causes the
        // conversation loop to exit gracefully.

        Ok(())
    }

    /// Abort the current in-flight turn for a conversation.
    ///
    /// Cancels the current turn's token, causing the conversation loop to
    /// send an abort SSE event and continue waiting for the next message.
    pub async fn abort_conversation(
        &self,
        agent_id: &str,
        conversation_id: &str,
    ) -> Result<(), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        let handle = state
            .conversations
            .get(conversation_id)
            .ok_or_else(|| BridgeError::ConversationNotFound(conversation_id.to_string()))?;

        // Cancel the current turn's token
        let token = handle.abort_token.lock().await;
        token.cancel();

        info!(
            agent_id = agent_id,
            conversation_id = conversation_id,
            "conversation aborted"
        );
        Ok(())
    }

    /// Gracefully shut down all agents.
    pub async fn shutdown(&self) {
        self.cancel.cancel();

        for agent_id in self.agent_map.agent_ids() {
            if let Some(state) = self.agent_map.get(&agent_id) {
                state.cancel.cancel();
                state.tracker.close();
                state.tracker.wait().await;
            }
            self.mcp_manager.disconnect_agent(&agent_id).await;
        }

        // Shut down LSP servers
        if let Some(ref lsp) = self.lsp_manager {
            lsp.shutdown().await;
        }

        info!("all agents shut down");
    }

    /// Collect metrics from all agents.
    pub async fn collect_metrics(&self) -> Vec<MetricsSnapshot> {
        let agents = self.agent_map.list().await;
        agents
            .iter()
            .filter_map(|summary| {
                self.agent_map
                    .get(&summary.id)
                    .map(|state| state.metrics.snapshot(&summary.id, &summary.name))
            })
            .collect()
    }

    /// Get the number of loaded agents.
    pub fn agent_count(&self) -> usize {
        self.agent_map.len()
    }
}
