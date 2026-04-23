use bridge_core::conversation::Message;
use bridge_core::event::BridgeEvent;
use bridge_core::mcp::McpServerDefinition;
use bridge_core::{AgentDefinition, BridgeError};
use llm::{adapt_tools, build_agent};
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification};
use tracing::info;

use super::conv_mcp::{connect_per_conv_mcp, validate_per_conv_mcp_servers};
use super::conversations_helpers::{
    acquire_conversation_permit, build_conversation_subagents, build_system_reminder,
    register_journal_tools, validate_api_key_overrides,
};
use super::helpers::{filter_conversation_tools, get_ping_state_from_registry};
use super::AgentSupervisor;
use crate::agent_runner::ConversationSubAgentRunner;
use crate::agent_state::ConversationHandle;
use crate::conversation::{run_conversation, ConversationParams};

impl AgentSupervisor {
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

        let conversation_permit = acquire_conversation_permit(self, &state, agent_id).await?;

        validate_per_conv_mcp_servers(self, per_conversation_mcp_servers.as_deref())?;
        validate_api_key_overrides(
            &state,
            api_key_override.as_deref(),
            subagent_api_key_overrides.as_ref(),
        )?;

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
        let conversation_subagents =
            build_conversation_subagents(&state, &def, subagent_api_key_overrides.as_ref())?;

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
        let history_strip_config = def.config.history_strip.clone();
        let tool_calls_only = def.config.tool_calls_only.unwrap_or(false);
        let tool_requirements = def.config.tool_requirements.clone();
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
            register_journal_tools(js, &mut tool_names, &mut tool_executors);
        }

        // Connect per-conversation MCP servers and merge their tools into the
        // per-conversation executor map.
        let per_conv_mcp_scope = connect_per_conv_mcp(
            self,
            &state,
            &conv_id,
            per_conversation_mcp_servers.as_deref(),
            &mut tool_names,
            &mut tool_executors,
        )
        .await?;

        // Build a scoped definition when provider or API key overrides are present.
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
        // MCP servers were attached.
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
        let retry_agent = Arc::new(
            build_agent(effective_def, vec![]).expect("no-tools agent build should not fail"),
        );
        drop(def);

        // Check if todo tools are enabled
        let has_todo_tools = tool_names.contains("todoread") && tool_names.contains("todowrite");

        // Build system reminder with available skills, sub-agents, and optionally todos
        let system_reminder = build_system_reminder(&state, &tool_executors, has_todo_tools).await;

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
                history_strip_config,
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
                tool_requirements,
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
}
