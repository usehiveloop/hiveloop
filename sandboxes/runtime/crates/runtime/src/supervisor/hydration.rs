use bridge_core::conversation::{ConversationRecord, Message};
use bridge_core::event::BridgeEvent;
use bridge_core::BridgeError;
use llm::{adapt_tools, build_agent};
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification};
use tracing::{error, info};

use super::conversations_helpers::register_journal_tools;
use super::helpers::get_ping_state_from_registry;
use super::AgentSupervisor;
use crate::agent_runner::ConversationSubAgentRunner;
use crate::agent_state::ConversationHandle;
use crate::conversation::{run_conversation, ConversationParams};

impl AgentSupervisor {
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
        let history_strip_config = def.config.history_strip.clone();
        let tool_calls_only = def.config.tool_calls_only.unwrap_or(false);
        let system_reminder_refresh_turns = def.config.system_reminder_refresh_turns;
        let tool_requirements = def.config.tool_requirements.clone();
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

        // Register journal tools only if immortal mode is active AND the
        // agent hasn't opted out via `expose_journal_tools: false`.
        if let (Some(ref js), Some(ref imm)) = (&journal_state, &immortal_config) {
            if imm.expose_journal_tools {
                register_journal_tools(js, &mut tool_names, &mut tool_executors);
            }
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
                history_strip_config,
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
                system_reminder_refresh_turns,
                ping_state,
                tool_requirements,
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
}
