//! Bridge supervisor wired to the ACP harness adapter.
//!
//! Bridge enforces one agent per instance. Pushing a different agent id
//! when one is already loaded returns a 409 Conflict. Delegates conversation
//! lifecycle into the [`harness`] crate.

use bridge_core::mcp::McpServerDefinition;
use bridge_core::metrics::MetricsSnapshot;
use bridge_core::{AgentDefinition, AgentSummary, BridgeError, RuntimeConfig};
use harness::AcpSession;
use std::sync::Arc;
use storage::{StorageBackend, StorageHandle};
use tokio::sync::{Mutex, RwLock};
use tokio_util::sync::CancellationToken;
use tracing::info;
use webhooks::{EventBus, PermissionManager};

use crate::agent_map::AgentMap;
use crate::agent_state::AgentState;

/// Central supervisor for the single agent this instance runs.
pub struct AgentSupervisor {
    pub(super) agent_map: AgentMap,
    pub(super) cancel: CancellationToken,
    pub(super) event_bus: Option<Arc<EventBus>>,
    pub(super) permission_manager: Arc<PermissionManager>,
    pub(super) storage: Option<StorageHandle>,
    pub(super) storage_backend: Option<Arc<dyn StorageBackend>>,
    /// The single ACP harness adapter for this agent. `None` until an
    /// agent is pushed.
    harness: RwLock<Option<HarnessSlot>>,
    /// Lock used to serialize push/diff/upsert operations on the agent slot.
    push_lock: Mutex<()>,
}

struct HarnessSlot {
    agent_id: String,
    session: Arc<AcpSession>,
}

impl AgentSupervisor {
    pub fn new(cancel: CancellationToken) -> Self {
        Self {
            agent_map: AgentMap::new(),
            cancel,
            event_bus: None,
            permission_manager: Arc::new(PermissionManager::new()),
            storage: None,
            storage_backend: None,
            harness: RwLock::new(None),
            push_lock: Mutex::new(()),
        }
    }

    pub fn with_event_bus(mut self, event_bus: Option<Arc<EventBus>>) -> Self {
        self.event_bus = event_bus;
        self
    }

    pub fn with_storage(mut self, storage: Option<StorageHandle>) -> Self {
        self.storage = storage;
        self
    }

    pub fn with_storage_backend(mut self, backend: Option<Arc<dyn StorageBackend>>) -> Self {
        self.storage_backend = backend;
        self
    }

    pub fn with_capacity_limits(self, _config: &RuntimeConfig) -> Self {
        self
    }

    pub fn permission_manager(&self) -> Arc<PermissionManager> {
        self.permission_manager.clone()
    }

    pub fn get_agent(&self, agent_id: &str) -> Option<Arc<AgentState>> {
        self.agent_map.get(agent_id)
    }

    pub async fn list_agents(&self) -> Vec<AgentSummary> {
        self.agent_map.list().await
    }

    pub fn list_agent_states(&self) -> Vec<Arc<AgentState>> {
        self.agent_map.list_states()
    }

    pub fn agent_count(&self) -> usize {
        self.agent_map.len()
    }

    /// Bulk-load agents. Enforces the one-agent-per-instance rule.
    pub async fn load_agents(&self, defs: Vec<AgentDefinition>) -> Result<(), BridgeError> {
        if defs.len() > 1 {
            return Err(BridgeError::Conflict(format!(
                "bridge accepts at most one agent per instance; got {}",
                defs.len()
            )));
        }
        for def in defs {
            self.upsert_definition(def).await?;
        }
        Ok(())
    }

    /// Apply a control-plane diff. Honors the one-agent rule.
    pub async fn apply_diff(
        &self,
        added: Vec<AgentDefinition>,
        updated: Vec<AgentDefinition>,
        removed: Vec<String>,
    ) -> Result<(), BridgeError> {
        let _push = self.push_lock.lock().await;

        // Removals first.
        for id in removed {
            self.agent_map.remove(&id);
            if let Some(storage) = &self.storage {
                storage.delete_agent(id.clone());
            }
            let mut slot = self.harness.write().await;
            if slot.as_ref().is_some_and(|s| s.agent_id == id) {
                if let Some(s) = slot.as_ref() {
                    s.session.shutdown().await;
                }
                *slot = None;
            }
        }

        let mut all = added;
        all.extend(updated);
        if all.is_empty() {
            return Ok(());
        }
        if all.len() > 1 {
            return Err(BridgeError::Conflict(format!(
                "bridge accepts at most one agent per instance; got {}",
                all.len()
            )));
        }
        let def = all.into_iter().next().unwrap();
        drop(_push);
        self.upsert_definition(def).await
    }

    pub async fn update_agent_api_key(
        &self,
        agent_id: &str,
        api_key: String,
    ) -> Result<(), BridgeError> {
        let agent = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;
        {
            let mut def = agent.definition.write().await;
            def.provider.api_key = api_key;
        }
        let updated = agent.definition.read().await.clone();
        if let Some(storage) = &self.storage {
            storage.save_agent(updated.clone());
        }
        if let Some(slot) = self.harness.read().await.as_ref() {
            slot.session.set_definition(updated).await;
        }
        Ok(())
    }

