use bridge_core::{AgentSummary, RuntimeConfig};
use llm::PermissionManager;
use lsp::LspManager;
use mcp::McpManager;
use std::sync::Arc;
use storage::{StorageBackend, StorageHandle};
use tokio_util::sync::CancellationToken;
use webhooks::EventBus;

mod agent_build;
mod agent_loading;
mod conv_mcp;
mod conversations;
mod conversations_helpers;
mod helpers;
mod hydration;
mod messaging;

#[cfg(test)]
mod tests_conv;
#[cfg(test)]
mod tests_filters;
#[cfg(test)]
mod tests_mcp;
#[cfg(test)]
mod tests_mock;
#[cfg(test)]
mod tests_subagent;

use crate::agent_map::AgentMap;
use crate::agent_state::AgentState;

use helpers::DEFAULT_MAX_CONCURRENT_LLM_CALLS;

/// Central supervisor that manages all agent lifecycles.
///
/// Handles loading agents, creating conversations, routing messages,
/// and applying configuration diffs from the control plane.
pub struct AgentSupervisor {
    /// Map of all loaded agents.
    pub(super) agent_map: AgentMap,
    /// MCP connection manager shared across agents.
    pub(super) mcp_manager: Arc<McpManager>,
    /// LSP manager shared across agents (optional).
    pub(super) lsp_manager: Option<Arc<LspManager>>,
    /// Global cancellation token.
    pub(super) cancel: CancellationToken,
    /// Optional event bus for unified event delivery (SSE, WebSocket, webhooks, persistence).
    pub(super) event_bus: Option<Arc<EventBus>>,
    /// Shared permission manager for tool approval requests.
    pub(super) permission_manager: Arc<PermissionManager>,
    /// Limits total concurrent conversations across all agents.
    pub(super) conversation_semaphore: Option<Arc<tokio::sync::Semaphore>>,
    /// Limits total concurrent outbound LLM API calls.
    pub(super) llm_semaphore: Arc<tokio::sync::Semaphore>,
    /// Optional non-blocking persistence handle.
    pub(super) storage: Option<StorageHandle>,
    /// Optional persistence backend for startup/restore reads.
    pub(super) storage_backend: Option<Arc<dyn StorageBackend>>,
    /// When true, scan the working directory for skills from .claude/, .cursor/, etc.
    pub(super) skill_discovery_enabled: bool,
    /// Working directory for skill discovery. Defaults to `std::env::current_dir()`.
    pub(super) skill_discovery_dir: Option<String>,
    /// When true, API clients may attach `stdio` MCP servers per conversation.
    /// Default: false (only `streamable_http` accepted from the API).
    pub(super) allow_stdio_mcp_from_api: bool,
    /// When true, inject environment system reminder (installed tools, resource usage).
    pub(super) standalone_agent: bool,
}

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
    pub fn with_capacity_limits(mut self, config: &RuntimeConfig) -> Self {
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
    pub(super) fn resolve_working_dir(&self) -> std::path::PathBuf {
        self.skill_discovery_dir
            .as_deref()
            .map(std::path::PathBuf::from)
            .unwrap_or_else(|| std::env::current_dir().unwrap_or_default())
    }

    /// Merge control-plane skills with locally discovered skills.
    ///
    /// Control-plane skills always take precedence. Local skills are only added
    /// if discovery is enabled and no control-plane skill shares the same id.
    pub(super) async fn merge_with_discovered_skills(
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
}
