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
        output_tokens: 50,
        total_tokens: 150,
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
