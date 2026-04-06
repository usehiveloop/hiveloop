use bridge_core::event::BridgeEvent;
use dashmap::DashMap;
use llm::PermissionManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use storage::StorageBackend;
use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;
use webhooks::EventBus;

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
    pub sse_streams: Arc<DashMap<String, mpsc::Receiver<BridgeEvent>>>,
    /// API key for authenticating control plane push requests.
    pub control_plane_api_key: String,
    /// Optional storage backend for startup and restore reads.
    pub storage_backend: Option<Arc<dyn StorageBackend>>,
    /// Shared permission manager for tool approval requests.
    pub permission_manager: Arc<PermissionManager>,
    /// Unified event bus for SSE, WebSocket, webhook, and persistence delivery.
    pub event_bus: Arc<EventBus>,
    /// Global cancellation token for graceful shutdown.
    pub cancel: CancellationToken,
}

impl AppState {
    /// Create a new application state.
    pub fn new(
        supervisor: Arc<AgentSupervisor>,
        control_plane_api_key: String,
        storage_backend: Option<Arc<dyn StorageBackend>>,
        cancel: CancellationToken,
        event_bus: Arc<EventBus>,
    ) -> Self {
        let permission_manager = supervisor.permission_manager();
        Self {
            supervisor,
            startup_time: Instant::now(),
            sse_streams: Arc::new(DashMap::new()),
            control_plane_api_key,
            storage_backend,
            permission_manager,
            event_bus,
            cancel,
        }
    }
}
