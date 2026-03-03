use bridge_core::conversation::{ContentBlock, Message, Role};
use bridge_core::{AgentDefinition, AgentSummary, BridgeError, MetricsSnapshot};
use dashmap::DashMap;
use llm::{adapt_tools, build_agent, DynamicTool, SseEvent};
use mcp::McpManager;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification};
use tools::ToolRegistry;
use tracing::{error, info};

use crate::agent_map::AgentMap;
use crate::agent_runner::{ConversationSubAgentRunner, SubAgentEntry};
use crate::agent_state::{AgentState, ConversationHandle};
use crate::conversation::{run_conversation, ConversationParams};
use crate::drain::drain_and_replace;

/// Central supervisor that manages all agent lifecycles.
///
/// Handles loading agents, creating conversations, routing messages,
/// and applying configuration diffs from the control plane.
pub struct AgentSupervisor {
    /// Map of all loaded agents.
    agent_map: AgentMap,
    /// MCP connection manager shared across agents.
    mcp_manager: Arc<McpManager>,
    /// Global cancellation token.
    cancel: CancellationToken,
}

impl AgentSupervisor {
    /// Create a new supervisor.
    pub fn new(mcp_manager: Arc<McpManager>, cancel: CancellationToken) -> Self {
        Self {
            agent_map: AgentMap::new(),
            mcp_manager,
            cancel,
        }
    }

    /// Load a set of agent definitions, building their runtime state.
    pub async fn load_agents(&self, definitions: Vec<AgentDefinition>) -> Result<(), BridgeError> {
        for def in definitions {
            if let Err(e) = self.load_single_agent(def).await {
                error!(error = %e, "failed to load agent");
            }
        }
        Ok(())
    }

    /// Build and load a single agent.
    async fn load_single_agent(&self, definition: AgentDefinition) -> Result<(), BridgeError> {
        let agent_id = definition.id.clone();

        // Connect to MCP servers and discover tools
        let mcp_tools = self
            .mcp_manager
            .connect_agent(&agent_id, &definition.mcp_servers)
            .await?;

        // Build tool registry with MCP tools
        let mut tool_registry = ToolRegistry::new();
        let connections = self.mcp_manager.get_agent_connections(&agent_id);
        for conn in &connections {
            let bridged = mcp::bridge_mcp_tools(conn.clone(), mcp_tools.clone());
            for tool in bridged {
                tool_registry.register(tool);
            }
        }

        // Register built-in tools (filesystem, web fetch, web search)
        tools::builtin::register_builtin_tools(&mut tool_registry);

        // Collect all tool executors for the LLM agent
        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| tool_registry.get(name))
            .collect();

        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;

        // Build the rig agent
        let rig_agent = build_agent(&definition, dynamic_tools)?;

        // Build subagents from definition.subagents
        let subagent_map = build_subagents(&definition)?;

