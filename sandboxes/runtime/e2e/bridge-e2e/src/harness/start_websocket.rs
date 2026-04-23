use anyhow::{anyhow, Context, Result};
use std::collections::HashMap;
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use super::TestHarness;

impl TestHarness {
    /// Start the harness with WebSocket event streaming enabled.
    ///
    /// If `with_webhooks` is true, webhooks are also enabled (dual delivery).
    /// If false, only WebSocket is used (webhook URL is not set).
    pub async fn start_with_websocket(with_webhooks: bool) -> Result<Self> {
        let workspace_root = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .and_then(|p| p.parent())
            .ok_or_else(|| anyhow!("cannot determine workspace root"))?
            .to_path_buf();

        let target_dir = workspace_root.join("target").join("debug");

        // 1. Build both binaries
        let build_status = Command::new("cargo")
            .args(["build", "-p", "mock-control-plane", "-p", "bridge"])
            .current_dir(&workspace_root)
            .stdout(Stdio::null())
            .stderr(Stdio::inherit())
            .status()
            .context("failed to run cargo build")?;

        if !build_status.success() {
            return Err(anyhow!("cargo build failed with status {}", build_status));
        }

        let cp_binary = target_dir.join("mock-control-plane");
        let bridge_binary = target_dir.join("bridge");

        let fixtures_dir = workspace_root.join("fixtures").join("agents");

        // 2. Start mock control plane
        let mut cp_process = Command::new(&cp_binary)
            .args([
                "--port",
                "0",
                "--fixtures-dir",
                fixtures_dir.to_str().unwrap(),
            ])
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()
            .context("failed to start mock-control-plane")?;

        let cp_stdout = cp_process
            .stdout
            .take()
            .ok_or_else(|| anyhow!("cannot capture mock-control-plane stdout"))?;

        let (mock_cp_port, cp_drain) = Self::read_port_from_stdout(cp_stdout)?;
        let cp_base_url = format!("http://127.0.0.1:{}", mock_cp_port);

        tracing::info!(port = mock_cp_port, "mock control plane started (ws mode)");

        let _ = std::fs::create_dir_all("/tmp/workspace");

        // 3. Start bridge with WebSocket enabled
        let bridge_port = Self::find_free_port()?;
        let bridge_listen_addr = format!("127.0.0.1:{}", bridge_port);
        let bridge_base_url = format!("http://127.0.0.1:{}", bridge_port);

        let bridge_stdout_log = std::fs::File::create(
            std::env::temp_dir().join(format!("bridge-e2e-stdout-{}.log", bridge_port)),
        )
        .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());
        let bridge_stderr_log = std::fs::File::create(
            std::env::temp_dir().join(format!("bridge-e2e-stderr-{}.log", bridge_port)),
        )
        .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());

        eprintln!(
            "[harness] Bridge logs (ws): stdout={}/bridge-e2e-stdout-{}.log stderr={}/bridge-e2e-stderr-{}.log",
            std::env::temp_dir().display(), bridge_port,
            std::env::temp_dir().display(), bridge_port,
        );

        let mut bridge_command = Command::new(&bridge_binary);
        bridge_command
            .env("BRIDGE_CONTROL_PLANE_URL", &cp_base_url)
            .env("BRIDGE_CONTROL_PLANE_API_KEY", "e2e-test-key")
            .env("BRIDGE_LISTEN_ADDR", &bridge_listen_addr)
            .env("BRIDGE_LOG_LEVEL", "debug")
            .env("BRIDGE_WEBSOCKET_ENABLED", "true")
            .stdout(Stdio::from(bridge_stdout_log))
            .stderr(Stdio::from(bridge_stderr_log));

        if with_webhooks {
            bridge_command.env(
                "BRIDGE_WEBHOOK_URL",
                format!("{}/webhooks/receive", cp_base_url),
            );
        }

        for key in ["BRIDGE_STORAGE_PATH", "SSL_CERT_FILE", "SSL_CERT_DIR"] {
            if let Ok(value) = std::env::var(key) {
                bridge_command.env(key, value);
            }
        }

        let bridge_process = bridge_command.spawn().context("failed to start bridge")?;

        tracing::info!(
            port = bridge_port,
            with_webhooks = with_webhooks,
            "bridge process started (ws mode)"
        );

        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .pool_max_idle_per_host(0)
            .build()
            .context("failed to build reqwest client")?;

        let log_dir =
            std::env::temp_dir().join(format!("bridge-e2e-conversation-logs-{}", bridge_port));
        let _ = std::fs::remove_dir_all(&log_dir);
        let _ = std::fs::create_dir_all(&log_dir);

        let mut harness = Self {
            mock_cp_port,
            bridge_port,
            mock_cp_process: Some(cp_process),
            bridge_process: Some(bridge_process),
            client,
            bridge_base_url,
            cp_base_url,
            workspace_root,
            tool_log_dir: None,
            _cp_stdout_drain: Some(cp_drain),
            log_dir,
            conversation_agents: Mutex::new(HashMap::new()),
        };

        let health_timeout = Duration::from_secs(30);
        harness
            .wait_for_bridge_healthy_with_timeout(health_timeout)
            .await?;

        harness.push_agents_from_cp().await?;

        Ok(harness)
    }
}
