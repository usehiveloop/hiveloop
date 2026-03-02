use serde::Serialize;
use std::sync::atomic::{AtomicU64, Ordering};

use crate::agent::AgentId;

/// Atomic counters for tracking per-agent metrics.
pub struct AgentMetrics {
    /// Total input tokens consumed
    pub input_tokens: AtomicU64,
    /// Total output tokens generated
    pub output_tokens: AtomicU64,
    /// Total number of LLM requests made
    pub total_requests: AtomicU64,
    /// Number of failed requests
    pub failed_requests: AtomicU64,
    /// Currently active conversations
    pub active_conversations: AtomicU64,
    /// Total conversations ever created
    pub total_conversations: AtomicU64,
    /// Total tool calls executed
    pub tool_calls: AtomicU64,
    /// Sum of latencies in milliseconds (for computing average)
    pub latency_sum_ms: AtomicU64,
    /// Number of latency measurements
    pub latency_count: AtomicU64,
}

impl AgentMetrics {
    /// Create a new AgentMetrics with all counters initialized to zero.
    pub fn new() -> Self {
        Self {
            input_tokens: AtomicU64::new(0),
            output_tokens: AtomicU64::new(0),
            total_requests: AtomicU64::new(0),
            failed_requests: AtomicU64::new(0),
            active_conversations: AtomicU64::new(0),
            total_conversations: AtomicU64::new(0),
            tool_calls: AtomicU64::new(0),
            latency_sum_ms: AtomicU64::new(0),
            latency_count: AtomicU64::new(0),
        }
    }

    /// Create a serializable snapshot of the current metrics.
    pub fn snapshot(&self, agent_id: &str, agent_name: &str) -> MetricsSnapshot {
        let input_tokens = self.input_tokens.load(Ordering::Relaxed);
        let output_tokens = self.output_tokens.load(Ordering::Relaxed);
        let latency_sum = self.latency_sum_ms.load(Ordering::Relaxed);
        let latency_count = self.latency_count.load(Ordering::Relaxed);
        let avg_latency_ms = if latency_count > 0 {
            latency_sum as f64 / latency_count as f64
        } else {
            0.0
        };

        MetricsSnapshot {
            agent_id: agent_id.to_string(),
            agent_name: agent_name.to_string(),
            input_tokens,
            output_tokens,
            total_tokens: input_tokens + output_tokens,
            total_requests: self.total_requests.load(Ordering::Relaxed),
            failed_requests: self.failed_requests.load(Ordering::Relaxed),
            active_conversations: self.active_conversations.load(Ordering::Relaxed),
            total_conversations: self.total_conversations.load(Ordering::Relaxed),
            tool_calls: self.tool_calls.load(Ordering::Relaxed),
            avg_latency_ms,
        }
    }
}

impl Default for AgentMetrics {
    fn default() -> Self {
        Self::new()
    }
}

/// Serializable snapshot of agent metrics for the /metrics endpoint.
#[derive(Debug, Clone, Serialize)]
pub struct MetricsSnapshot {
    /// Agent identifier
    pub agent_id: AgentId,
    /// Agent name
    pub agent_name: String,
    /// Total input tokens consumed
    pub input_tokens: u64,
    /// Total output tokens generated
    pub output_tokens: u64,
    /// Total tokens (input + output)
    pub total_tokens: u64,
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
}

/// Global metrics across all agents.
#[derive(Debug, Clone, Serialize)]
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
pub struct MetricsResponse {
    /// Timestamp of the snapshot
    pub timestamp: chrono::DateTime<chrono::Utc>,
    /// Per-agent metrics
    pub agents: Vec<MetricsSnapshot>,
    /// Global metrics
    pub global: GlobalMetrics,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_agent_metrics_new_initializes_to_zero() {
        let m = AgentMetrics::new();
        assert_eq!(m.input_tokens.load(Ordering::Relaxed), 0);
        assert_eq!(m.output_tokens.load(Ordering::Relaxed), 0);
        assert_eq!(m.total_requests.load(Ordering::Relaxed), 0);
        assert_eq!(m.failed_requests.load(Ordering::Relaxed), 0);
        assert_eq!(m.active_conversations.load(Ordering::Relaxed), 0);
        assert_eq!(m.total_conversations.load(Ordering::Relaxed), 0);
        assert_eq!(m.tool_calls.load(Ordering::Relaxed), 0);
        assert_eq!(m.latency_sum_ms.load(Ordering::Relaxed), 0);
        assert_eq!(m.latency_count.load(Ordering::Relaxed), 0);
    }

    #[test]
    fn test_snapshot_reads_atomic_values() {
        let m = AgentMetrics::new();
        m.input_tokens.store(100, Ordering::Relaxed);
        m.output_tokens.store(50, Ordering::Relaxed);
        m.total_requests.store(10, Ordering::Relaxed);

        let snap = m.snapshot("agent_1", "Test Agent");
        assert_eq!(snap.agent_id, "agent_1");
        assert_eq!(snap.agent_name, "Test Agent");
        assert_eq!(snap.input_tokens, 100);
        assert_eq!(snap.output_tokens, 50);
        assert_eq!(snap.total_requests, 10);
    }

    #[test]
    fn test_snapshot_total_tokens() {
        let m = AgentMetrics::new();
        m.input_tokens.store(200, Ordering::Relaxed);
        m.output_tokens.store(100, Ordering::Relaxed);

        let snap = m.snapshot("a", "A");
        assert_eq!(snap.total_tokens, 300);
    }

    #[test]
    fn test_avg_latency_ms_computes_correctly() {
        let m = AgentMetrics::new();
        m.latency_sum_ms.store(1000, Ordering::Relaxed);
        m.latency_count.store(4, Ordering::Relaxed);

        let snap = m.snapshot("a", "A");
        assert!((snap.avg_latency_ms - 250.0).abs() < f64::EPSILON);
    }

    #[test]
    fn test_avg_latency_ms_zero_when_no_measurements() {
        let m = AgentMetrics::new();
        let snap = m.snapshot("a", "A");
        assert!((snap.avg_latency_ms - 0.0).abs() < f64::EPSILON);
    }

    #[test]
    fn test_default_impl() {
        let m = AgentMetrics::default();
        assert_eq!(m.input_tokens.load(Ordering::Relaxed), 0);
    }
}