        let state = Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
        ));
        self.agent_map.insert(agent_id.clone(), state);

        info!(agent_id = agent_id, "agent loaded");
        Ok(())
    }

    /// Get an agent state by ID.
    pub fn get_agent(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.agent_map.get(agent_id)
    }

    /// List all loaded agents.
    pub fn list_agents(&self) -> Vec<AgentSummary> {
        self.agent_map.list()
    }

    /// Create a new conversation for an agent.
    ///
    /// Returns the conversation ID and an SSE event receiver for streaming responses.
    pub fn create_conversation(
        &self,
        agent_id: &str,
    ) -> Result<(String, mpsc::Receiver<SseEvent>), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        let conv_id = uuid::Uuid::new_v4().to_string();
        let (message_tx, message_rx) = mpsc::channel::<Message>(32);
        let (sse_tx, sse_rx) = mpsc::channel::<SseEvent>(256);

        let handle = ConversationHandle {
            id: conv_id.clone(),
            message_tx,
            created_at: chrono::Utc::now(),
        };

        state.conversations.insert(conv_id.clone(), handle);

        // Spawn the conversation task
        let agent = Arc::new(state.rig_agent.clone());
        let metrics = state.metrics.clone();
        let cancel = state.cancel.clone();
        let max_turns = state.definition.config.max_turns;
        let agent_id_owned = agent_id.to_string();
        let conv_id_clone = conv_id.clone();

        // Build agent context for subagent tool
        let (notification_tx, notification_rx) =
            mpsc::channel::<AgentTaskNotification>(64);
        let runner = Arc::new(ConversationSubAgentRunner::new(
            state.subagents.clone(),
            state.session_store.clone(),
            notification_tx.clone(),
            cancel.clone(),
            sse_tx.clone(),
            conv_id_clone.clone(),
            0, // depth
            3, // max_depth
        ));
        let agent_context = AgentContext {
            runner,
            notification_tx,
            depth: 0,
            max_depth: 3,
        };
        let session_store = state.session_store.clone();

        state.tracker.spawn(async move {
            run_conversation(ConversationParams {
                agent_id: agent_id_owned,
                conversation_id: conv_id_clone,
                agent,
                message_rx,
                sse_tx,
                metrics,
                cancel,
                max_turns: max_turns.map(|t| t as usize),
                agent_context: Some(agent_context),
                notification_rx: Some(notification_rx),
                session_store: Some(session_store),
            })
            .await;
        });

        info!(
            agent_id = agent_id,
            conversation_id = conv_id,
            "conversation created"
        );

        Ok((conv_id, sse_rx))
    }

    /// Send a message to an active conversation.
    pub async fn send_message(
        &self,
        agent_id: &str,
        conversation_id: &str,
        content: String,
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
        };

        handle
            .message_tx
            .send(message)
            .await
            .map_err(|_| BridgeError::ConversationEnded(conversation_id.to_string()))?;

        Ok(())
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

        info!(
            agent_id = agent_id,
            conversation_id = conversation_id,
            "conversation ended"
        );

        // Dropping the handle closes the message_tx sender, which causes the
        // conversation loop to exit gracefully.

        Ok(())
    }

    /// Apply a diff of agent changes (add/update/remove).
    pub async fn apply_diff(
        &self,
        added: Vec<AgentDefinition>,
        updated: Vec<AgentDefinition>,
        removed: Vec<String>,
    ) -> Result<(), BridgeError> {
        // Add new agents
        for def in added {
            if let Err(e) = self.load_single_agent(def).await {
                error!(error = %e, "failed to add agent");
            }
        }

        // Update existing agents with drain
        for def in updated {
            let agent_id = def.id.clone();
            match self.load_single_agent_state(def).await {
                Ok(new_state) => {
                    let timeout = std::time::Duration::from_secs(60);
                    if let Err(e) =
                        drain_and_replace(&self.agent_map, &agent_id, new_state, timeout).await
                    {
                        error!(agent_id = agent_id, error = %e, "failed to drain and replace agent");
                    }
                }
                Err(e) => {
                    error!(agent_id = agent_id, error = %e, "failed to build updated agent");
                }
            }
        }

        // Remove agents
        for agent_id in removed {
            if let Some(state) = self.agent_map.remove(&agent_id) {
                state.cancel.cancel();
                state.tracker.close();
                state.tracker.wait().await;
                self.mcp_manager.disconnect_agent(&agent_id).await;
                info!(agent_id = agent_id, "agent removed");
            }
        }

        Ok(())
    }

    /// Build agent state without inserting into the map (used for drain_and_replace).
    async fn load_single_agent_state(
        &self,
        definition: AgentDefinition,
    ) -> Result<Arc<AgentState>, BridgeError> {
        let agent_id = definition.id.clone();

        let mcp_tools = self
            .mcp_manager
            .connect_agent(&agent_id, &definition.mcp_servers)
            .await?;

        let mut tool_registry = ToolRegistry::new();
        let connections = self.mcp_manager.get_agent_connections(&agent_id);
        for conn in &connections {
            let bridged = mcp::bridge_mcp_tools(conn.clone(), mcp_tools.clone());
            for tool in bridged {
                tool_registry.register(tool);
            }
        }

        // Register built-in tools (filesystem, web fetch, web search)
        tools::builtin::register_builtin_tools(&mut tool_registry);

        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| tool_registry.get(name))
            .collect();

        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;
        let rig_agent = build_agent(&definition, dynamic_tools)?;

        // Build subagents from definition.subagents
        let subagent_map = build_subagents(&definition)?;

        Ok(Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
        )))
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

        info!("all agents shut down");
    }

    /// Collect metrics from all agents.
    pub fn collect_metrics(&self) -> Vec<MetricsSnapshot> {
        self.agent_map
            .list()
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

/// Build subagent entries from an agent definition's subagents list.
///
/// Each subagent gets built-in tools only (no MCP, no agent tool to prevent
/// unbounded recursion at the configuration level).
fn build_subagents(
    definition: &AgentDefinition,
) -> Result<Arc<DashMap<String, SubAgentEntry>>, BridgeError> {
    let subagent_map = Arc::new(DashMap::new());

    for subagent_def in &definition.subagents {
        let mut sub_registry = ToolRegistry::new();
        tools::builtin::register_builtin_tools_for_subagent(&mut sub_registry);

        let sub_executors: Vec<Arc<dyn tools::ToolExecutor>> = sub_registry
            .list()
            .iter()
            .filter_map(|(name, _)| sub_registry.get(name))
            .collect();

        let sub_dynamic = adapt_tools(sub_executors)?;
        let sub_agent = build_agent(subagent_def, sub_dynamic)?;

        let description = subagent_def
            .description
            .clone()
            .unwrap_or_else(|| subagent_def.name.clone());

        subagent_map.insert(
            subagent_def.name.clone(),
            SubAgentEntry {
                name: subagent_def.name.clone(),
                description,
                agent: Arc::new(sub_agent),
            },
        );
    }

    Ok(subagent_map)
}
