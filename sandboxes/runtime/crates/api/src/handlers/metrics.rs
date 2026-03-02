use axum::extract::State;
use axum::Json;
use bridge_core::{GlobalMetrics, MetricsResponse};

use crate::state::AppState;

/// GET /metrics — collect and return metrics from all agents.
pub async fn get_metrics(State(state): State<AppState>) -> Json<MetricsResponse> {
    let agent_metrics = state.supervisor.collect_metrics();

    let total_active: u64 = agent_metrics.iter().map(|m| m.active_conversations).sum();

    let response = MetricsResponse {
        timestamp: chrono::Utc::now(),
        agents: agent_metrics,
        global: GlobalMetrics {
            total_agents: state.supervisor.agent_count(),
            total_active_conversations: total_active,
            uptime_secs: state.startup_time.elapsed().as_secs(),
        },
    };

    Json(response)
}
