use bridge_core::conversation::{ContentBlock, ConversationRecord, Message, Role};
use bridge_core::{AgentDefinition, AgentSummary, BridgeError, MetricsSnapshot};
use dashmap::DashMap;
use llm::{adapt_tools, build_agent, DynamicTool, PermissionManager, SseEvent};
use lsp::LspManager;
use mcp::McpManager;
use std::sync::{Arc, Mutex};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification};
use tools::join::TaskRegistry;
use tools::ToolRegistry;
use tracing::{error, info};
use webhooks::WebhookContext;

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
    /// LSP manager shared across agents (optional).
    lsp_manager: Option<Arc<LspManager>>,
    /// Global cancellation token.
    cancel: CancellationToken,
    /// Optional webhook context for dispatching webhook events.
    webhook_ctx: Option<WebhookContext>,
    /// Shared permission manager for tool approval requests.
    permission_manager: Arc<PermissionManager>,
}

impl AgentSupervisor {
    /// Create a new supervisor.
    pub fn new(mcp_manager: Arc<McpManager>, cancel: CancellationToken) -> Self {
        Self {
            agent_map: AgentMap::new(),
            mcp_manager,
            lsp_manager: None,
            cancel,
            webhook_ctx: None,
            permission_manager: Arc::new(PermissionManager::new()),
        }
    }

    /// Create a new supervisor with LSP support.
    pub fn with_lsp(
        mcp_manager: Arc<McpManager>,
        lsp_manager: Arc<LspManager>,
        cancel: CancellationToken,
    ) -> Self {
        Self {
            agent_map: AgentMap::new(),
            mcp_manager,
            lsp_manager: Some(lsp_manager),
            cancel,
            webhook_ctx: None,
            permission_manager: Arc::new(PermissionManager::new()),
        }
    }

    /// Get the permission manager (shared across all conversations).
    pub fn permission_manager(&self) -> Arc<PermissionManager> {
        self.permission_manager.clone()
    }

