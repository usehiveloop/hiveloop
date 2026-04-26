#![allow(dead_code, unused_imports, unused_mut, unused_variables)]
use bridge_core::{
    AgentDefinition, BridgeEvent, BridgeEventType, ContentBlock, Message, MetricsSnapshot, Role,
    ToolCallStatsSnapshot,
};
use chrono::Utc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use storage::{SqliteBackend, StorageBackend, StorageHandle};

static COUNTER: AtomicU64 = AtomicU64::new(0);

/// Generate a unique prefix for each test to avoid cross-contamination.
fn test_prefix() -> String {
    let n = COUNTER.fetch_add(1, Ordering::SeqCst);
    let ts = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    format!("t{ts}_{n}")
}

/// Create a local SQLite backend for testing.
async fn connect() -> Arc<SqliteBackend> {
    let config = storage::StorageConfig {
        path: format!("/tmp/storage_e2e_{}.db", test_prefix()),
    };

    Arc::new(
        SqliteBackend::new(&config)
            .await
            .expect("failed to open test database"),
    )
}

fn make_agent(id: &str, name: &str) -> AgentDefinition {
    serde_json::from_value(serde_json::json!({
        "id": id,
        "name": name,
        "system_prompt": "You are a test agent.",
        "provider": {
            "provider_type": "open_ai",
            "model": "gpt-4o",
            "api_key": "test-key"
        }
    }))
    .unwrap()
}

fn make_message(role: Role, text: &str) -> Message {
    Message {
        role,
        content: vec![ContentBlock::Text {
            text: text.to_string(),
        }],
        timestamp: Utc::now(),
        system_reminder: None,
    }
}

fn make_metrics(agent_id: &str) -> MetricsSnapshot {
    MetricsSnapshot {
        agent_id: agent_id.to_string(),
        agent_name: "Test Agent".to_string(),
        input_tokens: 100,
        cached_input_tokens: 0,
        output_tokens: 50,
        total_tokens: 150,
        cache_hit_ratio: 0.0,
        total_requests: 10,
        failed_requests: 1,
        active_conversations: 2,
        total_conversations: 5,
        tool_calls: 20,
        avg_latency_ms: 123.45,
        tool_call_details: vec![ToolCallStatsSnapshot {
            tool_name: "bash".to_string(),
            total_calls: 15,
            successes: 14,
            failures: 1,
            failure_results: 0,
            success_rate: 14.0 / 15.0,
            avg_latency_ms: 100.0,
        }],
    }
}

fn make_event(agent_id: &str, conv_id: &str, seq: u64) -> BridgeEvent {
    let mut event = BridgeEvent::new(
        BridgeEventType::ToolCallCompleted,
        agent_id,
        conv_id,
        serde_json::json!({"tool_name": "bash", "result": "ok"}),
    );
    event.sequence_number = seq;
    event
}

// ── Tests ──────────────────────────────────────────────────────

