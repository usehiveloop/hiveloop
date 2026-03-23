use dashmap::DashMap;
use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use crate::agent::AgentId;

/// Atomic counters for a single tool's call statistics.
pub struct ToolCallStats {
    /// Total number of calls to this tool
    pub total_calls: AtomicU64,
    /// Number of successful completions
    pub successes: AtomicU64,
    /// Number of failed completions
    pub failures: AtomicU64,
    /// Sum of latencies in milliseconds (for computing average)
    pub latency_sum_ms: AtomicU64,
    /// Number of latency measurements
    pub latency_count: AtomicU64,
}

impl ToolCallStats {
    pub fn new() -> Self {
        Self {
            total_calls: AtomicU64::new(0),
            successes: AtomicU64::new(0),
            failures: AtomicU64::new(0),
            latency_sum_ms: AtomicU64::new(0),
            latency_count: AtomicU64::new(0),
        }
    }
}

impl Default for ToolCallStats {
    fn default() -> Self {
        Self::new()
    }
}

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
    /// Per-tool-name metrics. Key = tool name, value = stats.
    pub tool_metrics: DashMap<String, Arc<ToolCallStats>>,
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
            tool_metrics: DashMap::new(),
        }
    }

    /// Record a completed tool call with name, success/failure, and latency.
    ///
    /// Uses a read-first pattern: the common path (tool already seen) takes
    /// only a DashMap read lock and an Arc clone. The write lock + String
    /// allocation only happens on the first call for a given tool name.
    pub fn record_tool_call_detailed(&self, tool_name: &str, is_error: bool, latency_ms: u64) {
        // Bump the global tool_calls counter
        self.tool_calls.fetch_add(1, Ordering::Relaxed);

        // Fast path: read lock only (no String allocation)
        let stats = if let Some(existing) = self.tool_metrics.get(tool_name) {
            existing.clone()
        } else {
            // Slow path: first time seeing this tool — allocate and insert
            self.tool_metrics
                .entry(tool_name.to_string())
                .or_insert_with(|| Arc::new(ToolCallStats::new()))
                .clone()
        };

        stats.total_calls.fetch_add(1, Ordering::Relaxed);
        if is_error {
            stats.failures.fetch_add(1, Ordering::Relaxed);
        } else {
            stats.successes.fetch_add(1, Ordering::Relaxed);
        }
        stats
            .latency_sum_ms
            .fetch_add(latency_ms, Ordering::Relaxed);
        stats.latency_count.fetch_add(1, Ordering::Relaxed);
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

        let mut tool_call_details: Vec<ToolCallStatsSnapshot> = self
            .tool_metrics
            .iter()
            .map(|entry| {
                let name = entry.key().clone();
                let stats = entry.value();
                let total = stats.total_calls.load(Ordering::Relaxed);
                let successes = stats.successes.load(Ordering::Relaxed);
                let failures = stats.failures.load(Ordering::Relaxed);
                let lat_sum = stats.latency_sum_ms.load(Ordering::Relaxed);
                let lat_count = stats.latency_count.load(Ordering::Relaxed);
                let success_rate = if total > 0 {
                    successes as f64 / total as f64
                } else {
                    0.0
                };
                let avg_lat = if lat_count > 0 {
                    lat_sum as f64 / lat_count as f64
                } else {
                    0.0
                };
                ToolCallStatsSnapshot {
                    tool_name: name,
                    total_calls: total,
                    successes,
                    failures,
                    success_rate,
                    avg_latency_ms: avg_lat,
                }
            })
            .collect();

        // Sort by tool name for deterministic output
        tool_call_details.sort_by(|a, b| a.tool_name.cmp(&b.tool_name));

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
            tool_call_details,
        }
    }
}

impl Default for AgentMetrics {
    fn default() -> Self {
        Self::new()
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
        assert!(m.tool_metrics.is_empty());
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

    #[test]
    fn test_record_tool_call_detailed_success() {
        let m = AgentMetrics::new();
        m.record_tool_call_detailed("bash", false, 100);

        assert_eq!(m.tool_calls.load(Ordering::Relaxed), 1);
        let stats = m.tool_metrics.get("bash").unwrap();
        assert_eq!(stats.total_calls.load(Ordering::Relaxed), 1);
        assert_eq!(stats.successes.load(Ordering::Relaxed), 1);
        assert_eq!(stats.failures.load(Ordering::Relaxed), 0);
        assert_eq!(stats.latency_sum_ms.load(Ordering::Relaxed), 100);
        assert_eq!(stats.latency_count.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_record_tool_call_detailed_failure() {
        let m = AgentMetrics::new();
        m.record_tool_call_detailed("edit", true, 5);

        assert_eq!(m.tool_calls.load(Ordering::Relaxed), 1);
        let stats = m.tool_metrics.get("edit").unwrap();
        assert_eq!(stats.total_calls.load(Ordering::Relaxed), 1);
        assert_eq!(stats.successes.load(Ordering::Relaxed), 0);
        assert_eq!(stats.failures.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_record_tool_call_detailed_multiple_tools() {
        let m = AgentMetrics::new();
        m.record_tool_call_detailed("bash", false, 100);
        m.record_tool_call_detailed("bash", false, 200);
        m.record_tool_call_detailed("bash", true, 50);
        m.record_tool_call_detailed("read", false, 10);

        assert_eq!(m.tool_calls.load(Ordering::Relaxed), 4);

        let bash = m.tool_metrics.get("bash").unwrap();
        assert_eq!(bash.total_calls.load(Ordering::Relaxed), 3);
        assert_eq!(bash.successes.load(Ordering::Relaxed), 2);
        assert_eq!(bash.failures.load(Ordering::Relaxed), 1);
        assert_eq!(bash.latency_sum_ms.load(Ordering::Relaxed), 350);

        let read = m.tool_metrics.get("read").unwrap();
        assert_eq!(read.total_calls.load(Ordering::Relaxed), 1);
        assert_eq!(read.successes.load(Ordering::Relaxed), 1);
        assert_eq!(read.failures.load(Ordering::Relaxed), 0);
    }

    #[test]
    fn test_snapshot_includes_tool_call_details() {
        let m = AgentMetrics::new();
        m.record_tool_call_detailed("bash", false, 100);
        m.record_tool_call_detailed("bash", false, 200);
        m.record_tool_call_detailed("bash", true, 50);
        m.record_tool_call_detailed("read", false, 10);

        let snap = m.snapshot("a", "A");
        assert_eq!(snap.tool_call_details.len(), 2);
        assert_eq!(snap.tool_calls, 4);

        // Sorted by tool_name, so bash first
        let bash = &snap.tool_call_details[0];
        assert_eq!(bash.tool_name, "bash");
        assert_eq!(bash.total_calls, 3);
        assert_eq!(bash.successes, 2);
        assert_eq!(bash.failures, 1);
        assert!((bash.success_rate - 2.0 / 3.0).abs() < 1e-10);
        assert!((bash.avg_latency_ms - 350.0 / 3.0).abs() < 1e-10);

        let read = &snap.tool_call_details[1];
        assert_eq!(read.tool_name, "read");
        assert_eq!(read.total_calls, 1);
        assert_eq!(read.successes, 1);
        assert_eq!(read.failures, 0);
        assert!((read.success_rate - 1.0).abs() < f64::EPSILON);
        assert!((read.avg_latency_ms - 10.0).abs() < f64::EPSILON);
    }

    #[test]
    fn test_snapshot_empty_tool_call_details() {
        let m = AgentMetrics::new();
        let snap = m.snapshot("a", "A");
        assert!(snap.tool_call_details.is_empty());
    }
}