    /// Set the webhook context for dispatching webhook events.
    pub fn with_webhooks(mut self, ctx: Option<WebhookContext>) -> Self {
        self.webhook_ctx = ctx;
        self
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
    async fn load_single_agent(&self, mut definition: AgentDefinition) -> Result<(), BridgeError> {
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

        // Register built-in tools — filtered by the agent's tool list if non-empty
        let builtin_tool_names: Vec<String> =
            definition.tools.iter().map(|t| t.name.clone()).collect();
        if builtin_tool_names.is_empty() {
            tools::builtin::register_builtin_tools_with_lsp(
                &mut tool_registry,
                self.lsp_manager.clone(),
            );
        } else {
            tools::builtin::register_filtered_builtin_tools_with_lsp(
                &mut tool_registry,
                &builtin_tool_names,
                self.lsp_manager.clone(),
            );
        }

        // Register skill tool if the agent has skills
        if !definition.skills.is_empty() {
            tool_registry.register(Arc::new(tools::skill_tools::SkillTool::new(
                definition.skills.clone(),
            )));
        }

        // Create task registry for tracking background subagent tasks
        let task_registry = Arc::new(TaskRegistry::new());

        // Register join tool for waiting on background tasks
        tool_registry.register(Arc::new(tools::join::JoinTool::new(task_registry.clone())));

        // Register integration tools and inject their permissions
        let control_plane_url = std::env::var("BRIDGE_CONTROL_PLANE_URL")
            .unwrap_or_else(|_| "http://localhost:3000".to_string());
        let integration_tools = tools::integration::create_integration_tools(
            &definition.integrations,
            &control_plane_url,
        );
        for (tool, permission) in &integration_tools {
            tool_registry.register(tool.clone());
            if *permission != bridge_core::permission::ToolPermission::Allow {
                definition
                    .permissions
                    .insert(tool.name().to_string(), permission.clone());
            }
        }

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
        let subagent_map = build_subagents(&definition, &integration_tools)?;

        let state = Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            task_registry,
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

        let abort_token = Arc::new(Mutex::new(CancellationToken::new()));

        let handle = ConversationHandle {
            id: conv_id.clone(),
            message_tx,
            created_at: chrono::Utc::now(),
            abort_token: abort_token.clone(),
        };

        state.conversations.insert(conv_id.clone(), handle);

        // Spawn the conversation task
        let agent = state.rig_agent.clone(); // Arc<RwLock<BridgeAgent>> — shared ref
        let metrics = state.metrics.clone();
        let cancel = state.cancel.clone();
        let def = state.definition.read().unwrap();
        let max_turns = def.config.max_turns;
        let agent_id_owned = agent_id.to_string();
        let conv_id_clone = conv_id.clone();

        // Build agent context for subagent tool
        let (notification_tx, notification_rx) = mpsc::channel::<AgentTaskNotification>(64);
        let subagent_compaction = def.config.compaction.clone();
        let runner = Arc::new(
            ConversationSubAgentRunner::new(
                state.subagents.clone(),
                state.session_store.clone(),
                notification_tx.clone(),
                cancel.clone(),
                sse_tx.clone(),
                conv_id_clone.clone(),
                0, // depth
                3, // max_depth
            )
            .with_compaction(subagent_compaction)
            .with_task_registry(state.task_registry.clone()),
        );
        let agent_context = AgentContext {
            runner,
            notification_tx,
            depth: 0,
            max_depth: 3,
        };
        let session_store = state.session_store.clone();

        let tool_names = state
            .tool_registry
            .tool_names()
            .into_iter()
            .collect::<std::collections::HashSet<String>>();
        let tool_executors = state.tool_registry.snapshot();

        // Build a no-tools retry agent for recovering from empty responses
        let retry_agent =
            Arc::new(build_agent(&def, vec![]).expect("no-tools agent build should not fail"));

        let webhook_ctx = self.webhook_ctx.clone();
        let permission_manager = self.permission_manager.clone();
        let agent_permissions = def.permissions.clone();
        let compaction_config = def.config.compaction.clone();
        let skills = def.skills.clone();
        drop(def); // release read lock before spawning

        // Build system reminder with available skills
        let system_reminder = crate::system_reminder::create_reminder_with_skills(&skills);

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
                tool_names,
                tool_executors,
                initial_history: None,
                retry_agent,
                abort_token,
                webhook_ctx,
                permission_manager,
                agent_permissions,
                compaction_config,
                system_reminder,
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

    /// Abort the current in-flight turn for a conversation.
    ///
    /// Cancels the current turn's token, causing the conversation loop to
    /// send an abort SSE event and continue waiting for the next message.
    pub fn abort_conversation(
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
        let token = handle.abort_token.lock().unwrap();
        token.cancel();

        info!(
            agent_id = agent_id,
            conversation_id = conversation_id,
            "conversation aborted"
        );
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
        mut definition: AgentDefinition,
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

        // Register built-in tools — filtered by the agent's tool list if non-empty
        let builtin_tool_names: Vec<String> =
            definition.tools.iter().map(|t| t.name.clone()).collect();
        if builtin_tool_names.is_empty() {
            tools::builtin::register_builtin_tools_with_lsp(
                &mut tool_registry,
                self.lsp_manager.clone(),
            );
        } else {
            tools::builtin::register_filtered_builtin_tools_with_lsp(
                &mut tool_registry,
                &builtin_tool_names,
                self.lsp_manager.clone(),
            );
        }

        // Register skill tool if the agent has skills
        if !definition.skills.is_empty() {
            tool_registry.register(Arc::new(tools::skill_tools::SkillTool::new(
                definition.skills.clone(),
            )));
        }

        // Create task registry for tracking background subagent tasks
        let task_registry = Arc::new(TaskRegistry::new());

        // Register join tool for waiting on background tasks
        tool_registry.register(Arc::new(tools::join::JoinTool::new(task_registry.clone())));

        // Register integration tools and inject their permissions
        let control_plane_url = std::env::var("BRIDGE_CONTROL_PLANE_URL")
            .unwrap_or_else(|_| "http://localhost:3000".to_string());
        let integration_tools = tools::integration::create_integration_tools(
            &definition.integrations,
            &control_plane_url,
        );
        for (tool, permission) in &integration_tools {
            tool_registry.register(tool.clone());
            if *permission != bridge_core::permission::ToolPermission::Allow {
                definition
                    .permissions
                    .insert(tool.name().to_string(), permission.clone());
            }
        }

        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| tool_registry.get(name))
            .collect();

        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;
        let rig_agent = build_agent(&definition, dynamic_tools)?;

        // Build subagents from definition.subagents
        let subagent_map = build_subagents(&definition, &integration_tools)?;

        Ok(Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            task_registry,
        )))
    }

    /// Hydrate pre-fetched conversations for a specific agent.
    ///
    /// Spawns each conversation as a fully active loop with pre-seeded history.
    /// Returns a list of `(conversation_id, sse_rx)` pairs for storing in
    /// the application's SSE stream map.
    pub fn hydrate_conversations(
        &self,
        agent_id: &str,
        records: Vec<ConversationRecord>,
    ) -> Vec<(String, mpsc::Receiver<SseEvent>)> {
        let mut sse_receivers = Vec::new();

        info!(
            agent_id = agent_id,
            count = records.len(),
            "hydrating conversations"
        );

        for record in records {
            match self.spawn_hydrated_conversation(agent_id, record) {
                Ok((conv_id, sse_rx)) => {
                    sse_receivers.push((conv_id, sse_rx));
                }
                Err(e) => {
                    error!(
                        agent_id = agent_id,
                        error = %e,
                        "failed to hydrate conversation"
                    );
                }
            }
        }

        sse_receivers
    }

    /// Spawn a single hydrated conversation with pre-seeded history.
    fn spawn_hydrated_conversation(
        &self,
        agent_id: &str,
        record: ConversationRecord,
    ) -> Result<(String, mpsc::Receiver<SseEvent>), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        let conv_id = record.id;
        let (message_tx, message_rx) = mpsc::channel::<Message>(32);
        let (sse_tx, sse_rx) = mpsc::channel::<SseEvent>(256);

        let abort_token = Arc::new(Mutex::new(CancellationToken::new()));

        let handle = ConversationHandle {
            id: conv_id.clone(),
            message_tx,
            created_at: record.created_at,
            abort_token: abort_token.clone(),
        };

        state.conversations.insert(conv_id.clone(), handle);

        // Convert stored messages to rig history
        let initial_history = crate::conversation::convert_messages(&record.messages);

        // Spawn the conversation task (same setup as create_conversation)
        let agent = state.rig_agent.clone(); // Arc<RwLock<BridgeAgent>> — shared ref
        let metrics = state.metrics.clone();
        let cancel = state.cancel.clone();
        let def = state.definition.read().unwrap();
        let max_turns = def.config.max_turns;
        let agent_id_owned = agent_id.to_string();
        let conv_id_clone = conv_id.clone();

        let (notification_tx, notification_rx) = mpsc::channel::<AgentTaskNotification>(64);
        let subagent_compaction = def.config.compaction.clone();
        let runner = Arc::new(
            ConversationSubAgentRunner::new(
                state.subagents.clone(),
                state.session_store.clone(),
                notification_tx.clone(),
                cancel.clone(),
                sse_tx.clone(),
                conv_id_clone.clone(),
                0,
                3,
            )
            .with_compaction(subagent_compaction)
            .with_task_registry(state.task_registry.clone()),
        );
        let agent_context = AgentContext {
            runner,
            notification_tx,
            depth: 0,
            max_depth: 3,
        };
        let session_store = state.session_store.clone();

        let tool_names = state
            .tool_registry
            .tool_names()
            .into_iter()
            .collect::<std::collections::HashSet<String>>();
        let tool_executors = state.tool_registry.snapshot();

        // Build a no-tools retry agent for recovering from empty responses
        let retry_agent =
            Arc::new(build_agent(&def, vec![]).expect("no-tools agent build should not fail"));

        let webhook_ctx = self.webhook_ctx.clone();
        let permission_manager = self.permission_manager.clone();
        let agent_permissions = def.permissions.clone();
        let compaction_config = def.config.compaction.clone();
        let skills = def.skills.clone();
        drop(def); // release read lock before spawning

        // Build system reminder with available skills
        let system_reminder = crate::system_reminder::create_reminder_with_skills(&skills);

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
                tool_names,
                tool_executors,
                initial_history: Some(initial_history),
                retry_agent,
                abort_token,
                webhook_ctx,
                permission_manager,
                agent_permissions,
                compaction_config,
                system_reminder,
            })
            .await;
        });

        info!(
            agent_id = agent_id,
            conversation_id = conv_id,
            "conversation hydrated"
        );

        Ok((conv_id, sse_rx))
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

    /// Update the API key for an agent at runtime.
    ///
    /// Rebuilds the `BridgeAgent` with the new key and swaps it in-place so
    /// both existing and new conversations pick up the rotated key on their
    /// next LLM turn. No drain, no cancellation.
    pub fn update_agent_api_key(&self, agent_id: &str, api_key: String) -> Result<(), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        // Clone current definition and update the API key
        let mut updated_def = state.definition.read().unwrap().clone();
        updated_def.provider.api_key = api_key;

        // Rebuild the agent with existing tools
        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = state
            .tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| state.tool_registry.get(name))
            .collect();
        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;
        let new_agent = build_agent(&updated_def, dynamic_tools)?;

        // Swap in the new agent — all conversations sharing this Arc see the new agent on next turn
        *state.rig_agent.write().unwrap() = new_agent;
        *state.definition.write().unwrap() = updated_def;

        info!(agent_id = agent_id, "agent API key updated");
        Ok(())
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
/// Each subagent gets built-in tools plus the parent's integration tools
/// (with the same permissions). No MCP, no agent tool to prevent
/// unbounded recursion at the configuration level.
fn build_subagents(
    definition: &AgentDefinition,
    parent_integration_tools: &[(
        Arc<dyn tools::ToolExecutor>,
        bridge_core::permission::ToolPermission,
    )],
) -> Result<Arc<DashMap<String, SubAgentEntry>>, BridgeError> {
    let subagent_map = Arc::new(DashMap::new());

    for subagent_def in &definition.subagents {
        let mut sub_registry = ToolRegistry::new();
        tools::builtin::register_builtin_tools_for_subagent(&mut sub_registry);

        // Inherit parent's integration tools with same permissions
        for (tool, _) in parent_integration_tools {
            sub_registry.register(tool.clone());
        }

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