#[tokio::test(flavor = "multi_thread")]
async fn test_event_outbox_lifecycle() {
    let backend = connect().await;
    let p = test_prefix();

    let e1 = make_event(&format!("{p}_wh"), &format!("{p}_conv"), 1);
    let e2 = make_event(&format!("{p}_wh"), &format!("{p}_conv"), 2);
    let e3 = make_event(&format!("{p}_wh"), &format!("{p}_conv"), 3);

    let id1 = backend.enqueue_event(&e1).await.unwrap();
    let id2 = backend.enqueue_event(&e2).await.unwrap();
    let id3 = backend.enqueue_event(&e3).await.unwrap();

    backend.mark_webhook_delivered(&id1).await.unwrap();

    let pending = backend.load_pending_events().await.unwrap();
    let our_pending: Vec<_> = pending
        .iter()
        .filter(|e| e.event_id == id2 || e.event_id == id3)
        .collect();
    assert_eq!(our_pending.len(), 2);

    // Test load_events_since
    let since = backend.load_events_since(1, 100).await.unwrap();
    let our_since: Vec<_> = since
        .iter()
        .filter(|e| e.event_id == id2 || e.event_id == id3)
        .collect();
    assert_eq!(our_since.len(), 2);

    // Cleanup
    backend.mark_webhook_delivered(&id2).await.unwrap();
    backend.mark_webhook_delivered(&id3).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_session_store_persistence() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_sess");

    let history1 = serde_json::to_vec(&serde_json::json!({"messages": ["hello"]})).unwrap();
    let history2 = serde_json::to_vec(&serde_json::json!({"messages": ["world"]})).unwrap();

    backend
        .save_session(&format!("{p}_task_a"), &agent_id, &history1)
        .await
        .unwrap();
    backend
        .save_session(&format!("{p}_task_b"), &agent_id, &history2)
        .await
        .unwrap();

    let sessions = backend.load_sessions(&agent_id).await.unwrap();
    assert_eq!(sessions.len(), 2);

    // Cleanup
    backend.delete_sessions_for_agent(&agent_id).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_delete_sessions_for_agent() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_a = format!("{p}_agent_a");
    let agent_b = format!("{p}_agent_b");

    backend
        .save_session(&format!("{p}_tx"), &agent_a, b"{}")
        .await
        .unwrap();
    backend
        .save_session(&format!("{p}_ty"), &agent_b, b"{}")
        .await
        .unwrap();

    backend.delete_sessions_for_agent(&agent_a).await.unwrap();

    let a_sessions = backend.load_sessions(&agent_a).await.unwrap();
    assert!(a_sessions.is_empty());

    let b_sessions = backend.load_sessions(&agent_b).await.unwrap();
    assert_eq!(b_sessions.len(), 1);

    // Cleanup
    backend.delete_sessions_for_agent(&agent_b).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_metrics_snapshot_persistence() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_met");

    backend
        .save_metrics_snapshot(&agent_id, &make_metrics(&agent_id))
        .await
        .unwrap();
    backend
        .save_metrics_snapshot(&agent_id, &make_metrics(&agent_id))
        .await
        .unwrap();
    backend
        .save_metrics_snapshot(&agent_id, &make_metrics(&agent_id))
        .await
        .unwrap();

    // No load_metrics method yet — verify no errors during save.
}

#[tokio::test(flavor = "multi_thread")]
async fn test_large_agent_definition() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_large");

    let large_prompt = "x".repeat(50_000);
    let mut agent = make_agent(&agent_id, "Large Agent");
    agent.system_prompt = large_prompt.clone();

    backend.save_agent(&agent).await.unwrap();

    let loaded = backend.load_all_agents().await.unwrap();
    let ours = loaded.iter().find(|a| a.id == agent_id).unwrap();
    assert_eq!(ours.system_prompt, large_prompt);

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_empty_db_hydration() {
    let backend = connect().await;
    let p = test_prefix();

    let convs = backend
        .load_conversations(&format!("{p}_nonexistent"))
        .await
        .unwrap();
    assert!(convs.is_empty());

    let sessions = backend
        .load_sessions(&format!("{p}_nonexistent"))
        .await
        .unwrap();
    assert!(sessions.is_empty());
}

#[tokio::test(flavor = "multi_thread")]
async fn test_flush_guarantees_persistence() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_flush");

    let (tx, rx) = tokio::sync::mpsc::unbounded_channel();
    let handle = StorageHandle::new(tx);

    let writer_backend = backend.clone() as Arc<dyn StorageBackend>;
    tokio::spawn(storage::writer::run_writer(rx, writer_backend));

    let agent = make_agent(&agent_id, "Flush Agent");
    handle.save_agent(agent);

    handle.flush().await;

    let loaded = backend.load_all_agents().await.unwrap();
    let ours = loaded.iter().find(|a| a.id == agent_id).unwrap();
    assert_eq!(ours.name, "Flush Agent");

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}
