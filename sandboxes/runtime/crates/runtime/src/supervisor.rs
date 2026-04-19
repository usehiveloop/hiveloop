use bridge_core::conversation::{ContentBlock, ConversationRecord, Message, Role};
use bridge_core::event::BridgeEvent;
use bridge_core::mcp::{McpServerDefinition, McpTransport};
use bridge_core::{AgentDefinition, AgentSummary, BridgeError, MetricsSnapshot};
use dashmap::DashMap;
use llm::{adapt_tools, build_agent, DynamicTool, PermissionManager};
use lsp::LspManager;
use mcp::McpManager;
use std::collections::HashMap;
use std::sync::Arc;
use storage::{StorageBackend, StorageHandle};
use tokio::sync::mpsc;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification};
use tools::registry::ToolExecutor;
use tools::ToolRegistry;
use tracing::{error, info};
use webhooks::EventBus;

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
    /// Optional event bus for unified event delivery (SSE, WebSocket, webhooks, persistence).
    event_bus: Option<Arc<EventBus>>,
    /// Shared permission manager for tool approval requests.
    permission_manager: Arc<PermissionManager>,
    /// Limits total concurrent conversations across all agents.
    conversation_semaphore: Option<Arc<tokio::sync::Semaphore>>,
    /// Limits total concurrent outbound LLM API calls.
    llm_semaphore: Arc<tokio::sync::Semaphore>,
    /// Optional non-blocking persistence handle.
    storage: Option<StorageHandle>,
    /// Optional persistence backend for startup/restore reads.
    storage_backend: Option<Arc<dyn StorageBackend>>,
    /// When true, scan the working directory for skills from .claude/, .cursor/, etc.
    skill_discovery_enabled: bool,
    /// Working directory for skill discovery. Defaults to `std::env::current_dir()`.
    skill_discovery_dir: Option<String>,
    /// When true, API clients may attach `stdio` MCP servers per conversation.
    /// Default: false (only `streamable_http` accepted from the API).
    allow_stdio_mcp_from_api: bool,
    /// When true, inject environment system reminder (installed tools, resource usage).
    standalone_agent: bool,
}

/// Default maximum concurrent LLM calls when not configured.
const DEFAULT_MAX_CONCURRENT_LLM_CALLS: usize = 500;

impl AgentSupervisor {
    /// Create a new supervisor.
    pub fn new(mcp_manager: Arc<McpManager>, cancel: CancellationToken) -> Self {
        Self {
            agent_map: AgentMap::new(),
            mcp_manager,
            lsp_manager: None,
            cancel,
            event_bus: None,
            permission_manager: Arc::new(PermissionManager::new()),
            conversation_semaphore: None,
            llm_semaphore: Arc::new(tokio::sync::Semaphore::new(
                DEFAULT_MAX_CONCURRENT_LLM_CALLS,
            )),
            storage: None,
            storage_backend: None,
            skill_discovery_enabled: false,
            skill_discovery_dir: None,
            allow_stdio_mcp_from_api: false,
            standalone_agent: false,
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
            event_bus: None,
            permission_manager: Arc::new(PermissionManager::new()),
            conversation_semaphore: None,
            llm_semaphore: Arc::new(tokio::sync::Semaphore::new(
                DEFAULT_MAX_CONCURRENT_LLM_CALLS,
            )),
            storage: None,
            storage_backend: None,
            skill_discovery_enabled: false,
            skill_discovery_dir: None,
            allow_stdio_mcp_from_api: false,
            standalone_agent: false,
        }
    }

    /// Attach an optional non-blocking persistence handle.
    pub fn with_storage(mut self, storage: Option<StorageHandle>) -> Self {
        self.storage = storage;
        self
    }

    /// Attach an optional persistence backend for restore reads.
    pub fn with_storage_backend(
        mut self,
        storage_backend: Option<Arc<dyn StorageBackend>>,
    ) -> Self {
        self.storage_backend = storage_backend;
        self
    }

    /// Configure admission control from runtime config.
    pub fn with_capacity_limits(mut self, config: &bridge_core::RuntimeConfig) -> Self {
        if let Some(max_convs) = config.max_concurrent_conversations {
            self.conversation_semaphore = Some(Arc::new(tokio::sync::Semaphore::new(max_convs)));
        }
        let max_llm = config
            .max_concurrent_llm_calls
            .unwrap_or(DEFAULT_MAX_CONCURRENT_LLM_CALLS);
        self.llm_semaphore = Arc::new(tokio::sync::Semaphore::new(max_llm));
        self.skill_discovery_enabled = config.skill_discovery_enabled;
        self.skill_discovery_dir = config.skill_discovery_dir.clone();
        self.allow_stdio_mcp_from_api = config.allow_stdio_mcp_from_api;
        self.standalone_agent = config.standalone_agent;
        self
    }

    /// Configure skill discovery from working directory.
    pub fn with_skill_discovery(mut self, enabled: bool, dir: Option<String>) -> Self {
        self.skill_discovery_enabled = enabled;
        self.skill_discovery_dir = dir;
        self
    }

    /// Get a reference to the LLM semaphore (for passing to conversations).
    pub fn llm_semaphore(&self) -> Arc<tokio::sync::Semaphore> {
        self.llm_semaphore.clone()
    }

    /// Get the permission manager (shared across all conversations).
    pub fn permission_manager(&self) -> Arc<PermissionManager> {
        self.permission_manager.clone()
    }

    /// Set the event bus for unified event delivery.
    pub fn with_event_bus(mut self, bus: Option<Arc<EventBus>>) -> Self {
        self.event_bus = bus;
        self
    }

    /// Load a set of agent definitions, building their runtime state.
    pub async fn load_agents(&self, definitions: Vec<AgentDefinition>) -> Result<(), BridgeError> {
        for def in definitions {
            let agent_id = def.id.clone();

            if let Some(existing) = self.agent_map.get(&agent_id) {
                let existing_def = existing.definition.read().await.clone();
                if definitions_equivalent(&existing_def, &def) {
                    continue;
                }

                match self.load_single_agent_state(def).await {
                    Ok(new_state) => {
                        let persisted_def = new_state.definition.read().await.clone();
                        let timeout = std::time::Duration::from_secs(60);
                        if let Err(e) =
                            drain_and_replace(&self.agent_map, &agent_id, new_state, timeout).await
                        {
                            error!(agent_id = agent_id, error = %e, "failed to replace existing agent");
                        } else if let Some(storage) = &self.storage {
                            storage.save_agent(persisted_def);
                        }
                    }
                    Err(e) => {
                        error!(agent_id = agent_id, error = %e, "failed to rebuild existing agent");
                    }
                }
            } else if let Err(e) = self.load_single_agent(def).await {
                error!(error = %e, "failed to load agent");
            }
        }
        Ok(())
    }

    /// Build and load a single agent.
    async fn load_single_agent(&self, mut definition: AgentDefinition) -> Result<(), BridgeError> {
        let agent_id = definition.id.clone();

        // Connect to MCP servers
        self.mcp_manager
            .connect_agent(&agent_id, &definition.mcp_servers)
            .await?;

        // Extract tool allow-list early — used for both MCP and built-in tool filtering
        let builtin_tool_names: Vec<String> =
            definition.tools.iter().map(|t| t.name.clone()).collect();

        // Build tool registry with MCP tools (each connection's tools bridged to its own connection)
        let mut tool_registry = ToolRegistry::new();
        let mut mcp_server_tools: std::collections::HashMap<String, Vec<String>> =
            std::collections::HashMap::new();
        let connections = self.mcp_manager.get_agent_connections(&agent_id);
        for conn in &connections {
            if let Ok(tools) = conn.list_tools().await {
                let server_name = conn.server_name().to_string();
                let tool_names: Vec<String> = tools.iter().map(|t| t.name.clone()).collect();
                mcp_server_tools.insert(server_name, tool_names);
                let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                for tool in bridged {
                    tool_registry.register(tool);
                }
            }
        }

        // Register built-in tools — filtered by the agent's tool list if non-empty
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

        // Merge control-plane skills with locally discovered skills
        let all_skills = self
            .merge_with_discovered_skills(definition.skills.clone())
            .await;
        if !all_skills.is_empty() {
            let base_dir = self.resolve_working_dir();
            tools::skill_files::write_skill_files(&all_skills, &base_dir).await;
            tool_registry.register(Arc::new(tools::skill_tools::SkillTool::with_base_dir(
                all_skills, base_dir,
            )));
        }

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

        // Remove disabled tools — takes priority over everything else.
        // The LLM will never see these tools.
        for name in &definition.config.disabled_tools {
            tool_registry.remove(name);
        }

        // Collect all tool executors for the LLM agent
        // Remove disabled tools — takes priority over everything else.
        for name in &definition.config.disabled_tools {
            tool_registry.remove(name);
        }

        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| tool_registry.get(name))
            .collect();

        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;

