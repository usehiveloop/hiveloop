use bridge_core::AgentMetrics;
use std::sync::atomic::Ordering;

/// Record a completed LLM request's token usage and latency.
pub fn record_request(
    metrics: &AgentMetrics,
    input_tokens: u64,
    output_tokens: u64,
    latency_ms: u64,
) {
    metrics
        .input_tokens
        .fetch_add(input_tokens, Ordering::Relaxed);
    metrics
        .output_tokens
        .fetch_add(output_tokens, Ordering::Relaxed);
    metrics.total_requests.fetch_add(1, Ordering::Relaxed);
    metrics
        .latency_sum_ms
        .fetch_add(latency_ms, Ordering::Relaxed);
    metrics.latency_count.fetch_add(1, Ordering::Relaxed);
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
        record_request(&metrics, 100, 200, 50);

        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 100);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 200);
        assert_eq!(metrics.total_requests.load(Ordering::Relaxed), 1);
        assert_eq!(metrics.latency_sum_ms.load(Ordering::Relaxed), 50);
        assert_eq!(metrics.latency_count.load(Ordering::Relaxed), 1);
    }

    #[test]
    fn test_record_multiple_requests_accumulates() {
        let metrics = AgentMetrics::new();
        record_request(&metrics, 100, 200, 50);
        record_request(&metrics, 50, 100, 30);

        assert_eq!(metrics.input_tokens.load(Ordering::Relaxed), 150);
        assert_eq!(metrics.output_tokens.load(Ordering::Relaxed), 300);
        assert_eq!(metrics.total_requests.load(Ordering::Relaxed), 2);
        assert_eq!(metrics.latency_sum_ms.load(Ordering::Relaxed), 80);
        assert_eq!(metrics.latency_count.load(Ordering::Relaxed), 2);
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
