use bridge_core::conversation::Message;
use bridge_core::{AgentDefinition, AgentMetrics};
use dashmap::DashMap;
use llm::BridgeAgent;
use std::sync::Arc;
use tokio::sync::{Mutex, RwLock};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tokio_util::task::TaskTracker;
use tools::join::TaskRegistry;
use tools::ToolRegistry;

use crate::agent_runner::{AgentSessionStore, SubAgentEntry};

/// Handle for an active conversation, used by the supervisor to send messages.
pub struct ConversationHandle {
    /// Unique conversation identifier.
    pub id: String,
    /// Channel sender for delivering user messages to the conversation loop.
    pub message_tx: mpsc::Sender<Message>,
    /// When this conversation was created.
    pub created_at: chrono::DateTime<chrono::Utc>,
    /// Token for aborting the current in-flight turn.
    /// Replaced with a fresh token at the start of each turn.
    pub abort_token: Arc<Mutex<CancellationToken>>,
}

/// Complete runtime state for a single agent.
///
/// Holds the agent's definition, LLM client, tool registry, active conversations,
/// metrics, and lifecycle management primitives.
pub struct AgentState {
    /// The agent definition from the control plane (behind RwLock for API key rotation).
    pub definition: RwLock<AgentDefinition>,
    /// The built rig-core agent for LLM interactions (behind RwLock for API key rotation).
    pub rig_agent: Arc<RwLock<BridgeAgent>>,
    /// Registry of all available tools (built-in + MCP).
    pub tool_registry: ToolRegistry,
    /// Active conversation handles, keyed by conversation ID.
    pub conversations: DashMap<String, ConversationHandle>,
    /// Cancellation token for graceful shutdown of this agent's tasks.
    pub cancel: CancellationToken,
    /// Task tracker for monitoring conversation tasks.
    pub tracker: TaskTracker,
    /// Metrics counters for this agent.
    pub metrics: Arc<AgentMetrics>,
    /// Pre-built subagent entries, keyed by subagent name.
    pub subagents: Arc<DashMap<String, SubAgentEntry>>,
    /// Session store for subagent history persistence.
    pub session_store: Arc<AgentSessionStore>,
    /// Task registry for tracking background subagent tasks.
    pub task_registry: Arc<TaskRegistry>,
}

impl AgentState {
    /// Create a new agent state.
    pub fn new(
        definition: AgentDefinition,
        rig_agent: BridgeAgent,
        tool_registry: ToolRegistry,
        subagents: Arc<DashMap<String, SubAgentEntry>>,
        task_registry: Arc<TaskRegistry>,
    ) -> Self {
        Self {
            definition: RwLock::new(definition),
            rig_agent: Arc::new(RwLock::new(rig_agent)),
            tool_registry,
            conversations: DashMap::new(),
            cancel: CancellationToken::new(),
            tracker: TaskTracker::new(),
            metrics: Arc::new(AgentMetrics::new()),
            subagents,
            session_store: Arc::new(AgentSessionStore::new()),
            task_registry,
        }
    }

    /// Get the agent's ID.
    pub async fn id(&self) -> String {
        self.definition.read().await.id.clone()
    }

    /// Get the agent's name.
    pub async fn name(&self) -> String {
        self.definition.read().await.name.clone()
    }

    /// Get the agent's version.
    pub async fn version(&self) -> Option<String> {
        self.definition.read().await.version.clone()
    }

    /// Check if this agent has an active conversation with the given ID.
    pub fn has_conversation(&self, conv_id: &str) -> bool {
        self.conversations.contains_key(conv_id)
    }

    /// Get the number of active conversations.
    pub fn active_conversation_count(&self) -> usize {
        self.conversations.len()
    }
}
