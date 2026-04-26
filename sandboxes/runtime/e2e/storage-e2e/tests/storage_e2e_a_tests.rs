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
async fn test_save_and_load_agents() {
    let backend = connect().await;
    let p = test_prefix();
    let id1 = format!("{p}_agent_1");
    let id2 = format!("{p}_agent_2");

    let agent1 = make_agent(&id1, "Agent One");
    let agent2 = make_agent(&id2, "Agent Two");

    backend.save_agent(&agent1).await.unwrap();
    backend.save_agent(&agent2).await.unwrap();

    let loaded = backend.load_all_agents().await.unwrap();
    let our_agents: Vec<_> = loaded.iter().filter(|a| a.id.starts_with(&p)).collect();
    assert_eq!(our_agents.len(), 2);

    let a1 = our_agents.iter().find(|a| a.id == id1).unwrap();
    assert_eq!(a1.name, "Agent One");
    assert_eq!(a1.system_prompt, "You are a test agent.");

    // Cleanup
    backend.delete_agent(&id1).await.unwrap();
    backend.delete_agent(&id2).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_delete_agent_cascades() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_cascade");
    let conv_id = format!("{p}_conv1");
    let task_id = format!("{p}_task1");

    let agent = make_agent(&agent_id, "Cascade Agent");
    backend.save_agent(&agent).await.unwrap();

    backend
        .create_conversation(&agent_id, &conv_id, None, Utc::now())
        .await
        .unwrap();
    backend
        .append_message(&conv_id, 0, &make_message(Role::User, "hello"))
        .await
        .unwrap();
    backend
        .save_session(&task_id, &agent_id, b"{}")
        .await
        .unwrap();

    // Delete agent — should cascade
    backend.delete_agent(&agent_id).await.unwrap();

    let convs = backend.load_conversations(&agent_id).await.unwrap();
    assert!(convs.is_empty());

    let sessions = backend.load_sessions(&agent_id).await.unwrap();
    assert!(sessions.is_empty());
}

#[tokio::test(flavor = "multi_thread")]
async fn test_conversation_lifecycle() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_conv_agent");
    let conv_id = format!("{p}_conv_lc");

    let agent = make_agent(&agent_id, "Conv Agent");
    backend.save_agent(&agent).await.unwrap();

    backend
        .create_conversation(&agent_id, &conv_id, Some("Test Conv"), Utc::now())
        .await
        .unwrap();

    backend
        .append_message(&conv_id, 0, &make_message(Role::User, "Hello"))
        .await
        .unwrap();
    backend
        .append_message(&conv_id, 1, &make_message(Role::Assistant, "Hi there!"))
        .await
        .unwrap();
    backend
        .append_message(&conv_id, 2, &make_message(Role::User, "How are you?"))
        .await
        .unwrap();

    let convs = backend.load_conversations(&agent_id).await.unwrap();
    assert_eq!(convs.len(), 1);

    let conv = &convs[0];
    assert_eq!(conv.id, conv_id);
    assert_eq!(conv.title.as_deref(), Some("Test Conv"));
    assert_eq!(conv.messages.len(), 3);
    assert_eq!(conv.messages[0].role, Role::User);
    assert_eq!(conv.messages[1].role, Role::Assistant);
    assert_eq!(conv.messages[2].role, Role::User);

    if let ContentBlock::Text { text } = &conv.messages[0].content[0] {
        assert_eq!(text, "Hello");
    } else {
        panic!("expected text content block");
    }

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_message_compression_roundtrip() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_compress");
    let conv_id = format!("{p}_conv_comp");

    let agent = make_agent(&agent_id, "Compress Agent");
    backend.save_agent(&agent).await.unwrap();
    backend
        .create_conversation(&agent_id, &conv_id, None, Utc::now())
        .await
        .unwrap();

    let large_text = "x".repeat(100_000);
    let msg = make_message(Role::Assistant, &large_text);
    backend.append_message(&conv_id, 0, &msg).await.unwrap();

    let convs = backend.load_conversations(&agent_id).await.unwrap();
    assert_eq!(convs[0].messages.len(), 1);
    if let ContentBlock::Text { text } = &convs[0].messages[0].content[0] {
        assert_eq!(text.len(), 100_000);
        assert_eq!(text, &large_text);
    } else {
        panic!("expected text content block");
    }

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}

#[tokio::test(flavor = "multi_thread")]
async fn test_replace_messages_after_compaction() {
    let backend = connect().await;
    let p = test_prefix();
    let agent_id = format!("{p}_compact");
    let conv_id = format!("{p}_conv_cmp");

    let agent = make_agent(&agent_id, "Compact Agent");
    backend.save_agent(&agent).await.unwrap();
    backend
        .create_conversation(&agent_id, &conv_id, None, Utc::now())
        .await
        .unwrap();

    for i in 0..20 {
        backend
            .append_message(&conv_id, i, &make_message(Role::User, &format!("msg {i}")))
            .await
            .unwrap();
    }

    let compacted: Vec<Message> = (0..5)
        .map(|i| make_message(Role::Assistant, &format!("summary {i}")))
        .collect();
    backend
        .replace_messages(&conv_id, &compacted)
        .await
        .unwrap();

    let convs = backend.load_conversations(&agent_id).await.unwrap();
    assert_eq!(convs[0].messages.len(), 5);
    if let ContentBlock::Text { text } = &convs[0].messages[0].content[0] {
        assert_eq!(text, "summary 0");
    }

    // Cleanup
    backend.delete_agent(&agent_id).await.unwrap();
}