    /// Create a new conversation. The SSE broadcast channel is registered
    /// inside the harness; subscribers attach later via
    /// `EventBus::subscribe_sse(conversation_id)`.
    pub async fn create_conversation(
        &self,
        agent_id: &str,
        api_key_override: Option<String>,
        provider_override: Option<bridge_core::ProviderConfig>,
        per_conversation_mcp: Option<Vec<McpServerDefinition>>,
    ) -> Result<String, BridgeError> {
        if self.agent_map.get(agent_id).is_none() {
            return Err(BridgeError::AgentNotFound(agent_id.to_string()));
        }
        let slot = self.harness.read().await;
        let slot = slot.as_ref().ok_or(BridgeError::HarnessUnavailable)?;
        if slot.agent_id != agent_id {
            return Err(BridgeError::AgentNotFound(agent_id.to_string()));
        }
        let ctx = slot
            .session
            .create_conversation(api_key_override, provider_override, per_conversation_mcp)
            .await?;

        // Track the conversation on the AgentState so the API's
        // `find_agent_for_conversation` can find it, and persist a row
        // so a stop/start of the bridge process can restore it.
        let created_at = chrono::Utc::now();
        if let Some(agent) = self.agent_map.get(agent_id) {
            agent.conversations.insert(
                ctx.conversation_id.clone(),
                crate::agent_state::ConversationHandle {
                    id: ctx.conversation_id.clone(),
                    created_at,
                },
            );
        }
        if let Some(storage) = &self.storage {
            storage.create_conversation(
                agent_id.to_string(),
                ctx.conversation_id.clone(),
                None,
                created_at,
            );
        }

        Ok(ctx.conversation_id)
    }

    pub async fn send_message(
        &self,
        _agent_id: &str,
        conv_id: &str,
        content: String,
        system_reminder: Option<String>,
    ) -> Result<(), BridgeError> {
        let slot = self.harness.read().await;
        let slot = slot.as_ref().ok_or(BridgeError::HarnessUnavailable)?;
        slot.session
            .send_message(conv_id, content, system_reminder)
            .await
    }

    pub fn end_conversation(&self, agent_id: &str, conv_id: &str) -> Result<(), BridgeError> {
        if let Some(agent) = self.agent_map.get(agent_id) {
            agent.conversations.remove(conv_id);
        }
        if let Some(storage) = &self.storage {
            storage.delete_conversation(conv_id.to_string());
        }
        let session_opt = self
            .harness
            .try_read()
            .ok()
            .and_then(|s| s.as_ref().map(|x| x.session.clone()));
        if let Some(session) = session_opt {
            let conv_id = conv_id.to_string();
            tokio::spawn(async move { session.end(&conv_id).await });
        }
        Ok(())
    }

    pub async fn abort_conversation(
        &self,
        _agent_id: &str,
        conv_id: &str,
    ) -> Result<(), BridgeError> {
        let slot = self.harness.read().await;
        let slot = slot.as_ref().ok_or(BridgeError::HarnessUnavailable)?;
        slot.session.abort(conv_id).await
    }

    /// Resume a previously-created conversation by id. Used at bridge boot
    /// time to restore sessions after a `docker stop` / `docker start`.
    /// Re-registers the SSE broadcast on the EventBus; subscribers attach
    /// independently via `EventBus::subscribe_sse`.
    pub async fn restore_conversation(
        &self,
        agent_id: &str,
        conv_id: &str,
    ) -> Result<(), BridgeError> {
        let slot = self.harness.read().await;
        let slot = slot.as_ref().ok_or(BridgeError::HarnessUnavailable)?;
        if slot.agent_id != agent_id {
            return Err(BridgeError::AgentNotFound(agent_id.to_string()));
        }
        let ctx = slot.session.restore_conversation(conv_id).await?;
        if let Some(agent) = self.agent_map.get(agent_id) {
            agent.conversations.insert(
                ctx.conversation_id.clone(),
                crate::agent_state::ConversationHandle {
                    id: ctx.conversation_id.clone(),
                    created_at: chrono::Utc::now(),
                },
            );
        }
        Ok(())
    }

    pub async fn collect_metrics(&self) -> Vec<MetricsSnapshot> {
        Vec::new()
    }

    pub async fn shutdown(&self) {
        let mut slot = self.harness.write().await;
        if let Some(s) = slot.take() {
            s.session.shutdown().await;
        }
        self.cancel.cancel();
        info!("supervisor shutdown");
    }

    async fn upsert_definition(&self, def: AgentDefinition) -> Result<(), BridgeError> {
        let id = def.id.clone();

        // One-agent rule.
        {
            let map_ids = self.agent_map.agent_ids();
            if map_ids.len() == 1 && map_ids[0] != id {
                return Err(BridgeError::Conflict(format!(
                    "bridge already runs agent '{}'; cannot accept '{}'",
                    map_ids[0], id
                )));
            }
        }

        if let Some(storage) = &self.storage {
            storage.save_agent(def.clone());
        }

        if let Some(existing) = self.agent_map.get(&id) {
            *existing.definition.write().await = def.clone();
        } else {
            self.agent_map
                .insert(id.clone(), Arc::new(AgentState::new(def.clone())));
        }

        // Spawn or update the harness adapter.
        let mut slot = self.harness.write().await;
        if let Some(existing_slot) = slot.as_ref() {
            if existing_slot.agent_id == id {
                existing_slot.session.set_definition(def.clone()).await;
                return Ok(());
            }
            existing_slot.session.shutdown().await;
            *slot = None;
        }

        let event_bus = self
            .event_bus
            .clone()
            .ok_or_else(|| BridgeError::Internal("event_bus not configured".into()))?;
        let permission_manager = self.permission_manager.clone();

        let session = harness::spawn(def, event_bus, permission_manager).await?;

        *slot = Some(HarnessSlot {
            agent_id: id,
            session,
        });
        Ok(())
    }
}
