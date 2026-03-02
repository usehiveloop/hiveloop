use dashmap::DashMap;
use llm::SseEvent;
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio::time::Instant;

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
}

impl AppState {
    /// Create a new application state.
    pub fn new(supervisor: Arc<AgentSupervisor>) -> Self {
        Self {
            supervisor,
            startup_time: Instant::now(),
            sse_streams: Arc::new(DashMap::new()),
        }
    }
}
