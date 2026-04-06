use bridge_e2e::TestHarness;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use storage::{SqliteBackend, StorageBackend, StorageConfig};

const TIMEOUT: Duration = Duration::from_secs(30);

fn unique_storage_path() -> String {
    let millis = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("clock before epoch")
        .as_millis();
    std::env::temp_dir()
        .join(format!("bridge-storage-integration-{millis}.db"))
        .display()
        .to_string()
}

fn configure_storage_env() -> Option<String> {
    let path = unique_storage_path();
    std::env::set_var("BRIDGE_STORAGE_PATH", &path);

    Some(path)
}

async fn cleanup_agents(path: &str, agent_ids: &[String]) {
    let backend = match SqliteBackend::new(&StorageConfig {
        path: path.to_string(),
    })
    .await
    {
        Ok(backend) => backend,
        Err(_) => return,
    };

    for agent_id in agent_ids {
        let _ = backend.delete_agent(agent_id).await;
    }
}

async fn load_conversation_ids(path: &str, agent_id: &str) -> Vec<String> {
    let backend = match SqliteBackend::new(&StorageConfig {
        path: path.to_string(),
    })
    .await
    {
        Ok(backend) => backend,
        Err(_) => return Vec::new(),
    };

    match backend.load_conversations(agent_id).await {
        Ok(records) => records.into_iter().map(|record| record.id).collect(),
        Err(_) => Vec::new(),
    }
}

async fn load_agent_ids(path: &str) -> Vec<String> {
    let backend = match SqliteBackend::new(&StorageConfig {
        path: path.to_string(),
    })
    .await
    {
        Ok(backend) => backend,
        Err(_) => return Vec::new(),
    };

    match backend.load_all_agents().await {
        Ok(records) => records.into_iter().map(|record| record.id).collect(),
        Err(_) => Vec::new(),
    }
}

#[tokio::test(flavor = "multi_thread")]
async fn test_storage_restores_conversation_after_restart() {
    let Some(path) = configure_storage_env() else {
        eprintln!("storage env not configured; skipping storage integration test");
        return;
    };

    let agent_ids: Vec<String>;
    let conversation_id: String;

    {
        let harness = TestHarness::start()
            .await
            .expect("failed to start first harness");

        let agents = harness.get_agents().await.expect("get_agents failed");
        agent_ids = agents
            .iter()
            .filter_map(|agent| agent.get("id").and_then(|v| v.as_str()))
            .map(|id| id.to_string())
            .collect();

        let response = harness
            .create_conversation("agent_simple")
            .await
            .expect("create_conversation failed");
        let body: serde_json::Value = response
            .json()
            .await
            .expect("parse create conversation body");
        conversation_id = body["conversation_id"]
            .as_str()
            .expect("conversation_id missing")
            .to_string();

        harness
            .send_message(&conversation_id, "Reply with exactly: first persisted turn")
            .await
            .expect("send_message failed");

        let (_events, text) = harness
            .stream_sse_until_done(&conversation_id, TIMEOUT)
            .await
            .expect("stream failed");
        assert!(!text.is_empty(), "first run should produce a response");
    }

    let stored_conversations = load_conversation_ids(&path, "agent_simple").await;
    assert!(
        stored_conversations.contains(&conversation_id),
        "conversation should be persisted before restart; got {:?}",
        stored_conversations
    );

    let stored_agents = load_agent_ids(&path).await;
    assert!(
        stored_agents
            .iter()
            .any(|agent_id| agent_id == "agent_simple"),
        "agent should be persisted before restart; got {:?}",
        stored_agents
    );

    {
        let harness = TestHarness::start()
            .await
            .expect("failed to start second harness");

        let accepted = harness
            .send_message(
                &conversation_id,
                "Reply with exactly: second turn after restart",
            )
            .await
            .expect("send_message after restart failed");
        assert_eq!(accepted.status().as_u16(), 202);

        let (_events, text) = harness
            .stream_sse_until_done(&conversation_id, TIMEOUT)
            .await
            .expect("stream after restart failed");
        assert!(
            !text.is_empty(),
            "restored conversation should continue after restart"
        );
    }

    cleanup_agents(&path, &agent_ids).await;
    let _ = std::fs::remove_file(&path);
}
