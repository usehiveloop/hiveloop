use bridge_e2e::TestHarness;
use std::path::PathBuf;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use storage::{LibSqlBackend, StorageBackend, StorageConfig};

const TIMEOUT: Duration = Duration::from_secs(30);

fn workspace_root() -> PathBuf {
    std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|p| p.parent())
        .expect("cannot determine workspace root")
        .to_path_buf()
}

fn load_env_file_vars() -> std::collections::HashMap<String, String> {
    let env_path = workspace_root().join(".env");
    let mut vars = std::collections::HashMap::new();
    let Ok(contents) = std::fs::read_to_string(env_path) else {
        return vars;
    };

    for line in contents.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        if let Some((key, value)) = line.split_once('=') {
            vars.insert(
                key.trim().to_string(),
                value.trim().trim_matches('"').to_string(),
            );
        }
    }

    vars
}

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

fn configure_storage_env() -> Option<(String, String, String)> {
    let file_vars = load_env_file_vars();
    let url = std::env::var("BRIDGE_STORAGE_URL")
        .ok()
        .or_else(|| file_vars.get("BRIDGE_STORAGE_URL").cloned())?;
    let auth_token = std::env::var("BRIDGE_STORAGE_AUTH_TOKEN")
        .ok()
        .or_else(|| file_vars.get("BRIDGE_STORAGE_AUTH_TOKEN").cloned())?;

    let path = unique_storage_path();
    std::env::set_var("BRIDGE_STORAGE_URL", &url);
    std::env::set_var("BRIDGE_STORAGE_AUTH_TOKEN", &auth_token);
    std::env::set_var("BRIDGE_STORAGE_PATH", &path);
    std::env::set_var("BRIDGE_STORAGE_SYNC_INTERVAL_SECS", "5");
    std::env::set_var("SSL_CERT_FILE", "/etc/ssl/cert.pem");
    std::env::set_var("SSL_CERT_DIR", "/etc/ssl/certs");

    Some((url, auth_token, path))
}

async fn cleanup_agents(url: &str, auth_token: &str, path: &str, agent_ids: &[String]) {
    let backend = match LibSqlBackend::new(&StorageConfig {
        url: url.to_string(),
        auth_token: auth_token.to_string(),
        path: path.to_string(),
        sync_interval_secs: 5,
        encryption_key: None,
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

async fn load_conversation_ids(url: &str, auth_token: &str, path: &str, agent_id: &str) -> Vec<String> {
    let backend = match LibSqlBackend::new(&StorageConfig {
        url: url.to_string(),
        auth_token: auth_token.to_string(),
        path: path.to_string(),
        sync_interval_secs: 5,
        encryption_key: None,
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

async fn load_agent_ids(url: &str, auth_token: &str, path: &str) -> Vec<String> {
    let backend = match LibSqlBackend::new(&StorageConfig {
        url: url.to_string(),
        auth_token: auth_token.to_string(),
        path: path.to_string(),
        sync_interval_secs: 5,
        encryption_key: None,
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
    let Some((url, auth_token, path)) = configure_storage_env() else {
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

    let stored_conversations = load_conversation_ids(&url, &auth_token, &path, "agent_simple").await;
    assert!(
        stored_conversations.contains(&conversation_id),
        "conversation should be persisted before restart; got {:?}",
        stored_conversations
    );

    let stored_agents = load_agent_ids(&url, &auth_token, &path).await;
    assert!(
        stored_agents.iter().any(|agent_id| agent_id == "agent_simple"),
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

    cleanup_agents(&url, &auth_token, &path, &agent_ids).await;
    let _ = std::fs::remove_file(path);
}