        // Build the rig agent
        let rig_agent = build_agent(&definition, dynamic_tools)?;

        // Build subagents from definition.subagents
        let subagent_map = build_subagents(
            &definition,
            &integration_tools,
            &self.mcp_manager,
            &self.resolve_working_dir(),
        )
        .await?;

        // Inject the parent agent as "__self__" for self-delegation
        subagent_map.insert(
            tools::self_agent::SELF_AGENT_NAME.to_string(),
            SubAgentEntry {
                name: tools::self_agent::SELF_AGENT_NAME.to_string(),
                description: "Self-delegation agent".to_string(),
                agent: Arc::new(rig_agent.clone()),
                registered_tools: vec![], // uses parent's tool registry
            },
        );

        let persisted_definition = definition.clone();

        let state = Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            self.storage.clone(),
            mcp_server_tools,
        ));

        if let Some(storage_backend) = &self.storage_backend {
            restore_agent_sessions(storage_backend.as_ref(), &agent_id, &state.session_store).await;
        }

        self.agent_map.insert(agent_id.clone(), state);

        if let Some(storage) = &self.storage {
            storage.save_agent(persisted_definition);
        }

        info!(agent_id = agent_id, "agent loaded");
        Ok(())
    }

    /// Get an agent state by ID.
    pub fn get_agent(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.agent_map.get(agent_id)
    }

    /// List all loaded agents.
    pub async fn list_agents(&self) -> Vec<AgentSummary> {
        self.agent_map.list().await
    }

    /// Return all agent states for enriched API responses.
    pub fn list_agent_states(&self) -> Vec<Arc<AgentState>> {
        self.agent_map.list_states()
    }

    /// Resolve the working directory for skill discovery and skill file materialization.
    fn resolve_working_dir(&self) -> std::path::PathBuf {
        self.skill_discovery_dir
            .as_deref()
            .map(std::path::PathBuf::from)
            .unwrap_or_else(|| std::env::current_dir().unwrap_or_default())
    }

    /// Merge control-plane skills with locally discovered skills.
    ///
    /// Control-plane skills always take precedence. Local skills are only added
    /// if discovery is enabled and no control-plane skill shares the same id.
    async fn merge_with_discovered_skills(
        &self,
        mut cp_skills: Vec<bridge_core::SkillDefinition>,
    ) -> Vec<bridge_core::SkillDefinition> {
        if !self.skill_discovery_enabled {
            return cp_skills;
        }

        let dir = self.resolve_working_dir();

        let local_skills = crate::skill_discovery::discover_skills(&dir).await;

        if local_skills.is_empty() {
            return cp_skills;
        }

        let cp_ids: std::collections::HashSet<String> =
            cp_skills.iter().map(|s| s.id.clone()).collect();

        for skill in local_skills {
            if !cp_ids.contains(&skill.id) {
                cp_skills.push(skill);
            }
        }

        cp_skills
    }

    /// Create a new conversation for an agent.
    ///
    /// Returns the conversation ID and a BridgeEvent receiver for streaming responses.
    /// Returns `CapacityExhausted` if global or per-agent conversation limits are reached.
    #[allow(clippy::too_many_arguments)]
    pub async fn create_conversation(
        &self,
        agent_id: &str,
        filter_tool_names: Option<Vec<String>>,
        filter_mcp_server_names: Option<Vec<String>>,
        api_key_override: Option<String>,
        subagent_api_key_overrides: Option<HashMap<String, String>>,
        provider_override: Option<bridge_core::ProviderConfig>,
        per_conversation_mcp_servers: Option<Vec<McpServerDefinition>>,
    ) -> Result<(String, mpsc::Receiver<BridgeEvent>), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        // --- Admission control: global conversation limit ---
        let conversation_permit = match &self.conversation_semaphore {
            Some(sem) => match sem.clone().try_acquire_owned() {
                Ok(permit) => Some(permit),
                Err(_) => {
                    return Err(BridgeError::CapacityExhausted(
                        "global max concurrent conversations reached".to_string(),
                    ));
                }
            },
            None => None,
        };

        // --- Admission control: per-agent conversation limit ---
        {
            let def = state.definition.read().await;
            if let Some(max) = def.config.max_concurrent_conversations {
                if state.conversations.len() >= max as usize {
                    return Err(BridgeError::CapacityExhausted(format!(
                        "agent {} at max concurrent conversations ({})",
                        agent_id, max
                    )));
                }
            }
        }

        // --- Validate per-conversation MCP servers (before any resource acquisition) ---
        if let Some(ref servers) = per_conversation_mcp_servers {
            let mut seen = std::collections::HashSet::new();
            for server in servers {
                if server.name.trim().is_empty() {
                    return Err(BridgeError::InvalidRequest(
                        "mcp_servers: server name cannot be empty".to_string(),
                    ));
                }
                if !seen.insert(server.name.clone()) {
                    return Err(BridgeError::InvalidRequest(format!(
                        "mcp_servers: duplicate server name '{}'",
                        server.name
                    )));
                }
                if matches!(server.transport, McpTransport::Stdio { .. })
                    && !self.allow_stdio_mcp_from_api
                {
                    return Err(BridgeError::InvalidRequest(format!(
                        "mcp_servers: stdio transport not allowed from API (server '{}'); \
                         enable allow_stdio_mcp_from_api in runtime config to permit it",
                        server.name
                    )));
                }
            }
        }

        // --- Validate API key overrides ---
        if let Some(ref key) = api_key_override {
            if key.trim().is_empty() {
                return Err(BridgeError::InvalidRequest(
                    "api_key cannot be empty".to_string(),
                ));
            }
        }
        if let Some(ref overrides) = subagent_api_key_overrides {
            for (name, key) in overrides {
                if key.trim().is_empty() {
                    return Err(BridgeError::InvalidRequest(format!(
                        "subagent_api_keys: key for '{}' cannot be empty",
                        name
                    )));
                }
                if !state.subagents.contains_key(name) {
                    return Err(BridgeError::InvalidRequest(format!(
                        "subagent_api_keys: unknown subagent '{}'",
                        name
                    )));
                }
            }
        }

        let conv_id = uuid::Uuid::new_v4().to_string();
        let (message_tx, message_rx) = mpsc::channel::<Message>(32);

        // Register an SSE stream for this conversation on the event bus.
        let event_bus = self
            .event_bus
            .clone()
            .expect("event_bus must be set before creating conversations");
        let sse_rx = event_bus.register_sse_stream(conv_id.clone(), 256);

        let abort_token = Arc::new(Mutex::new(CancellationToken::new()));

        let handle = ConversationHandle {
            id: conv_id.clone(),
            message_tx,
            created_at: chrono::Utc::now(),
            abort_token: abort_token.clone(),
        };
        let created_at = handle.created_at;

        state.conversations.insert(conv_id.clone(), handle);

        if let Some(storage) = &self.storage {
            storage.create_conversation(agent_id.to_string(), conv_id.clone(), None, created_at);
        }

        // Spawn the conversation task
        let metrics = state.metrics.clone();
        let cancel = state.cancel.clone();
        let def = state.definition.read().await;
        let max_turns = def.config.max_turns;
        let agent_id_owned = agent_id.to_string();
        let conv_id_clone = conv_id.clone();

        // Build agent context for subagent tool
        let (notification_tx, notification_rx) = mpsc::channel::<AgentTaskNotification>(64);
        let subagent_compaction = def.config.compaction.clone();
        // Create task budget for this conversation (before runner so runner can share it)
        let max_tasks = def.config.max_tasks_per_conversation.unwrap_or(50) as usize;
        let task_budget = Arc::new(tools::TaskBudget::new(max_tasks));

        // Build conversation-scoped subagent map when API key overrides are provided.
        let conversation_subagents = if let Some(ref overrides) = subagent_api_key_overrides {
            let scoped_map = Arc::new(DashMap::new());
            let control_plane_url = std::env::var("BRIDGE_CONTROL_PLANE_URL")
                .unwrap_or_else(|_| "http://localhost:3000".to_string());
            let integration_tools =
                tools::integration::create_integration_tools(&def.integrations, &control_plane_url);

            for entry in state.subagents.iter() {
                let name = entry.key().clone();
                let original = entry.value();
                if let Some(override_key) = overrides.get(&name) {
                    // Find the subagent definition and rebuild with overridden key
                    if let Some(sub_def) = def.subagents.iter().find(|s| s.name == name) {
                        let mut overridden_def = sub_def.clone();
                        overridden_def.provider.api_key = override_key.clone();

                        let mut sub_registry = tools::ToolRegistry::new();
                        tools::builtin::register_builtin_tools_for_subagent(&mut sub_registry);
                        for (tool, _) in &integration_tools {
                            sub_registry.register(tool.clone());
                        }
                        let sub_executors: Vec<Arc<dyn tools::ToolExecutor>> = sub_registry
                            .list()
                            .iter()
                            .filter_map(|(n, _)| sub_registry.get(n))
                            .collect();
                        let sub_dynamic = adapt_tools(sub_executors)?;
                        let sub_agent = build_agent(&overridden_def, sub_dynamic)?;

                        scoped_map.insert(
                            name,
                            SubAgentEntry {
                                name: original.name.clone(),
                                description: original.description.clone(),
                                agent: Arc::new(sub_agent),
                                registered_tools: original.registered_tools.clone(),
                            },
                        );
                    } else {
                        // Subagent name exists in runtime but not in definition (e.g. __self__)
                        scoped_map.insert(
                            name,
                            SubAgentEntry {
                                name: original.name.clone(),
                                description: original.description.clone(),
                                agent: original.agent.clone(),
                                registered_tools: original.registered_tools.clone(),
                            },
                        );
                    }
                } else {
                    // No override — share original entry
                    scoped_map.insert(
                        name,
                        SubAgentEntry {
                            name: original.name.clone(),
                            description: original.description.clone(),
                            agent: original.agent.clone(),
                            registered_tools: original.registered_tools.clone(),
                        },
                    );
                }
            }
            scoped_map
        } else {
            state.subagents.clone()
        };

        let runner = Arc::new(
            ConversationSubAgentRunner::new(
                conversation_subagents,
                state.session_store.clone(),
                notification_tx.clone(),
                cancel.clone(),
                event_bus.clone(),
                conv_id_clone.clone(),
                0, // depth
                3, // max_depth
                metrics.clone(),
            )
            .with_compaction(subagent_compaction)
            .with_task_budget(task_budget.clone())
            .with_agent_id(agent_id.to_string()),
        );

        let agent_context = AgentContext {
            runner,
            notification_tx,
            depth: 0,
            max_depth: 3,
            task_budget,
        };
        let session_store = state.session_store.clone();

        let has_filters = filter_tool_names.is_some()
            || filter_mcp_server_names.is_some()
            || api_key_override.is_some()
            || provider_override.is_some();

        let mut tool_names: std::collections::HashSet<String> =
            state.tool_registry.tool_names().into_iter().collect();
        let mut tool_executors = state.tool_registry.snapshot();

        filter_conversation_tools(
            agent_id,
            &mut tool_names,
            &mut tool_executors,
            &state.mcp_server_tools,
            filter_mcp_server_names.as_ref(),
            filter_tool_names.as_ref(),
        )?;

        let conv_event_bus = event_bus.clone();
        let permission_manager = self.permission_manager.clone();
        let agent_permissions = def.permissions.clone();
        let immortal_config = def.config.immortal.clone();
        // When immortal mode is active, compaction is disabled
        let compaction_config = if immortal_config.is_some() {
            None
        } else {
            def.config.compaction.clone()
        };
        let tool_calls_only = def.config.tool_calls_only.unwrap_or(false);
        // Get skills from the registered SkillTool (includes local discoveries)
        let skills = state
            .tool_registry
            .get("skill")
            .and_then(|t| {
                t.as_any()
                    .downcast_ref::<tools::skill_tools::SkillTool>()
                    .map(|st| st.skills().clone())
            })
            .unwrap_or_default();
        let llm_semaphore = self.llm_semaphore.clone();
        let storage = self.storage.clone();
        let model_name = def.provider.model.clone();

        // Create journal state for immortal conversations
        let journal_state = immortal_config.as_ref().map(|_| {
            Arc::new(tools::journal::JournalState::new(
                conv_id.clone(),
                storage.clone(),
            ))
        });

        // Register journal tools if immortal mode is active
        if let Some(ref js) = journal_state {
            let write_tool = Arc::new(tools::journal::JournalWriteTool::new(js.clone()));
            tool_names.insert(write_tool.name().to_string());
            tool_executors.insert(
                write_tool.name().to_string(),
                write_tool.clone() as Arc<dyn tools::ToolExecutor>,
            );

            let read_tool = Arc::new(tools::journal::JournalReadTool::new(js.clone()));
            tool_names.insert(read_tool.name().to_string());
            tool_executors.insert(
                read_tool.name().to_string(),
                read_tool.clone() as Arc<dyn tools::ToolExecutor>,
            );
        }

        // Connect per-conversation MCP servers and merge their tools into the
        // per-conversation executor map. Scoped in McpManager under the conversation
        // UUID, which cannot collide with any agent ID. On any error in this block
        // the partial connection state and the conversation handle are unwound
        // before returning.
        let per_conv_mcp_scope: Option<String> = match per_conversation_mcp_servers {
            Some(ref servers) if !servers.is_empty() => {
                let scope_id = conv_id.clone();
                if let Err(e) = self.mcp_manager.connect_agent(&scope_id, servers).await {
                    self.mcp_manager.disconnect_agent(&scope_id).await;
                    state.conversations.remove(&conv_id);
                    return Err(e);
                }

                let expected: std::collections::HashSet<&str> =
                    servers.iter().map(|s| s.name.as_str()).collect();
                let connected: Vec<Arc<mcp::McpConnection>> =
                    self.mcp_manager.get_agent_connections(&scope_id);
                let connected_names: std::collections::HashSet<&str> =
                    connected.iter().map(|c| c.server_name()).collect();
                for name in &expected {
                    if !connected_names.contains(name) {
                        self.mcp_manager.disconnect_agent(&scope_id).await;
                        state.conversations.remove(&conv_id);
                        return Err(BridgeError::InvalidRequest(format!(
                            "mcp_servers: failed to connect to '{}'",
                            name
                        )));
                    }
                }

                for conn in connected {
                    let server_name = conn.server_name().to_string();
                    let tools = match conn.list_tools().await {
                        Ok(t) => t,
                        Err(e) => {
                            self.mcp_manager.disconnect_agent(&scope_id).await;
                            state.conversations.remove(&conv_id);
                            return Err(BridgeError::InvalidRequest(format!(
                                "mcp_servers: failed to list tools from '{}': {}",
                                server_name, e
                            )));
                        }
                    };
                    let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                    for tool in bridged {
                        let tool_name = tool.name().to_string();
                        if tool_names.contains(&tool_name) {
                            self.mcp_manager.disconnect_agent(&scope_id).await;
                            state.conversations.remove(&conv_id);
                            return Err(BridgeError::InvalidRequest(format!(
                                "mcp_servers: tool '{}' from server '{}' collides with an \
                                 existing agent tool",
                                tool_name, server_name
                            )));
                        }
                        tool_names.insert(tool_name.clone());
                        tool_executors.insert(tool_name, tool);
                    }
                }

                Some(scope_id)
            }
            _ => None,
        };

        // Build a scoped definition when provider or API key overrides are present.
        // Provider override takes precedence (replaces the entire provider config).
        // API key override only swaps the key on the existing provider.
        let scoped_def: Option<AgentDefinition> = if let Some(provider) = provider_override {
            let mut d = (*def).clone();
            d.provider = provider;
            Some(d)
        } else {
            api_key_override.map(|key| {
                let mut d = (*def).clone();
                d.provider.api_key = key;
                d
            })
        };
        let effective_def: &AgentDefinition = scoped_def.as_ref().unwrap_or(&def);

        // Build a conversation-scoped agent when filters are active, when immortal
        // mode adds per-conversation tools (journal_write), or when per-conversation
        // MCP servers were attached. When unfiltered and no extra tools, share the
        // agent-wide instance.
        let needs_scoped_agent =
            has_filters || journal_state.is_some() || per_conv_mcp_scope.is_some();
        let conversation_agent = if needs_scoped_agent {
            let scoped_executors: Vec<Arc<dyn tools::ToolExecutor>> =
                tool_executors.values().cloned().collect();
            let scoped_dynamic = adapt_tools(scoped_executors)?;
            Arc::new(tokio::sync::RwLock::new(build_agent(
                effective_def,
                scoped_dynamic,
            )?))
        } else {
            state.rig_agent.clone()
        };

        // Build a no-tools retry agent for recovering from empty responses.
        // Uses effective_def so it shares the API key override when present.
        let retry_agent = Arc::new(
            build_agent(effective_def, vec![]).expect("no-tools agent build should not fail"),
        );
        drop(def);

        // Check if todo tools are enabled
        let has_todo_tools = tool_names.contains("todoread") && tool_names.contains("todowrite");

        // Extract subagent names and descriptions, filtering out __self__
        let subagent_list: Vec<(String, String)> = state
            .subagents
            .iter()
            .filter(|entry| entry.key() != tools::self_agent::SELF_AGENT_NAME)
            .map(|entry| {
                (
                    entry.value().name.clone(),
                    entry.value().description.clone(),
                )
            })
            .collect();

        // Build system reminder with available skills, sub-agents, and optionally todos
        let system_reminder = if has_todo_tools {
            // Try to get todo state from the tool registry
            let todos = get_todos_from_registry(&tool_executors).await;
            crate::system_reminder::create_reminder_with_skills_todos_and_date(
                &skills,
                &subagent_list,
                todos.as_deref(),
                chrono::Utc::now(),
            )
        } else {
            crate::system_reminder::create_reminder_with_skills(&skills, &subagent_list)
        };

        let conv_metrics = Arc::new(bridge_core::metrics::ConversationMetrics::new(
            conv_id.clone(),
            agent_id.to_string(),
            model_name.clone(),
        ));

        let cleanup_mcp_manager = self.mcp_manager.clone();
        let standalone_agent = self.standalone_agent;

        state.tracker.spawn(async move {
            // Hold the conversation permit for the lifetime of the conversation.
            // When the conversation ends, the permit is dropped, freeing a slot.
            let _conversation_permit = conversation_permit;
            let ping_state = get_ping_state_from_registry(&tool_executors);

            run_conversation(ConversationParams {
                agent_id: agent_id_owned,
                conversation_id: conv_id_clone,
                agent: conversation_agent,
                message_rx,
                event_bus: conv_event_bus,
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
                permission_manager,
                agent_permissions,
                compaction_config,
                system_reminder,
                conversation_date: chrono::Utc::now(),
                llm_semaphore,
                initial_persisted_messages: None,
                storage,
                tool_calls_only,
                conversation_metrics: conv_metrics,
                immortal_config,
                journal_state,
                per_conversation_mcp_scope: per_conv_mcp_scope,
                mcp_manager: Some(cleanup_mcp_manager),
                standalone_agent,
                ping_state,
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
                    let persisted_def = new_state.definition.read().await.clone();
                    let timeout = std::time::Duration::from_secs(60);
                    if let Err(e) =
                        drain_and_replace(&self.agent_map, &agent_id, new_state, timeout).await
                    {
                        error!(agent_id = agent_id, error = %e, "failed to drain and replace agent");
                    } else if let Some(storage) = &self.storage {
                        storage.save_agent(persisted_def);
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
                // Clean up materialized skill files and subagent MCP connections.
                {
                    let base_dir = self.resolve_working_dir();
                    let def = state.definition.read().await;

                    // Collect skill IDs from parent and all subagents for cleanup.
                    let mut skill_ids: Vec<&str> =
                        def.skills.iter().map(|s| s.id.as_str()).collect();
                    for subagent_def in &def.subagents {
                        for skill in &subagent_def.skills {
                            skill_ids.push(skill.id.as_str());
                        }
                        let subagent_mcp_id =
                            format!("{}::subagent::{}", agent_id, subagent_def.name);
                        self.mcp_manager.disconnect_agent(&subagent_mcp_id).await;
                    }
                    tools::skill_files::cleanup_skill_files(&skill_ids, &base_dir).await;
                }

                state.cancel.cancel();
                state.tracker.close();
                state.tracker.wait().await;
                self.mcp_manager.disconnect_agent(&agent_id).await;
                if let Some(storage) = &self.storage {
                    storage.delete_agent(agent_id.clone());
                }
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

        self.mcp_manager
            .connect_agent(&agent_id, &definition.mcp_servers)
            .await?;

        // Extract tool allow-list early — used for both MCP and built-in tool filtering
        let builtin_tool_names: Vec<String> =
            definition.tools.iter().map(|t| t.name.clone()).collect();

        let mut tool_registry = ToolRegistry::new();
        let mut mcp_server_tools: std::collections::HashMap<String, Vec<String>> =
            std::collections::HashMap::new();
        let connections = self.mcp_manager.get_agent_connections(&agent_id);
        for conn in &connections {
            if let Ok(tools) = conn.list_tools().await {
                let server_name = conn.server_name().to_string();
                let tool_names: Vec<String> = tools.iter().map(|t| t.name.clone()).collect();
                mcp_server_tools.insert(server_name, tool_names);
                let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                for tool in bridged {
                    tool_registry.register(tool);
                }
            }
        }

        // Register built-in tools — filtered by the agent's tool list if non-empty
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

        // Merge control-plane skills with locally discovered skills
        let all_skills = self
            .merge_with_discovered_skills(definition.skills.clone())
            .await;
        if !all_skills.is_empty() {
            let base_dir = self.resolve_working_dir();

            // Clean up old skill files and subagent MCP connections when updating an existing agent.
            if let Some(old_state) = self.agent_map.get(&agent_id) {
                let old_def = old_state.definition.read().await;
                let mut old_skill_ids: Vec<&str> =
                    old_def.skills.iter().map(|s| s.id.as_str()).collect();
                for subagent_def in &old_def.subagents {
                    for skill in &subagent_def.skills {
                        old_skill_ids.push(skill.id.as_str());
                    }
                    let subagent_mcp_id = format!("{}::subagent::{}", agent_id, subagent_def.name);
                    self.mcp_manager.disconnect_agent(&subagent_mcp_id).await;
                }
                tools::skill_files::cleanup_skill_files(&old_skill_ids, &base_dir).await;
            }

            tools::skill_files::write_skill_files(&all_skills, &base_dir).await;
            tool_registry.register(Arc::new(tools::skill_tools::SkillTool::with_base_dir(
                all_skills, base_dir,
            )));
        }

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

        // Remove disabled tools — takes priority over everything else.
        for name in &definition.config.disabled_tools {
            tool_registry.remove(name);
        }

        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| tool_registry.get(name))
            .collect();

        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;
        let rig_agent = build_agent(&definition, dynamic_tools)?;

        // Build subagents from definition.subagents
        let subagent_map = build_subagents(
            &definition,
            &integration_tools,
            &self.mcp_manager,
            &self.resolve_working_dir(),
        )
        .await?;

        // Inject the parent agent as "__self__" for self-delegation
        subagent_map.insert(
            tools::self_agent::SELF_AGENT_NAME.to_string(),
            SubAgentEntry {
                name: tools::self_agent::SELF_AGENT_NAME.to_string(),
                description: "Self-delegation agent".to_string(),
                agent: Arc::new(rig_agent.clone()),
                registered_tools: vec![], // uses parent's tool registry
            },
        );

        let state = Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            self.storage.clone(),
            mcp_server_tools,
        ));

        if let Some(storage_backend) = &self.storage_backend {
            restore_agent_sessions(storage_backend.as_ref(), &agent_id, &state.session_store).await;
        }

        Ok(state)
    }

    /// Hydrate pre-fetched conversations for a specific agent.
    ///
    /// Spawns each conversation as a fully active loop with pre-seeded history.
    /// Returns a list of `(conversation_id, sse_rx)` pairs for storing in
    /// the application's SSE stream map.
    pub async fn hydrate_conversations(
        &self,
        agent_id: &str,
        records: Vec<ConversationRecord>,
    ) -> Vec<(String, mpsc::Receiver<BridgeEvent>)> {
        let mut sse_receivers = Vec::new();

        info!(
            agent_id = agent_id,
            count = records.len(),
            "hydrating conversations"
        );

        for record in records {
            match self.spawn_hydrated_conversation(agent_id, record).await {
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
    async fn spawn_hydrated_conversation(
        &self,
        agent_id: &str,
        record: ConversationRecord,
    ) -> Result<(String, mpsc::Receiver<BridgeEvent>), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        let conv_id = record.id;
        let (message_tx, message_rx) = mpsc::channel::<Message>(32);

        // Register an SSE stream for this conversation on the event bus.
        let event_bus = self
            .event_bus
            .clone()
            .expect("event_bus must be set before hydrating conversations");
        let sse_rx = event_bus.register_sse_stream(conv_id.clone(), 256);

        let abort_token = Arc::new(Mutex::new(CancellationToken::new()));

        let handle = ConversationHandle {
            id: conv_id.clone(),
            message_tx,
            created_at: record.created_at,
            abort_token: abort_token.clone(),
        };

        state.conversations.insert(conv_id.clone(), handle);

        // Convert stored messages to rig history
        let initial_persisted_messages =
            crate::conversation::normalize_messages_for_persistence(&record.messages);
        let initial_history = crate::conversation::convert_messages(&initial_persisted_messages);

        // Spawn the conversation task (same setup as create_conversation)
        let agent = state.rig_agent.clone(); // Arc<RwLock<BridgeAgent>> — shared ref
        let metrics = state.metrics.clone();
        let cancel = state.cancel.clone();
        let def = state.definition.read().await;
        let max_turns = def.config.max_turns;
        let agent_id_owned = agent_id.to_string();
        let conv_id_clone = conv_id.clone();

        let (notification_tx, notification_rx) = mpsc::channel::<AgentTaskNotification>(64);
        let subagent_compaction = def.config.compaction.clone();
        // Create task budget for this conversation
        let max_tasks = def.config.max_tasks_per_conversation.unwrap_or(50) as usize;
        let task_budget = Arc::new(tools::TaskBudget::new(max_tasks));

        let runner = Arc::new(
            ConversationSubAgentRunner::new(
                state.subagents.clone(),
                state.session_store.clone(),
                notification_tx.clone(),
                cancel.clone(),
                event_bus.clone(),
                conv_id_clone.clone(),
                0,
                3,
                metrics.clone(),
            )
            .with_compaction(subagent_compaction)
            .with_task_budget(task_budget.clone())
            .with_agent_id(agent_id.to_string()),
        );

        let agent_context = AgentContext {
            runner,
            notification_tx,
            depth: 0,
            max_depth: 3,
            task_budget,
        };
        let session_store = state.session_store.clone();

        let mut tool_names = state
            .tool_registry
            .tool_names()
            .into_iter()
            .collect::<std::collections::HashSet<String>>();
        let mut tool_executors = state.tool_registry.snapshot();

        // Build a no-tools retry agent for recovering from empty responses
        let retry_agent =
            Arc::new(build_agent(&def, vec![]).expect("no-tools agent build should not fail"));

        let conv_event_bus = event_bus.clone();
        let permission_manager = self.permission_manager.clone();
        let agent_permissions = def.permissions.clone();
        let immortal_config = def.config.immortal.clone();
        let compaction_config = if immortal_config.is_some() {
            None
        } else {
            def.config.compaction.clone()
        };
        let tool_calls_only = def.config.tool_calls_only.unwrap_or(false);
        // Get skills from the registered SkillTool (includes local discoveries)
        let skills = state
            .tool_registry
            .get("skill")
            .and_then(|t| {
                t.as_any()
                    .downcast_ref::<tools::skill_tools::SkillTool>()
                    .map(|st| st.skills().clone())
            })
            .unwrap_or_default();
        let llm_semaphore = self.llm_semaphore.clone();
        let storage = self.storage.clone();
        let model_name = def.provider.model.clone();

        // Create journal state for immortal conversations (hydration path)
        // TODO: Load journal entries from storage when StorageBackend is available here
        let journal_state = immortal_config.as_ref().map(|_| {
            Arc::new(tools::journal::JournalState::new(
                conv_id.clone(),
                storage.clone(),
            ))
        });

        // Register journal tools if immortal mode is active
        if let Some(ref js) = journal_state {
            let write_tool = Arc::new(tools::journal::JournalWriteTool::new(js.clone()));
            tool_names.insert(write_tool.name().to_string());
            tool_executors.insert(
                write_tool.name().to_string(),
                write_tool.clone() as Arc<dyn tools::ToolExecutor>,
            );

            let read_tool = Arc::new(tools::journal::JournalReadTool::new(js.clone()));
            tool_names.insert(read_tool.name().to_string());
            tool_executors.insert(
                read_tool.name().to_string(),
                read_tool.clone() as Arc<dyn tools::ToolExecutor>,
            );
        }

        // Rebuild agent with journal tool when immortal mode is active
        let agent = if journal_state.is_some() {
            let scoped_executors: Vec<Arc<dyn tools::ToolExecutor>> =
                tool_executors.values().cloned().collect();
            let scoped_dynamic = adapt_tools(scoped_executors)?;
            Arc::new(tokio::sync::RwLock::new(build_agent(&def, scoped_dynamic)?))
        } else {
            agent
        };

        drop(def); // release read lock before spawning

        // Extract subagent names and descriptions, filtering out __self__
        let subagent_list: Vec<(String, String)> = state
            .subagents
            .iter()
            .filter(|entry| entry.key() != tools::self_agent::SELF_AGENT_NAME)
            .map(|entry| {
                (
                    entry.value().name.clone(),
                    entry.value().description.clone(),
                )
            })
            .collect();

        // Build system reminder with available skills and sub-agents
        let system_reminder =
            crate::system_reminder::create_reminder_with_skills(&skills, &subagent_list);

        let conv_metrics = Arc::new(bridge_core::metrics::ConversationMetrics::new(
            conv_id.clone(),
            agent_id.to_string(),
            model_name.clone(),
        ));
        let standalone_agent = self.standalone_agent;

        let ping_state = get_ping_state_from_registry(&tool_executors);
        state.tracker.spawn(async move {
            run_conversation(ConversationParams {
                agent_id: agent_id_owned,
                conversation_id: conv_id_clone,
                agent,
                message_rx,
                event_bus: conv_event_bus,
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
                permission_manager,
                agent_permissions,
                compaction_config,
                system_reminder,
                conversation_date: chrono::Utc::now(),
                llm_semaphore,
                initial_persisted_messages: Some(initial_persisted_messages),
                storage,
                tool_calls_only,
                conversation_metrics: conv_metrics,
                immortal_config,
                journal_state,
                per_conversation_mcp_scope: None,
                mcp_manager: None,
                standalone_agent,
                ping_state,
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
    pub async fn update_agent_api_key(
        &self,
        agent_id: &str,
        api_key: String,
    ) -> Result<(), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        // Clone current definition and update the API key
        let mut updated_def = state.definition.read().await.clone();
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
        *state.rig_agent.write().await = new_agent;
        *state.definition.write().await = updated_def.clone();

        if let Some(storage) = &self.storage {
            storage.save_agent(updated_def.clone());
        }

        info!(agent_id = agent_id, "agent API key updated");
        Ok(())
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

fn definitions_equivalent(existing: &AgentDefinition, incoming: &AgentDefinition) -> bool {
    match (&existing.version, &incoming.version) {
        (Some(existing_version), Some(incoming_version)) => existing_version == incoming_version,
        _ => existing == incoming,
    }
}

/// Apply optional tool/MCP scoping filters to a conversation's tool set.
///
/// - `mcp_server_names`: if provided, only tools from these MCP servers are kept.
/// - `tool_names_filter`: if provided, only these tools are kept (applied after MCP filter).
///
/// Returns `Err(BridgeError::InvalidRequest)` if any name is unrecognized.
pub(crate) fn filter_conversation_tools(
    agent_id: &str,
    tool_names: &mut std::collections::HashSet<String>,
    tool_executors: &mut std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
    mcp_server_tools: &std::collections::HashMap<String, Vec<String>>,
    filter_mcp_server_names: Option<&Vec<String>>,
    filter_tool_names: Option<&Vec<String>>,
) -> Result<(), BridgeError> {
    // Apply MCP server name filter: keep only tools from specified servers
    if let Some(server_names) = filter_mcp_server_names {
        for name in server_names {
            if !mcp_server_tools.contains_key(name) {
                return Err(BridgeError::InvalidRequest(format!(
                    "MCP server '{}' not found on agent '{}'",
                    name, agent_id
                )));
            }
        }
        let allowed_mcp_tools: std::collections::HashSet<String> = server_names
            .iter()
            .filter_map(|name| mcp_server_tools.get(name))
            .flat_map(|tools| tools.iter().cloned())
            .collect();
        let all_mcp_tools: std::collections::HashSet<String> = mcp_server_tools
            .values()
            .flat_map(|v| v.iter().cloned())
            .collect();
        let disallowed: std::collections::HashSet<String> = all_mcp_tools
            .difference(&allowed_mcp_tools)
            .cloned()
            .collect();
        tool_names.retain(|n| !disallowed.contains(n));
        tool_executors.retain(|n, _| !disallowed.contains(n));
    }

    // Apply tool name filter: retain only the requested tools
    if let Some(requested) = filter_tool_names {
        let requested_set: std::collections::HashSet<String> = requested.iter().cloned().collect();
        for name in &requested_set {
            if !tool_names.contains(name) {
                return Err(BridgeError::InvalidRequest(format!(
                    "tool '{}' not found on agent '{}'",
                    name, agent_id
                )));
            }
        }
        tool_names.retain(|n| requested_set.contains(n));
        tool_executors.retain(|n, _| requested_set.contains(n));
    }

    Ok(())
}

/// Build subagent entries from an agent definition's subagents list.
///
/// Each subagent gets built-in tools, the parent's integration tools
/// (with the same permissions), its own MCP server tools (if defined),
/// and its own skills (if defined). Skill files are written to disk
/// idempotently so shared skills between parent and subagents are safe.
/// No agent tool to prevent unbounded recursion at the configuration level.
async fn build_subagents(
    definition: &AgentDefinition,
    parent_integration_tools: &[(
        Arc<dyn tools::ToolExecutor>,
        bridge_core::permission::ToolPermission,
    )],
    mcp_manager: &McpManager,
    working_dir: &std::path::Path,
) -> Result<Arc<DashMap<String, SubAgentEntry>>, BridgeError> {
    let subagent_map = Arc::new(DashMap::new());

    for subagent_def in &definition.subagents {
        let mut sub_registry = ToolRegistry::new();
        tools::builtin::register_builtin_tools_for_subagent(&mut sub_registry);

        // Inherit parent's integration tools with same permissions
        for (tool, _) in parent_integration_tools {
            sub_registry.register(tool.clone());
        }

        // Connect subagent to its own MCP servers (if any)
        if !subagent_def.mcp_servers.is_empty() {
            let subagent_mcp_id = format!("{}::subagent::{}", definition.id, subagent_def.name);
            mcp_manager
                .connect_agent(&subagent_mcp_id, &subagent_def.mcp_servers)
                .await?;

            let connections = mcp_manager.get_agent_connections(&subagent_mcp_id);
            for conn in &connections {
                if let Ok(tools) = conn.list_tools().await {
                    let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                    for tool in bridged {
                        sub_registry.register(tool);
                    }
                }
            }
        }

        // Register subagent's skills (if any)
        if !subagent_def.skills.is_empty() {
            tools::skill_files::write_skill_files(&subagent_def.skills, working_dir).await;
            sub_registry.register(Arc::new(tools::skill_tools::SkillTool::with_base_dir(
                subagent_def.skills.clone(),
                working_dir.to_path_buf(),
            )));
        }

        // Apply subagent's disabled_tools
        for name in &subagent_def.config.disabled_tools {
            sub_registry.remove(name);
        }

        // Capture tool list before consuming the registry
        let registered_tools: Vec<(String, String)> = sub_registry
            .list()
            .into_iter()
            .map(|(name, desc)| (name.to_string(), desc.to_string()))
            .collect();

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
                registered_tools,
            },
        );
    }

    Ok(subagent_map)
}

async fn restore_agent_sessions(
    storage_backend: &dyn StorageBackend,
    agent_id: &str,
    session_store: &Arc<crate::agent_runner::AgentSessionStore>,
) {
    match storage_backend.load_sessions(agent_id).await {
        Ok(sessions) => {
            for (task_id, history_json) in sessions {
                match serde_json::from_slice::<Vec<rig::message::Message>>(&history_json) {
                    Ok(history) => session_store.restore(task_id, history),
                    Err(e) => {
                        error!(agent_id = %agent_id, error = %e, "failed to deserialize stored session history")
                    }
                }
            }
        }
        Err(e) => {
            error!(agent_id = %agent_id, error = %e, "failed to load stored sessions");
        }
    }
}

/// Helper function to get todos from the tool registry.
/// Looks for the TodoReadTool and extracts its state.
async fn get_todos_from_registry(
    tool_executors: &std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
) -> Option<Vec<crate::system_reminder::TodoItem>> {
    // Try to get the todoread tool to access its state
    if let Some(todo_tool) = tool_executors.get("todoread") {
        // Downcast to TodoReadTool to get the state
        if let Some(todo_read_tool) = todo_tool
            .as_ref()
            .as_any()
            .downcast_ref::<tools::todo::TodoReadTool>()
        {
            let todos = todo_read_tool.state().get().await;
            return Some(
                todos
                    .into_iter()
                    .map(|t| crate::system_reminder::TodoItem {
                        content: t.content,
                        status: t.status,
                        priority: t.priority,
                    })
                    .collect(),
            );
        }
    }

    // Alternative: try todowrite tool
    if let Some(todo_tool) = tool_executors.get("todowrite") {
        if let Some(todo_write_tool) = todo_tool
            .as_ref()
            .as_any()
            .downcast_ref::<tools::todo::TodoWriteTool>()
        {
            let todos = todo_write_tool.state().get().await;
            return Some(
                todos
                    .into_iter()
                    .map(|t| crate::system_reminder::TodoItem {
                        content: t.content,
                        status: t.status,
                        priority: t.priority,
                    })
                    .collect(),
            );
        }
    }

    None
}

/// Extract `PingState` from the tool registry by downcasting the ping_me_back_in tool.
fn get_ping_state_from_registry(
    tool_executors: &std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
) -> Option<tools::ping_me_back::PingState> {
    tool_executors
        .get("ping_me_back_in")
        .and_then(|tool| {
            tool.as_ref()
                .as_any()
                .downcast_ref::<tools::ping_me_back::PingMeBackTool>()
        })
        .map(|t| t.state().clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::any::Any;
    use std::collections::{HashMap, HashSet};
    use tools::ToolExecutor;

    /// Minimal mock tool executor for testing.
    struct MockTool {
        name: String,
    }

    impl MockTool {
        fn new(name: &str) -> Arc<dyn ToolExecutor> {
            Arc::new(Self {
                name: name.to_string(),
            })
        }
    }

    #[async_trait::async_trait]
    impl ToolExecutor for MockTool {
        fn name(&self) -> &str {
            &self.name
        }
        fn description(&self) -> &str {
            "mock tool"
        }
        fn parameters_schema(&self) -> serde_json::Value {
            serde_json::json!({})
        }
        async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
            Ok("ok".to_string())
        }
        fn as_any(&self) -> &dyn Any {
            self
        }
    }

    /// Helper: build a tool_names set and tool_executors map from a list of names.
    fn make_tools(names: &[&str]) -> (HashSet<String>, HashMap<String, Arc<dyn ToolExecutor>>) {
        let tool_names: HashSet<String> = names.iter().map(|n| n.to_string()).collect();
        let tool_executors: HashMap<String, Arc<dyn ToolExecutor>> = names
            .iter()
            .map(|n| (n.to_string(), MockTool::new(n)))
            .collect();
        (tool_names, tool_executors)
    }

    // ── filter_conversation_tools: no filters ─────────────────────────────────

    #[test]
    fn no_filters_returns_all_tools() {
        let (mut names, mut executors) = make_tools(&["bash", "read", "write", "glob"]);
        let mcp_map = HashMap::new();

        let result =
            filter_conversation_tools("agent1", &mut names, &mut executors, &mcp_map, None, None);

        assert!(result.is_ok());
        assert_eq!(names.len(), 4);
        assert_eq!(executors.len(), 4);
    }

    // ── filter_conversation_tools: tool_names filter ──────────────────────────

    #[test]
    fn tool_names_filter_retains_only_requested() {
        let (mut names, mut executors) = make_tools(&["bash", "read", "write", "glob"]);
        let mcp_map = HashMap::new();
        let filter = vec!["bash".to_string(), "read".to_string()];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            None,
            Some(&filter),
        );

        assert!(result.is_ok());
        assert_eq!(names.len(), 2);
        assert!(names.contains("bash"));
        assert!(names.contains("read"));
        assert!(!names.contains("write"));
        assert!(!names.contains("glob"));
        assert_eq!(executors.len(), 2);
        assert!(executors.contains_key("bash"));
        assert!(executors.contains_key("read"));
    }

    #[test]
    fn tool_names_filter_single_tool() {
        let (mut names, mut executors) = make_tools(&["bash", "read", "write"]);
        let mcp_map = HashMap::new();
        let filter = vec!["bash".to_string()];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            None,
            Some(&filter),
        );

        assert!(result.is_ok());
        assert_eq!(names.len(), 1);
        assert!(names.contains("bash"));
        assert_eq!(executors.len(), 1);
    }

    #[test]
    fn tool_names_filter_empty_array_means_no_tools() {
        let (mut names, mut executors) = make_tools(&["bash", "read", "write"]);
        let mcp_map = HashMap::new();
        let filter: Vec<String> = vec![];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            None,
            Some(&filter),
        );

        assert!(result.is_ok());
        assert_eq!(names.len(), 0);
        assert_eq!(executors.len(), 0);
    }

    #[test]
    fn tool_names_filter_unknown_tool_returns_error() {
        let (mut names, mut executors) = make_tools(&["bash", "read"]);
        let mcp_map = HashMap::new();
        let filter = vec!["bash".to_string(), "nonexistent".to_string()];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            None,
            Some(&filter),
        );

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("nonexistent"),
            "error should name the tool: {err}"
        );
        assert!(err.contains("agent1"), "error should name the agent: {err}");
    }

    // ── filter_conversation_tools: mcp_server_names filter ────────────────────

    #[test]
    fn mcp_filter_keeps_only_specified_server_tools() {
        // Agent has builtin tools + tools from two MCP servers
        let (mut names, mut executors) =
            make_tools(&["bash", "read", "search", "query", "index", "delete"]);
        let mut mcp_map = HashMap::new();
        mcp_map.insert(
            "server-a".to_string(),
            vec!["search".to_string(), "query".to_string()],
        );
        mcp_map.insert(
            "server-b".to_string(),
            vec!["index".to_string(), "delete".to_string()],
        );
        let filter = vec!["server-a".to_string()];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            Some(&filter),
            None,
        );

        assert!(result.is_ok());
        // Builtin tools (bash, read) remain, server-a tools (search, query) remain
        // server-b tools (index, delete) are removed
        assert_eq!(names.len(), 4);
        assert!(names.contains("bash"));
        assert!(names.contains("read"));
        assert!(names.contains("search"));
        assert!(names.contains("query"));
        assert!(!names.contains("index"));
        assert!(!names.contains("delete"));
        assert_eq!(executors.len(), 4);
    }

    #[test]
    fn mcp_filter_empty_array_removes_all_mcp_tools() {
        let (mut names, mut executors) = make_tools(&["bash", "read", "search", "index"]);
        let mut mcp_map = HashMap::new();
        mcp_map.insert("server-a".to_string(), vec!["search".to_string()]);
        mcp_map.insert("server-b".to_string(), vec!["index".to_string()]);
        let filter: Vec<String> = vec![];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            Some(&filter),
            None,
        );

        assert!(result.is_ok());
        // Only builtin tools remain
        assert_eq!(names.len(), 2);
        assert!(names.contains("bash"));
        assert!(names.contains("read"));
        assert!(!names.contains("search"));
        assert!(!names.contains("index"));
    }

    #[test]
    fn mcp_filter_unknown_server_returns_error() {
        let (mut names, mut executors) = make_tools(&["bash", "search"]);
        let mut mcp_map = HashMap::new();
        mcp_map.insert("server-a".to_string(), vec!["search".to_string()]);
        let filter = vec!["server-a".to_string(), "nonexistent-server".to_string()];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            Some(&filter),
            None,
        );

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("nonexistent-server"),
            "error should name the server: {err}"
        );
        assert!(err.contains("agent1"), "error should name the agent: {err}");
    }

    // ── filter_conversation_tools: both filters combined ──────────────────────

    #[test]
    fn both_filters_mcp_applied_first_then_tool_names() {
        let (mut names, mut executors) = make_tools(&["bash", "read", "search", "query", "index"]);
        let mut mcp_map = HashMap::new();
        mcp_map.insert(
            "server-a".to_string(),
            vec!["search".to_string(), "query".to_string()],
        );
        mcp_map.insert("server-b".to_string(), vec!["index".to_string()]);

        // MCP filter: only server-a → removes "index"
        // Tool filter: only "bash" and "search" → removes "read" and "query"
        let mcp_filter = vec!["server-a".to_string()];
        let tool_filter = vec!["bash".to_string(), "search".to_string()];

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            Some(&mcp_filter),
            Some(&tool_filter),
        );

        assert!(result.is_ok());
        assert_eq!(names.len(), 2);
        assert!(names.contains("bash"));
        assert!(names.contains("search"));
        assert_eq!(executors.len(), 2);
    }

    #[test]
    fn tool_filter_referencing_mcp_tool_removed_by_server_filter_errors() {
        // MCP filter removes "index", then tool filter requests "index" → error
        let (mut names, mut executors) = make_tools(&["bash", "search", "index"]);
        let mut mcp_map = HashMap::new();
        mcp_map.insert("server-a".to_string(), vec!["search".to_string()]);
        mcp_map.insert("server-b".to_string(), vec!["index".to_string()]);

        let mcp_filter = vec!["server-a".to_string()]; // removes "index"
        let tool_filter = vec!["bash".to_string(), "index".to_string()]; // requests "index"

        let result = filter_conversation_tools(
            "agent1",
            &mut names,
            &mut executors,
            &mcp_map,
            Some(&mcp_filter),
            Some(&tool_filter),
        );

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("index"),
            "error should name the unavailable tool: {err}"
        );
    }

    // ── Supervisor integration tests ──────────────────────────────────────────

    fn make_test_supervisor() -> AgentSupervisor {
        let mcp_manager = Arc::new(McpManager::new());
        let cancel = CancellationToken::new();
        let event_bus = Arc::new(webhooks::EventBus::new(
            None,
            None,
            String::new(),
            String::new(),
        ));
        AgentSupervisor::new(mcp_manager, cancel).with_event_bus(Some(event_bus))
    }

    fn make_test_definition(id: &str) -> AgentDefinition {
        AgentDefinition {
            id: id.to_string(),
            name: format!("Test Agent {}", id),
            description: None,
            system_prompt: "You are a test agent.".to_string(),
            provider: bridge_core::provider::ProviderConfig {
                provider_type: bridge_core::provider::ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "test-key".to_string(),
                base_url: Some("https://api.openai.com/v1".to_string()),
                prompt_caching_enabled: true,
                cache_ttl: Default::default(),
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: bridge_core::agent::AgentConfig::default(),
            subagents: vec![],
            permissions: std::collections::HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: Some("1".to_string()),
            updated_at: None,
        }
    }

    #[tokio::test]
    async fn supervisor_create_conversation_no_filters_succeeds() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let result = supervisor
            .create_conversation("agent1", None, None, None, None, None, None)
            .await;

        assert!(result.is_ok());
        let (conv_id, _sse_rx) = result.unwrap();
        assert!(!conv_id.is_empty());

        // Cleanup
        supervisor.end_conversation("agent1", &conv_id).unwrap();
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_valid_tool_filter_succeeds() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        // Agent has builtin tools (bash, read, write, etc.) because tools: [] means all builtins.
        // Pick some known builtin tool names.
        let state = supervisor.get_agent("agent1").unwrap();
        let all_tools: Vec<String> = state.tool_registry.tool_names();
        assert!(!all_tools.is_empty(), "agent should have builtin tools");

        // Request only the first two tools
        let filter = all_tools.iter().take(2).cloned().collect::<Vec<_>>();

        let result = supervisor
            .create_conversation("agent1", Some(filter.clone()), None, None, None, None, None)
            .await;

        assert!(result.is_ok());
        let (conv_id, _sse_rx) = result.unwrap();
        assert!(!conv_id.is_empty());

        supervisor.end_conversation("agent1", &conv_id).unwrap();
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_invalid_tool_returns_error() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let result = supervisor
            .create_conversation(
                "agent1",
                Some(vec!["totally_fake_tool".to_string()]),
                None,
                None,
                None,
                None,
                None,
            )
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("totally_fake_tool"));
        assert!(err.contains("agent1"));
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_invalid_mcp_server_returns_error() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let result = supervisor
            .create_conversation(
                "agent1",
                None,
                Some(vec!["nonexistent-mcp".to_string()]),
                None,
                None,
                None,
                None,
            )
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("nonexistent-mcp"));
    }

    #[tokio::test]
    async fn supervisor_create_conversation_unknown_agent_returns_error() {
        let supervisor = make_test_supervisor();

        let result = supervisor
            .create_conversation("no_such_agent", None, None, None, None, None, None)
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("no_such_agent"));
    }

    // ── per-conversation API key override ──────────────────────────────────

    #[tokio::test]
    async fn supervisor_create_conversation_with_api_key_override_succeeds() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let result = supervisor
            .create_conversation(
                "agent1",
                None,
                None,
                Some("sk-custom-override-key".to_string()),
                None,
                None,
                None,
            )
            .await;

        assert!(result.is_ok());
        let (conv_id, _sse_rx) = result.unwrap();
        assert!(!conv_id.is_empty());

        supervisor.end_conversation("agent1", &conv_id).unwrap();
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_empty_api_key_returns_error() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let result = supervisor
            .create_conversation("agent1", None, None, Some("".to_string()), None, None, None)
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("api_key cannot be empty"));
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_whitespace_api_key_returns_error() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let result = supervisor
            .create_conversation(
                "agent1",
                None,
                None,
                Some("   ".to_string()),
                None,
                None,
                None,
            )
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("api_key cannot be empty"));
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_api_key_and_tool_filter_succeeds() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let state = supervisor.get_agent("agent1").unwrap();
        let all_tools: Vec<String> = state.tool_registry.tool_names();
        let filter = all_tools.iter().take(2).cloned().collect::<Vec<_>>();

        let result = supervisor
            .create_conversation(
                "agent1",
                Some(filter),
                None,
                Some("sk-custom-key".to_string()),
                None,
                None,
                None,
            )
            .await;

        assert!(result.is_ok());
        let (conv_id, _sse_rx) = result.unwrap();
        supervisor.end_conversation("agent1", &conv_id).unwrap();
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_invalid_subagent_name_returns_error() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let mut overrides = std::collections::HashMap::new();
        overrides.insert("nonexistent_subagent".to_string(), "<provider-api-key>".to_string());

        let result = supervisor
            .create_conversation("agent1", None, None, None, Some(overrides), None, None)
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("nonexistent_subagent"));
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_empty_subagent_api_key_returns_error() {
        let supervisor = make_test_supervisor();

        let mut def = make_test_definition("agent1");
        def.subagents.push(AgentDefinition {
            id: "sub1".to_string(),
            name: "sub1".to_string(),
            description: Some("A test subagent".to_string()),
            system_prompt: "You are a sub agent.".to_string(),
            provider: bridge_core::provider::ProviderConfig {
                provider_type: bridge_core::provider::ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "sub-key".to_string(),
                base_url: Some("https://api.openai.com/v1".to_string()),
                prompt_caching_enabled: true,
                cache_ttl: Default::default(),
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: bridge_core::agent::AgentConfig::default(),
            subagents: vec![],
            permissions: std::collections::HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        });

        supervisor.load_agents(vec![def]).await.unwrap();

        let mut overrides = std::collections::HashMap::new();
        overrides.insert("sub1".to_string(), "".to_string());

        let result = supervisor
            .create_conversation("agent1", None, None, None, Some(overrides), None, None)
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(err.contains("cannot be empty"));
    }

    #[tokio::test]
    async fn supervisor_create_conversation_with_subagent_api_key_override_succeeds() {
        let supervisor = make_test_supervisor();

        let mut def = make_test_definition("agent1");
        def.subagents.push(AgentDefinition {
            id: "sub1".to_string(),
            name: "sub1".to_string(),
            description: Some("A test subagent".to_string()),
            system_prompt: "You are a sub agent.".to_string(),
            provider: bridge_core::provider::ProviderConfig {
                provider_type: bridge_core::provider::ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "sub-key".to_string(),
                base_url: Some("https://api.openai.com/v1".to_string()),
                prompt_caching_enabled: true,
                cache_ttl: Default::default(),
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: bridge_core::agent::AgentConfig::default(),
            subagents: vec![],
            permissions: std::collections::HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        });

        supervisor.load_agents(vec![def]).await.unwrap();

        let mut overrides = std::collections::HashMap::new();
        overrides.insert("sub1".to_string(), "sk-overridden-sub-key".to_string());

        let result = supervisor
            .create_conversation("agent1", None, None, None, Some(overrides), None, None)
            .await;

        assert!(result.is_ok());
        let (conv_id, _sse_rx) = result.unwrap();
        supervisor.end_conversation("agent1", &conv_id).unwrap();
    }

    // ── per-conversation MCP server validation ──────────────────────────────

    #[tokio::test]
    async fn per_conv_mcp_rejects_stdio_when_flag_disabled() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let servers = vec![McpServerDefinition {
            name: "local".to_string(),
            transport: McpTransport::Stdio {
                command: "/bin/echo".to_string(),
                args: vec![],
                env: std::collections::HashMap::new(),
            },
        }];

        let result = supervisor
            .create_conversation("agent1", None, None, None, None, None, Some(servers))
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("stdio transport not allowed"),
            "error should describe stdio gate, got: {err}"
        );
        assert!(
            err.contains("'local'"),
            "error should name server, got: {err}"
        );

        // Nothing should have leaked into the MCP manager.
        assert_eq!(supervisor.mcp_manager.connection_count(), 0);
        // No dangling conversation handle.
        let state = supervisor.get_agent("agent1").unwrap();
        assert_eq!(state.conversations.len(), 0);
    }

    #[tokio::test]
    async fn per_conv_mcp_rejects_empty_server_name() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let servers = vec![McpServerDefinition {
            name: "   ".to_string(),
            transport: McpTransport::StreamableHttp {
                url: "http://127.0.0.1:1".to_string(),
                headers: std::collections::HashMap::new(),
            },
        }];

        let result = supervisor
            .create_conversation("agent1", None, None, None, None, None, Some(servers))
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("server name cannot be empty"),
            "error should describe empty-name rejection, got: {err}"
        );
    }

    #[tokio::test]
    async fn per_conv_mcp_rejects_duplicate_server_names() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let servers = vec![
            McpServerDefinition {
                name: "dup".to_string(),
                transport: McpTransport::StreamableHttp {
                    url: "http://127.0.0.1:1".to_string(),
                    headers: std::collections::HashMap::new(),
                },
            },
            McpServerDefinition {
                name: "dup".to_string(),
                transport: McpTransport::StreamableHttp {
                    url: "http://127.0.0.1:2".to_string(),
                    headers: std::collections::HashMap::new(),
                },
            },
        ];

        let result = supervisor
            .create_conversation("agent1", None, None, None, None, None, Some(servers))
            .await;

        assert!(result.is_err());
        let err = result.unwrap_err().to_string();
        assert!(
            err.contains("duplicate server name 'dup'"),
            "error should describe duplicate, got: {err}"
        );
    }

    #[tokio::test]
    async fn per_conv_mcp_empty_list_is_no_op() {
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        // Empty servers vec — should succeed and behave identically to None.
        let result = supervisor
            .create_conversation("agent1", None, None, None, None, None, Some(vec![]))
            .await;

        assert!(result.is_ok(), "empty mcp_servers list should be a no-op");
        let (conv_id, _sse_rx) = result.unwrap();
        assert!(!conv_id.is_empty());
        assert_eq!(supervisor.mcp_manager.connection_count(), 0);
        supervisor.end_conversation("agent1", &conv_id).unwrap();
    }

    #[tokio::test]
    async fn per_conv_mcp_unreachable_http_rolls_back_cleanly() {
        // Point at an unreachable TCP port so the connect attempt fails.
        // Expected: InvalidRequest error, no leaked MCP connections, no dangling
        // conversation handle. This exercises the error-unwind path.
        let supervisor = make_test_supervisor();
        supervisor
            .load_agents(vec![make_test_definition("agent1")])
            .await
            .unwrap();

        let servers = vec![McpServerDefinition {
            name: "unreachable".to_string(),
            transport: McpTransport::StreamableHttp {
                url: "http://127.0.0.1:1".to_string(),
                headers: std::collections::HashMap::new(),
            },
        }];

        let result = supervisor
            .create_conversation("agent1", None, None, None, None, None, Some(servers))
            .await;

        assert!(
            result.is_err(),
            "unreachable MCP server should surface an error"
        );
        assert_eq!(
            supervisor.mcp_manager.connection_count(),
            0,
            "no leaked MCP connections after failed connect"
        );
        let state = supervisor.get_agent("agent1").unwrap();
        assert_eq!(
            state.conversations.len(),
            0,
            "no dangling conversation handle after failed connect"
        );
    }
}
