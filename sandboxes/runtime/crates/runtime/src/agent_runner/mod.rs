use dashmap::DashMap;
use llm::BridgeAgent;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentTaskNotification, TaskBudget};
use webhooks::EventBus;

mod background;
mod foreground;
mod runner_impl;
mod session_store;

pub use session_store::AgentSessionStore;

#[cfg(test)]
mod tests;

/// Resolve (foreground, background) subagent timeouts from an `AgentConfig`,
/// falling back to [`bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS`].
pub fn resolve_subagent_timeouts(config: &bridge_core::agent::AgentConfig) -> (Duration, Duration) {
    let default_secs = bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS;
    let fg = config
        .subagent_timeout_foreground_secs
        .unwrap_or(default_secs);
    let bg = config
        .subagent_timeout_background_secs
        .unwrap_or(default_secs);
    (Duration::from_secs(fg), Duration::from_secs(bg))
}

/// A pre-built subagent entry ready for invocation.
pub struct SubAgentEntry {
    pub name: String,
    pub description: String,
    pub agent: Arc<BridgeAgent>,
    /// Tool names and descriptions registered for this subagent at build time.
    pub registered_tools: Vec<(String, String)>,
    /// Wall-clock timeout for foreground invocations of this subagent,
    /// resolved from its `AgentConfig.subagent_timeout_foreground_secs`
    /// (falling back to [`bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS`]).
    pub foreground_timeout: Duration,
    /// Wall-clock timeout for background invocations of this subagent,
    /// resolved from its `AgentConfig.subagent_timeout_background_secs`
    /// (falling back to [`bridge_core::agent::DEFAULT_SUBAGENT_TIMEOUT_SECS`]).
    pub background_timeout: Duration,
}

/// Runtime implementation of [`SubAgentRunner`] that uses rig-core agents.
pub struct ConversationSubAgentRunner {
    pub(super) subagents: Arc<DashMap<String, SubAgentEntry>>,
    pub(super) session_store: Arc<AgentSessionStore>,
    pub(super) notification_tx: mpsc::Sender<AgentTaskNotification>,
    pub(super) cancel: CancellationToken,
    pub(super) event_bus: Arc<EventBus>,
    pub(super) conversation_id: String,
    pub(super) depth: usize,
    pub(super) max_depth: usize,
    pub(super) compaction_config: Option<bridge_core::agent::CompactionConfig>,
    pub(super) task_budget: Arc<TaskBudget>,
    pub(super) metrics: Arc<bridge_core::AgentMetrics>,
    /// Agent ID for event payloads.
    pub(super) agent_id: String,
}

impl ConversationSubAgentRunner {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        subagents: Arc<DashMap<String, SubAgentEntry>>,
        session_store: Arc<AgentSessionStore>,
        notification_tx: mpsc::Sender<AgentTaskNotification>,
        cancel: CancellationToken,
        event_bus: Arc<EventBus>,
        conversation_id: String,
        depth: usize,
        max_depth: usize,
        metrics: Arc<bridge_core::AgentMetrics>,
    ) -> Self {
        Self {
            subagents,
            session_store,
            notification_tx,
            cancel,
            event_bus,
            conversation_id,
            depth,
            max_depth,
            compaction_config: None,
            task_budget: Arc::new(TaskBudget::new(50)),
            metrics,
            agent_id: String::new(),
        }
    }

    /// Set the agent ID for subagent trace events.
    pub fn with_agent_id(mut self, agent_id: String) -> Self {
        self.agent_id = agent_id;
        self
    }

    /// Set the compaction configuration for subagent sessions.
    pub fn with_compaction(mut self, config: Option<bridge_core::agent::CompactionConfig>) -> Self {
        self.compaction_config = config;
        self
    }

    /// Set the task budget for limiting subagent spawning.
    pub fn with_task_budget(mut self, budget: Arc<TaskBudget>) -> Self {
        self.task_budget = budget;
        self
    }

    /// Generate a task_id scoped to this conversation.
    pub(super) fn generate_task_id(&self) -> String {
        format!("{}-{}", self.conversation_id, uuid::Uuid::new_v4())
    }
}
