use bridge_core::conversation::Message;
use bridge_core::metrics::ConversationMetrics;
use bridge_core::permission::ToolPermission;
use bridge_core::AgentMetrics;
use llm::PermissionManager;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::Duration;
use storage::StorageHandle;
use tokio::sync::mpsc;
use tokio::sync::{Mutex, RwLock};
use tokio_util::sync::CancellationToken;
use tools::agent::{AgentContext, AgentTaskNotification};
use tools::ToolExecutor;
use webhooks::EventBus;

use crate::agent_runner::AgentSessionStore;

/// Timeout for a single agent.chat() call (includes internal tool loops).
pub(super) const AGENT_CHAT_TIMEOUT: Duration = Duration::from_secs(6 * 60 * 60);

/// Timeout for automatic continuation attempts when the agent returns an empty response.
pub(super) const CONTINUATION_TIMEOUT: Duration = Duration::from_secs(180);

/// Maximum number of automatic continuation attempts when the agent returns an
/// empty response. After this many continuations, fall back to the no-tools
/// retry agent.
pub(super) const MAX_CONTINUATIONS: usize = 3;

/// Incoming message for the conversation loop — either a user message or
/// a background subagent completion notification.
pub(super) enum IncomingMessage {
    User(Message),
    BackgroundComplete(AgentTaskNotification),
    PingFired(Vec<tools::ping_me_back::PendingPing>),
}

/// Parameters for running a conversation loop.
pub struct ConversationParams {
    /// Agent ID.
    pub agent_id: String,
    /// Conversation ID.
    pub conversation_id: String,
    /// The built rig-core agent (behind RwLock for API key rotation).
    pub agent: Arc<RwLock<llm::BridgeAgent>>,
    /// Receiver for user messages.
    pub message_rx: mpsc::Receiver<Message>,
    /// Unified event bus for SSE, WebSocket, webhook, and persistence delivery.
    pub event_bus: Arc<EventBus>,
    /// Metrics counters for this agent.
    pub metrics: Arc<AgentMetrics>,
    /// Cancellation token for graceful shutdown.
    pub cancel: CancellationToken,
    /// Maximum number of turns before ending the conversation.
    pub max_turns: Option<usize>,
    /// Optional agent context for subagent spawning.
    pub agent_context: Option<AgentContext>,
    /// Receiver for background task completion notifications.
    pub notification_rx: Option<mpsc::Receiver<AgentTaskNotification>>,
    /// Session store reference for cleanup on conversation end.
    pub session_store: Option<Arc<AgentSessionStore>>,
    /// Known tool names for tool repair (unknown tool name suggestion).
    pub tool_names: HashSet<String>,
    /// Tool executors for auto-repair dispatch (keyed by canonical name).
    pub tool_executors: HashMap<String, Arc<dyn ToolExecutor>>,
    /// Pre-seeded conversation history (used when hydrating from the control plane).
    pub initial_history: Option<Vec<rig::message::Message>>,
    /// No-tools agent used to retry when the primary agent returns an empty response.
    /// Because it has no tools registered the model is forced to produce text.
    pub retry_agent: Arc<llm::BridgeAgent>,
    /// Shared abort token — holds the current turn's CancellationToken.
    pub abort_token: Arc<Mutex<CancellationToken>>,
    /// Permission manager for handling tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
    /// Per-tool permission overrides for this agent.
    pub agent_permissions: HashMap<String, ToolPermission>,
    /// Optional compaction configuration for history summarization.
    pub compaction_config: Option<bridge_core::agent::CompactionConfig>,
    /// Optional tool-result stripping configuration. When `None`, defaults
    /// are applied (stripping enabled, standard thresholds).
    pub history_strip_config: Option<bridge_core::agent::HistoryStripConfig>,
    /// System reminder markdown to inject before every user message.
    pub system_reminder: String,
    /// Initial conversation date for date tracking.
    pub conversation_date: chrono::DateTime<chrono::Utc>,
    /// Global LLM call semaphore for admission control.
    pub llm_semaphore: Arc<tokio::sync::Semaphore>,
    /// Initial persistence-ready history, normalized to align with rig history.
    pub initial_persisted_messages: Option<Vec<Message>>,
    /// Optional non-blocking persistence handle.
    pub storage: Option<StorageHandle>,
    /// When true, empty text responses are accepted as success if tool calls were made.
    pub tool_calls_only: bool,
    /// Per-conversation metrics for token/tool tracking.
    pub conversation_metrics: Arc<ConversationMetrics>,
    /// Optional immortal conversation configuration (replaces compaction when set).
    pub immortal_config: Option<bridge_core::agent::ImmortalConfig>,
    /// Journal state shared with the journal_write tool (only set in immortal mode).
    pub journal_state: Option<Arc<tools::journal::JournalState>>,
    /// MCP scope key for per-conversation MCP servers.
    /// `Some(conv_id)` when the conversation owns its own MCP connections that
    /// must be torn down on exit; `None` when only agent-level MCP is in use.
    pub per_conversation_mcp_scope: Option<String>,
    /// MCP manager handle used to disconnect per-conversation servers during cleanup.
    /// Only meaningful when `per_conversation_mcp_scope` is set.
    pub mcp_manager: Option<Arc<mcp::McpManager>>,
    /// When true, inject environment system reminder (resource usage, installed tools).
    pub standalone_agent: bool,
    /// Shared state for ping-me-back timers (non-blocking delayed reminders).
    pub ping_state: Option<tools::ping_me_back::PingState>,
    /// Declarative tool-call requirements evaluated at the end of every
    /// successful turn (see [`bridge_core::agent::ToolRequirement`]).
    /// Empty vec disables enforcement.
    pub tool_requirements: Vec<bridge_core::agent::ToolRequirement>,
}
