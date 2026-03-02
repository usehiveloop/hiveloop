use bridge_core::conversation::Message;
use bridge_core::{AgentDefinition, AgentMetrics};
use dashmap::DashMap;
use llm::BridgeAgent;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tokio_util::task::TaskTracker;
use tools::ToolRegistry;

/// Handle for an active conversation, used by the supervisor to send messages.
pub struct ConversationHandle {
    /// Unique conversation identifier.
    pub id: String,
    /// Channel sender for delivering user messages to the conversation loop.
    pub message_tx: mpsc::Sender<Message>,
    /// When this conversation was created.
    pub created_at: chrono::DateTime<chrono::Utc>,
}

/// Complete runtime state for a single agent.
///
/// Holds the agent's definition, LLM client, tool registry, active conversations,
/// metrics, and lifecycle management primitives.
pub struct AgentState {
    /// The agent definition from the control plane.
    pub definition: AgentDefinition,
    /// The built rig-core agent for LLM interactions.
    pub rig_agent: BridgeAgent,
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
}

impl AgentState {
    /// Create a new agent state.
    pub fn new(
        definition: AgentDefinition,
        rig_agent: BridgeAgent,
        tool_registry: ToolRegistry,
    ) -> Self {
        Self {
            definition,
            rig_agent,
            tool_registry,
            conversations: DashMap::new(),
            cancel: CancellationToken::new(),
            tracker: TaskTracker::new(),
            metrics: Arc::new(AgentMetrics::new()),
        }
    }

    /// Get the agent's ID.
    pub fn id(&self) -> &str {
        &self.definition.id
    }

    /// Get the agent's name.
    pub fn name(&self) -> &str {
        &self.definition.name
    }

    /// Get the agent's version.
    pub fn version(&self) -> Option<&str> {
        self.definition.version.as_deref()
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
