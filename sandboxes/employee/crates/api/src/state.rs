use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use agent::cloud_agents::CloudTaskIndex;
use async_trait::async_trait;
use domain::{ConfigStore, OutboundChannelSpec, SessionId};
use mcp::McpRegistry;
use observability::ObservabilityRecorder;
use serde_json::Value;
use skills::SkillWriter;
use storage::{ConfigRepo, EventRepo, SessionRepo};
use tokio::sync::Notify;

use crate::http_gateway::HttpGatewayState;

#[derive(Clone)]
pub struct ApiState {
    pub config_store: ConfigStore,
    pub config_repo: Arc<dyn ConfigRepo>,
    pub session_repo: Arc<dyn SessionRepo>,
    pub event_repo: Arc<dyn EventRepo>,
    pub bearer_token: Arc<String>,
    pub gateway_ready: Arc<AtomicBool>,
    pub config_loaded: Arc<AtomicBool>,
    pub config_notify: Arc<Notify>,
    pub skill_writer: Arc<SkillWriter>,
    pub http_gateway: Option<HttpGatewayState>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
    pub cloud_task_index: Option<Arc<CloudTaskIndex>>,
    pub cloud_agent_callback_deliverer: Option<Arc<dyn CloudAgentCallbackDeliverer>>,
    pub observability: ObservabilityRecorder,
}

#[async_trait]
pub trait OutboundConfigReloader: Send + Sync {
    async fn reload_outbound_channels(&self, specs: &[OutboundChannelSpec]) -> anyhow::Result<()>;
}

#[async_trait]
pub trait CloudAgentCallbackDeliverer: Send + Sync {
    async fn deliver_cloud_agent_callback(
        &self,
        session_id: &SessionId,
        payload: Value,
    ) -> anyhow::Result<()>;
}

impl ApiState {
    pub fn new(
        config_store: ConfigStore,
        config_repo: Arc<dyn ConfigRepo>,
        session_repo: Arc<dyn SessionRepo>,
        event_repo: Arc<dyn EventRepo>,
        bearer_token: String,
        skill_writer: Arc<SkillWriter>,
        http_gateway: Option<HttpGatewayState>,
        mcp_registry: Option<Arc<McpRegistry>>,
        outbound_reloader: Option<Arc<dyn OutboundConfigReloader>>,
        cloud_task_index: Option<Arc<CloudTaskIndex>>,
        cloud_agent_callback_deliverer: Option<Arc<dyn CloudAgentCallbackDeliverer>>,
    ) -> Self {
        let observability = http_gateway
            .as_ref()
            .map(|gateway| gateway.broker.observability())
            .unwrap_or_default();
        Self {
            config_store,
            config_repo,
            session_repo,
            event_repo,
            bearer_token: Arc::new(bearer_token),
            gateway_ready: Arc::new(AtomicBool::new(false)),
            config_loaded: Arc::new(AtomicBool::new(false)),
            config_notify: Arc::new(Notify::new()),
            skill_writer,
            http_gateway,
            mcp_registry,
            outbound_reloader,
            cloud_task_index,
            cloud_agent_callback_deliverer,
            observability,
        }
    }

    pub fn mark_gateway_ready(&self) {
        self.gateway_ready.store(true, Ordering::Release);
    }

    pub fn mark_config_loaded(&self) {
        if !self.config_loaded.swap(true, Ordering::AcqRel) {
            self.config_notify.notify_waiters();
        }
    }

    pub async fn wait_for_config_loaded(&self) {
        loop {
            if self.config_loaded.load(Ordering::Acquire) {
                return;
            }
            let notified = self.config_notify.notified();
            if self.config_loaded.load(Ordering::Acquire) {
                return;
            }
            notified.await;
        }
    }

    pub fn is_ready(&self) -> bool {
        self.gateway_ready.load(Ordering::Acquire) && self.config_loaded.load(Ordering::Acquire)
    }
}
