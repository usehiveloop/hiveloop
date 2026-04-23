use anyhow::{anyhow, Context, Result};
use std::collections::HashMap;
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use super::TestHarness;

impl TestHarness {
    /// Start with real agents and Fireworks. Requires FIREWORKS_API_KEY env.
    /// Builds: bridge, mock-control-plane, mock-portal-mcp.
    /// Loads real agent fixtures from e2e/fixtures/real-agents/.
    pub async fn start_real() -> Result<Self> {
        let fireworks_key = std::env::var("FIREWORKS_API_KEY")
            .context("FIREWORKS_API_KEY environment variable not set")?;

        let workspace_root = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .and_then(|p| p.parent())
            .ok_or_else(|| anyhow!("cannot determine workspace root"))?
            .to_path_buf();

        let target_dir = workspace_root.join("target").join("debug");

        // 1. Build all three binaries
        let build_status = Command::new("cargo")
            .args([
                "build",
                "-p",
                "mock-control-plane",
                "-p",
                "mock-portal-mcp",
                "-p",
                "bridge",
            ])
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
        let mcp_binary = target_dir.join("mock-portal-mcp");

        for (name, path) in [
            ("mock-control-plane", &cp_binary),
            ("bridge", &bridge_binary),
            ("mock-portal-mcp", &mcp_binary),
        ] {
            if !path.exists() {
                return Err(anyhow!("{} binary not found at {}", name, path.display()));
            }
        }

        let fixtures_dir = workspace_root
            .join("e2e")
            .join("fixtures")
            .join("real-agents");
        let tool_log_dir = std::env::temp_dir().join("portal-mcp-logs");
        let _ = std::fs::remove_dir_all(&tool_log_dir);
        let _ = std::fs::create_dir_all(&tool_log_dir);

        // 2. Start mock control plane with real agent fixtures and Fireworks
        let mut cp_process = Command::new(&cp_binary)
            .args([
                "--port",
                "0",
                "--fixtures-dir",
                fixtures_dir.to_str().unwrap(),
                "--fireworks-key",
                &fireworks_key,
                "--mock-portal-mcp-path",
                mcp_binary.to_str().unwrap(),
                "--workspace-dir",
                workspace_root.to_str().unwrap(),
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

        tracing::info!(
            port = mock_cp_port,
            "mock control plane started (real agents)"
        );

        // 3. Start bridge
        let bridge_port = Self::find_free_port()?;
        let bridge_listen_addr = format!("127.0.0.1:{}", bridge_port);
        let bridge_base_url = format!("http://127.0.0.1:{}", bridge_port);

        // Redirect bridge stdout+stderr to per-instance files instead of piping.
        // CRITICAL: if stdout is piped but never read, the pipe buffer fills up
        // (~64KB on macOS) and blocks the bridge process when it writes logs,
        // which deadlocks the async runtime.
        // Use bridge_port in the filename so parallel tests don't overwrite each other.
        let bridge_stdout_log = std::fs::File::create(
            std::env::temp_dir().join(format!("bridge-e2e-stdout-{}.log", bridge_port)),
        )
        .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());
        let bridge_stderr_log = std::fs::File::create(
            std::env::temp_dir().join(format!("bridge-e2e-stderr-{}.log", bridge_port)),
        )
        .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());

        eprintln!(
            "[harness] Bridge logs: stdout={}/bridge-e2e-stdout-{}.log stderr={}/bridge-e2e-stderr-{}.log",
            std::env::temp_dir().display(), bridge_port,
            std::env::temp_dir().display(), bridge_port,
        );

        let mut bridge_command = Command::new(&bridge_binary);
        bridge_command
            .env("BRIDGE_CONTROL_PLANE_URL", &cp_base_url)
            .env("BRIDGE_CONTROL_PLANE_API_KEY", "e2e-test-key")
            .env("BRIDGE_LISTEN_ADDR", &bridge_listen_addr)
            .env("BRIDGE_LOG_LEVEL", "debug")
            .env("SEARCH_ENDPOINT", format!("{}/search", &cp_base_url))
            .env(
                "BRIDGE_WEBHOOK_URL",
                format!("{}/webhooks/receive", cp_base_url),
            )
            .stdout(Stdio::from(bridge_stdout_log))
            .stderr(Stdio::from(bridge_stderr_log));

        for key in ["BRIDGE_STORAGE_PATH", "SSL_CERT_FILE", "SSL_CERT_DIR"] {
            if let Ok(value) = std::env::var(key) {
                bridge_command.env(key, value);
            }
        }

        let bridge_process = bridge_command.spawn().context("failed to start bridge")?;

        tracing::info!(port = bridge_port, "bridge process started (real agents)");

        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .pool_max_idle_per_host(0)
            .build()
            .context("failed to build reqwest client")?;

        let log_dir =
            std::env::temp_dir().join(format!("bridge-e2e-conversation-logs-{}", bridge_port));
        let _ = std::fs::remove_dir_all(&log_dir);
        let _ = std::fs::create_dir_all(&log_dir);
        eprintln!("[harness] Conversation logs: {}", log_dir.display());

        let mut harness = Self {
            mock_cp_port,
            bridge_port,
            mock_cp_process: Some(cp_process),
            bridge_process: Some(bridge_process),
            client,
            bridge_base_url,
            cp_base_url,
            workspace_root,
            tool_log_dir: Some(tool_log_dir),
            _cp_stdout_drain: Some(cp_drain),
            log_dir,
            conversation_agents: Mutex::new(HashMap::new()),
        };

        // 4. Poll /health until 200 (max 60s for real agents — MCP connections take longer)
        harness
            .wait_for_bridge_healthy_with_timeout(Duration::from_secs(60))
            .await?;

        // 5. Push agents from mock CP to bridge
        harness.push_agents_from_cp().await?;

        // 6. Wait for agents to be loaded (MCP connections take longer)
        harness.wait_for_agents_loaded(8).await?;

        Ok(harness)
    }
}
