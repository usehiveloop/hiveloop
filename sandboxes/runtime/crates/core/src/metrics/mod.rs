use serde::{Deserialize, Serialize};

use crate::agent::AgentId;

mod agent_metrics;
mod conversation_metrics;

pub use agent_metrics::{AgentMetrics, ToolCallStats};
pub use conversation_metrics::{ConversationMetrics, ConversationMetricsSnapshot};

/// Compute cache hit ratio as `cached / (cached + fresh)`.
///
/// Returns 0.0 when no input tokens have been observed. This ignores output
/// tokens entirely — hit rate is an input-side metric.
pub fn cache_hit_ratio(input_tokens: u64, cached_input_tokens: u64) -> f64 {
    let total = input_tokens + cached_input_tokens;
    if total == 0 {
        0.0
    } else {
        cached_input_tokens as f64 / total as f64
    }
}

/// Serializable snapshot of per-tool call statistics.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ToolCallStatsSnapshot {
    /// Tool name
    pub tool_name: String,
    /// Total number of calls to this tool
    pub total_calls: u64,
    /// Number of successful completions
    pub successes: u64,
    /// Number of failed completions
    pub failures: u64,
    /// Success rate (successes / total_calls)
    pub success_rate: f64,
    /// Average latency in milliseconds
    pub avg_latency_ms: f64,
}

/// Serializable snapshot of agent metrics for the /metrics endpoint.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct MetricsSnapshot {
    /// Agent identifier
    pub agent_id: AgentId,
    /// Agent name
    pub agent_name: String,
    /// Total non-cached input tokens consumed (full price).
    pub input_tokens: u64,
    /// Total input tokens served from the provider prompt cache (discounted).
    #[serde(default)]
    pub cached_input_tokens: u64,
    /// Total output tokens generated
    pub output_tokens: u64,
    /// Total tokens (fresh input + cached input + output)
    pub total_tokens: u64,
    /// cached_input_tokens / (input_tokens + cached_input_tokens).
    #[serde(default)]
    pub cache_hit_ratio: f64,
    /// Total LLM requests
    pub total_requests: u64,
    /// Failed requests
    pub failed_requests: u64,
    /// Currently active conversations
    pub active_conversations: u64,
    /// Total conversations ever created
    pub total_conversations: u64,
    /// Total tool calls executed
    pub tool_calls: u64,
    /// Average latency in milliseconds
    pub avg_latency_ms: f64,
    /// Per-tool call metrics
    pub tool_call_details: Vec<ToolCallStatsSnapshot>,
}

/// Global metrics across all agents.
#[derive(Debug, Clone, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct GlobalMetrics {
    /// Total number of loaded agents
    pub total_agents: usize,
    /// Total active conversations across all agents
    pub total_active_conversations: u64,
    /// Seconds since the runtime started
    pub uptime_secs: u64,
}

/// Complete metrics response for GET /metrics.
#[derive(Debug, Clone, Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct MetricsResponse {
    /// Timestamp of the snapshot
    pub timestamp: chrono::DateTime<chrono::Utc>,
    /// Per-agent metrics
    pub agents: Vec<MetricsSnapshot>,
    /// Global metrics
    pub global: GlobalMetrics,
}

#[cfg(test)]
mod tests;
