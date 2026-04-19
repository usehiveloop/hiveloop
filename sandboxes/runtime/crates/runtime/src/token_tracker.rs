use bridge_core::metrics::ConversationMetrics;
use bridge_core::AgentMetrics;
use std::sync::atomic::Ordering;

/// Record a completed LLM request's token usage and latency.
///
/// `input_tokens` is the non-cached (full-price) token count.
/// `cached_input_tokens` is the count served from the provider prompt cache
/// at a discount — non-zero only when a cache hit occurred.
///
/// Writes to both the agent-level aggregate and per-conversation metrics.
pub fn record_request(
    metrics: &AgentMetrics,
    conv_metrics: Option<&ConversationMetrics>,
    input_tokens: u64,
    cached_input_tokens: u64,
    output_tokens: u64,
    latency_ms: u64,
) {
    metrics
        .input_tokens
        .fetch_add(input_tokens, Ordering::Relaxed);
    metrics
        .cached_input_tokens
        .fetch_add(cached_input_tokens, Ordering::Relaxed);
    metrics
        .output_tokens
        .fetch_add(output_tokens, Ordering::Relaxed);
    metrics.total_requests.fetch_add(1, Ordering::Relaxed);
    metrics
        .latency_sum_ms
        .fetch_add(latency_ms, Ordering::Relaxed);
    metrics.latency_count.fetch_add(1, Ordering::Relaxed);

    if let Some(cm) = conv_metrics {
        cm.record_turn(input_tokens, cached_input_tokens, output_tokens, latency_ms);
    }
}

/// Record a failed request.
pub fn record_error(metrics: &AgentMetrics) {
    metrics.failed_requests.fetch_add(1, Ordering::Relaxed);
}

/// Increment the active conversation count.
pub fn increment_active_conversations(metrics: &AgentMetrics) {
    metrics.active_conversations.fetch_add(1, Ordering::Relaxed);
}

/// Decrement the active conversation count.
pub fn decrement_active_conversations(metrics: &AgentMetrics) {
    metrics.active_conversations.fetch_sub(1, Ordering::Relaxed);
}

/// Increment the total conversation count.
pub fn increment_total_conversations(metrics: &AgentMetrics) {
    metrics.total_conversations.fetch_add(1, Ordering::Relaxed);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_record_request_increments_all_counters() {
        let metrics = AgentMetrics::new();
        record_request(&metrics, None, 100, 0, 200, 50);

        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 100);
        assert_eq!(metrics.cached_input_tokens.load(Ordering::Relaxed), 0);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 200);
        assert_eq!(metrics.total_requests.load(Ordering::Relaxed), 1);
        assert_eq!(metrics.latency_sum_ms.load(Ordering::Relaxed), 50);
        assert_eq!(metrics.latency_count.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_record_request_tracks_cache_reads() {
        let metrics = AgentMetrics::new();
        record_request(&metrics, None, 100, 900, 50, 80);

        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 100);
        assert_eq!(metrics.cached_input_tokens.load(Ordering::Relaxed), 900);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 50);
    }

    #[test]
    fn test_record_multiple_requests_accumulates() {
        let metrics = AgentMetrics::new();
        record_request(&metrics, None, 100, 0, 200, 50);
        record_request(&metrics, None, 50, 0, 100, 30);

        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 150);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 300);
        assert_eq!(metrics.total_requests.load(Ordering::Relaxed), 2);
        assert_eq!(metrics.latency_sum_ms.load(Ordering::Relaxed), 80);
        assert_eq!(metrics.latency_count.load(Ordering::Relaxed), 2);
    }

    #[test]
    fn test_record_multiple_requests_accumulates_cache_reads() {
        let metrics = AgentMetrics::new();
        record_request(&metrics, None, 100, 900, 50, 100);
        record_request(&metrics, None, 50, 450, 25, 80);
        record_request(&metrics, None, 20, 0, 10, 30); // cache miss on the third

        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 170);
        assert_eq!(metrics.cached_input_tokens.load(Ordering::Relaxed), 1350);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 85);
    }

    #[test]
    fn test_record_request_dual_write_conversation_metrics() {
        let metrics = AgentMetrics::new();
        let conv = ConversationMetrics::new(
            "conv-1".to_string(),
            "agent-1".to_string(),
            "claude-4".to_string(),
        );
        record_request(&metrics, Some(&conv), 100, 0, 200, 50);
        record_request(&metrics, Some(&conv), 50, 0, 100, 30);

        // Agent metrics
        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 150);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 300);

        // Conversation metrics
        assert_eq!(conv.input_tokens.load(Ordering::Relaxed), 150);
        assert_eq!(conv.output_tokens.load(Ordering::Relaxed), 300);
        assert_eq!(conv.total_turns.load(Ordering::Relaxed), 2);
        assert_eq!(conv.llm_latency_sum_ms.load(Ordering::Relaxed), 80);
    }

    #[test]
    fn test_record_request_dual_write_cache_reads() {
        let metrics = AgentMetrics::new();
        let conv = ConversationMetrics::new(
            "conv-2".to_string(),
            "agent-1".to_string(),
            "claude-4".to_string(),
        );
        record_request(&metrics, Some(&conv), 100, 900, 50, 100);

        assert_eq!(metrics.cached_input_tokens.load(Ordering::Relaxed), 900);
        assert_eq!(conv.cached_input_tokens.load(Ordering::Relaxed), 900);
    }

    #[test]
    fn test_record_error() {
        let metrics = AgentMetrics::new();
        record_error(&metrics);
        assert_eq!(metrics.failed_requests.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_active_conversations_increment_decrement() {
        let metrics = AgentMetrics::new();
        increment_active_conversations(&metrics);
        increment_active_conversations(&metrics);
        assert_eq!(metrics.active_conversations.load(Ordering::Relaxed), 2);

        decrement_active_conversations(&metrics);
        assert_eq!(metrics.active_conversations.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_total_conversations() {
        let metrics = AgentMetrics::new();
        increment_total_conversations(&metrics);
        increment_total_conversations(&metrics);
        increment_total_conversations(&metrics);
        assert_eq!(metrics.total_conversations.load(Ordering::Relaxed), 3);
    }
}
