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
async fn test_concurrent_writes() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_concurrent");

    let agent = make_agent(&agent_id, "Concurrent Agent");
    backend.save_agent(&agent).await.unwrap();

    let mut handles = Vec::new();
    for i in 0..10 {
        let b = backend.clone();
        let aid = agent_id.clone();
        let prefix = p.clone();
        handles.push(tokio::spawn(async move {
            let conv_id = format!("{prefix}_cc_{i}");
            b.create_conversation(&aid, &conv_id, None, Utc::now())
                .await
                .unwrap();
            for j in 0..5u64 {
                b.append_message(
                    &conv_id,
                    j,
                    &make_message(Role::User, &format!("msg {i}-{j}")),
                )
                .await
                .unwrap();
            }
        }));
    }

    for h in handles {
        h.await.unwrap();
    }

    let convs = backend.load_conversations(&agent_id).await.unwrap();
    assert_eq!(convs.len(), 10);
    for conv in &convs {
        assert_eq!(conv.messages.len(), 5);
    }

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_agent_upsert() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_upsert");

    let mut agent = make_agent(&agent_id, "Original Name");
    backend.save_agent(&agent).await.unwrap();

    agent.name = "Updated Name".to_string();
    agent.system_prompt = "Updated prompt".to_string();
    backend.save_agent(&agent).await.unwrap();

    let loaded = backend.load_all_agents().await.unwrap();
    let ours = loaded.iter().find(|a| a.id == agent_id).unwrap();
    assert_eq!(ours.name, "Updated Name");
    assert_eq!(ours.system_prompt, "Updated prompt");

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_delete_conversation() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_del_conv");
    let conv1 = format!("{p}_cd1");
    let conv2 = format!("{p}_cd2");

    let agent = make_agent(&agent_id, "Del Conv Agent");
    backend.save_agent(&agent).await.unwrap();

    backend
        .create_conversation(&agent_id, &conv1, None, Utc::now())
        .await
        .unwrap();
    backend
        .create_conversation(&agent_id, &conv2, None, Utc::now())
        .await
        .unwrap();

    backend
        .append_message(&conv1, 0, &make_message(Role::User, "hello"))
        .await
        .unwrap();

    backend.delete_conversation(&conv1).await.unwrap();

    let convs = backend.load_conversations(&agent_id).await.unwrap();
    assert_eq!(convs.len(), 1);
    assert_eq!(convs[0].id, conv2);

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}
