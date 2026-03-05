mod mock_llm;
mod routes;
mod store;

use axum::routing::{delete, get, post, put};
use axum::Router;
use std::sync::Arc;
use tokio::net::TcpListener;

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt().with_env_filter("info").init();

    let args: Vec<String> = std::env::args().collect();

    let port: u16 = args
        .iter()
        .position(|a| a == "--port")
        .and_then(|i| args.get(i + 1))
        .and_then(|p| p.parse().ok())
        .unwrap_or(0); // 0 = random available port

    let fixtures_dir = args
        .iter()
        .position(|a| a == "--fixtures-dir")
        .and_then(|i| args.get(i + 1).cloned())
        .unwrap_or_else(|| "fixtures/agents".to_string());

    let fireworks_key = args
        .iter()
        .position(|a| a == "--fireworks-key")
        .and_then(|i| args.get(i + 1).cloned());

    let mock_portal_mcp_path = args
        .iter()
        .position(|a| a == "--mock-portal-mcp-path")
        .and_then(|i| args.get(i + 1).cloned());

    let workspace_dir = args
        .iter()
        .position(|a| a == "--workspace-dir")
        .and_then(|i| args.get(i + 1).cloned());

    let mock_store = Arc::new(store::MockStore::new());

    // Load fixture agents if the directory exists
    if let Ok(entries) = std::fs::read_dir(&fixtures_dir) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.extension().is_some_and(|e| e == "json") {
                match std::fs::read_to_string(&path) {
                    Ok(contents) => {
                        // If Fireworks key is provided, replace placeholders in raw JSON
                        // before deserializing
                        let contents = if let Some(ref key) = fireworks_key {
                            let mut c = contents;
                            c = c.replace("PLACEHOLDER_FIREWORKS_KEY", key);
                            if let Some(ref mcp_path) = mock_portal_mcp_path {
                                c = c.replace("PLACEHOLDER_MOCK_PORTAL_MCP_PATH", mcp_path);
                            }
                            if let Some(ref ws_dir) = workspace_dir {
                                c = c.replace("PLACEHOLDER_WORKSPACE_DIR", ws_dir);
                            }
                            // Generate a unique log path per agent (use filename stem)
                            let stem = path
                                .file_stem()
                                .and_then(|s| s.to_str())
                                .unwrap_or("unknown");
                            let log_dir = std::env::temp_dir().join("portal-mcp-logs");
                            let _ = std::fs::create_dir_all(&log_dir);
                            let log_path = log_dir.join(format!("{stem}.jsonl"));
                            c = c.replace("PLACEHOLDER_LOG_PATH", &log_path.to_string_lossy());
                            c
                        } else {
                            contents
                        };

                        match serde_json::from_str::<bridge_core::AgentDefinition>(&contents) {
                            Ok(agent) => {
                                mock_store.create_agent(agent.clone());
                                tracing::info!(
                                    agent_id = %agent.id,
                                    file = %path.display(),
                                    "loaded fixture agent"
                                );
                            }
                            Err(e) => {
                                tracing::warn!(
                                    file = %path.display(),
                                    error = %e,
                                    "failed to parse fixture agent"
                                );
                            }
                        }
                    }
                    Err(e) => {
                        tracing::warn!(
                            file = %path.display(),
                            error = %e,
                            "failed to read fixture file"
                        );
                    }
                }
            }
        }
    }

    let app = Router::new()
        // Agent CRUD
        .route("/agents", get(routes::list_agents))
        .route("/agents", post(routes::create_agent))
        .route("/agents/{id}", get(routes::get_agent))
        .route("/agents/{id}", put(routes::update_agent))
        .route("/agents/{id}", delete(routes::delete_agent))
        .route(
            "/agents/{agent_id}/skills/{skill_id}",
            get(routes::get_skill),
        )
        // Webhook logging
        .route("/webhooks/receive", post(routes::receive_webhook))
        .route("/webhooks/log", get(routes::get_webhook_log))
        .route("/webhooks/log", delete(routes::clear_webhook_log))
        // Mock search
        .route("/search", post(routes::mock_search))
        // Mock LLM
        .route("/v1/chat/completions", post(mock_llm::chat_completions))
        .with_state(mock_store);

    let addr = format!("127.0.0.1:{}", port);
    let listener = TcpListener::bind(&addr)
        .await
        .expect("failed to bind listener");

    let actual_port = listener.local_addr().expect("no local addr").port();

    // Print port to stdout so the test harness can read it
    println!("PORT={}", actual_port);

    tracing::info!(port = actual_port, "mock control plane listening");

    axum::serve(listener, app).await.expect("server error");
}
