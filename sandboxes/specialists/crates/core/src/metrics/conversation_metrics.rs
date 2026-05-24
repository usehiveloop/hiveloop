use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicU64, Ordering};

use super::cache_hit_ratio;

/// Per-conversation metrics for tracking token usage and tool calls.
///
/// Created at conversation start, populated during the conversation,
/// and snapshotted at conversation end for webhook payloads.
pub struct ConversationMetrics {
    /// Conversation identifier.
    pub conversation_id: String,
    /// Agent identifier.
    pub agent_id: String,
    /// LLM model name (for cost attribution by consumers).
    pub model: String,
    /// Total non-cached input tokens consumed across all turns.
    pub input_tokens: AtomicU64,
    /// Total input tokens served from the provider prompt cache (discounted).
    pub cached_input_tokens: AtomicU64,
    /// Total output tokens generated across all turns.
    pub output_tokens: AtomicU64,
    /// Number of LLM turns completed.
    pub total_turns: AtomicU64,
    /// Total tool calls executed.
    pub tool_calls: AtomicU64,
    /// Sum of LLM response latencies in milliseconds.
    pub llm_latency_sum_ms: AtomicU64,
    /// Sum of tool call latencies in milliseconds.
    pub tool_latency_sum_ms: AtomicU64,
    /// When the conversation started.
    pub started_at: chrono::DateTime<chrono::Utc>,
}

impl ConversationMetrics {
    pub fn new(conversation_id: String, agent_id: String, model: String) -> Self {
        Self {
            conversation_id,
            agent_id,
            model,
            input_tokens: AtomicU64::new(0),
            cached_input_tokens: AtomicU64::new(0),
            output_tokens: AtomicU64::new(0),
            total_turns: AtomicU64::new(0),
            tool_calls: AtomicU64::new(0),
            llm_latency_sum_ms: AtomicU64::new(0),
            tool_latency_sum_ms: AtomicU64::new(0),
            started_at: chrono::Utc::now(),
        }
    }

    /// Record a completed LLM turn's token usage and latency.
    ///
    /// `input_tokens` is the non-cached (full-price) token count; `cached_input_tokens`
    /// is the count served from the provider prompt cache at a discount.
    pub fn record_turn(
        &self,
        input_tokens: u64,
        cached_input_tokens: u64,
        output_tokens: u64,
        latency_ms: u64,
    ) {
        self.input_tokens.fetch_add(input_tokens, Ordering::Relaxed);
        self.cached_input_tokens
            .fetch_add(cached_input_tokens, Ordering::Relaxed);
        self.output_tokens
            .fetch_add(output_tokens, Ordering::Relaxed);
        self.total_turns.fetch_add(1, Ordering::Relaxed);
        self.llm_latency_sum_ms
            .fetch_add(latency_ms, Ordering::Relaxed);
    }

    /// Record a tool call's latency.
    pub fn record_tool_call(&self, latency_ms: u64) {
        self.tool_calls.fetch_add(1, Ordering::Relaxed);
        self.tool_latency_sum_ms
            .fetch_add(latency_ms, Ordering::Relaxed);
    }

    /// Create a serializable snapshot of the current conversation metrics.
    pub fn snapshot(&self) -> ConversationMetricsSnapshot {
        let input_tokens = self.input_tokens.load(Ordering::Relaxed);
        let cached_input_tokens = self.cached_input_tokens.load(Ordering::Relaxed);
        let output_tokens = self.output_tokens.load(Ordering::Relaxed);
        ConversationMetricsSnapshot {
            conversation_id: self.conversation_id.clone(),
            agent_id: self.agent_id.clone(),
            model: self.model.clone(),
            input_tokens,
            cached_input_tokens,
            output_tokens,
            total_tokens: input_tokens + cached_input_tokens + output_tokens,
            cache_hit_ratio: cache_hit_ratio(input_tokens, cached_input_tokens),
            total_turns: self.total_turns.load(Ordering::Relaxed),
            tool_calls: self.tool_calls.load(Ordering::Relaxed),
            started_at: self.started_at,
            ended_at: chrono::Utc::now(),
            duration_ms: (chrono::Utc::now() - self.started_at).num_milliseconds() as u64,
        }
    }
}

/// Serializable snapshot of per-conversation metrics.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConversationMetricsSnapshot {
    pub conversation_id: String,
    pub agent_id: String,
    pub model: String,
    pub input_tokens: u64,
    #[serde(default)]
    pub cached_input_tokens: u64,
    pub output_tokens: u64,
    pub total_tokens: u64,
    #[serde(default)]
    pub cache_hit_ratio: f64,
    pub total_turns: u64,
    pub tool_calls: u64,
    pub started_at: chrono::DateTime<chrono::Utc>,
    pub ended_at: chrono::DateTime<chrono::Utc>,
    pub duration_ms: u64,
}
