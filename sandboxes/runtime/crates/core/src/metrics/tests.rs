use super::*;
use std::sync::atomic::Ordering;

#[test]
fn test_agent_metrics_new_initializes_to_zero() {
    let m = AgentMetrics::new();
    assert_eq!(m.input_tokens.load(Ordering::Relaxed), 0);
    assert_eq!(m.cached_input_tokens.load(Ordering::Relaxed), 0);
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
fn test_cache_hit_ratio_zero_when_no_tokens() {
    assert!((cache_hit_ratio(0, 0) - 0.0).abs() < f64::EPSILON);
}

#[test]
fn test_cache_hit_ratio_zero_when_all_fresh() {
    assert!((cache_hit_ratio(1000, 0) - 0.0).abs() < f64::EPSILON);
}

#[test]
fn test_cache_hit_ratio_one_when_all_cached() {
    assert!((cache_hit_ratio(0, 1000) - 1.0).abs() < f64::EPSILON);
}

#[test]
fn test_cache_hit_ratio_mixed() {
    // 900 cached / (100 fresh + 900 cached) = 0.9
    assert!((cache_hit_ratio(100, 900) - 0.9).abs() < f64::EPSILON);
}

#[test]
fn test_agent_metrics_snapshot_includes_cache_fields() {
    let m = AgentMetrics::new();
    m.input_tokens.store(100, Ordering::Relaxed);
    m.cached_input_tokens.store(900, Ordering::Relaxed);
    m.output_tokens.store(50, Ordering::Relaxed);

    let snap = m.snapshot("a", "A");
    assert_eq!(snap.input_tokens, 100);
    assert_eq!(snap.cached_input_tokens, 900);
    assert_eq!(snap.output_tokens, 50);
    // total_tokens now counts fresh + cached + output
    assert_eq!(snap.total_tokens, 1050);
    assert!((snap.cache_hit_ratio - 0.9).abs() < f64::EPSILON);
}

#[test]
fn test_conversation_metrics_record_turn_accumulates_cache_tokens() {
    let c = ConversationMetrics::new("c".into(), "a".into(), "m".into());
    c.record_turn(100, 900, 50, 100);
    c.record_turn(50, 450, 25, 80);

    assert_eq!(c.input_tokens.load(Ordering::Relaxed), 150);
    assert_eq!(c.cached_input_tokens.load(Ordering::Relaxed), 1350);
    assert_eq!(c.output_tokens.load(Ordering::Relaxed), 75);
    assert_eq!(c.total_turns.load(Ordering::Relaxed), 2);

    let snap = c.snapshot();
    assert_eq!(snap.input_tokens, 150);
    assert_eq!(snap.cached_input_tokens, 1350);
    assert_eq!(snap.total_tokens, 150 + 1350 + 75);
    assert!((snap.cache_hit_ratio - 1350.0 / 1500.0).abs() < f64::EPSILON);
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
