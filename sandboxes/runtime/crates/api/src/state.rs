use dashmap::DashMap;
use llm::{PermissionManager, SseEvent};
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio::time::Instant;
use webhooks::WebhookContext;

/// Shared application state for all request handlers.
#[derive(Clone)]
pub struct AppState {
    /// The agent supervisor managing all agent lifecycles.
    pub supervisor: Arc<AgentSupervisor>,
    /// Server startup time for uptime calculations.
    pub startup_time: Instant,
    /// Active SSE streams keyed by conversation ID.
    ///
    /// Stores the SSE receiver so the stream handler can pick it up.
    pub sse_streams: Arc<DashMap<String, mpsc::Receiver<SseEvent>>>,
    /// API key for authenticating control plane push requests.
    pub control_plane_api_key: String,
    /// Optional webhook context for dispatching webhook events.
    pub webhook_ctx: Option<WebhookContext>,
    /// Shared permission manager for tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
}

impl AppState {
    /// Create a new application state.
    pub fn new(
        supervisor: Arc<AgentSupervisor>,
        control_plane_api_key: String,
        webhook_ctx: Option<WebhookContext>,
    ) -> Self {
        let permission_manager = supervisor.permission_manager();
        Self {
            supervisor,
            startup_time: Instant::now(),
            sse_streams: Arc::new(DashMap::new()),
            control_plane_api_key,
            webhook_ctx,
            permission_manager,
        }
    }
}
