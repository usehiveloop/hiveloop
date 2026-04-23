use dashmap::DashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;

use super::{cache_hit_ratio, MetricsSnapshot, ToolCallStatsSnapshot};

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
    /// Total non-cached input tokens consumed (freshly computed, full price).
    pub input_tokens: AtomicU64,
    /// Total input tokens served from the provider prompt cache (discounted).
    pub cached_input_tokens: AtomicU64,
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
            cached_input_tokens: AtomicU64::new(0),
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
        let cached_input_tokens = self.cached_input_tokens.load(Ordering::Relaxed);
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

        let cache_hit_ratio = cache_hit_ratio(input_tokens, cached_input_tokens);

        MetricsSnapshot {
            agent_id: agent_id.to_string(),
            agent_name: agent_name.to_string(),
            input_tokens,
            cached_input_tokens,
            output_tokens,
            total_tokens: input_tokens + cached_input_tokens + output_tokens,
            cache_hit_ratio,
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
