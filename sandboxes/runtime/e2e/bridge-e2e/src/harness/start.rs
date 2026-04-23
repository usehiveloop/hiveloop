use anyhow::{anyhow, Context, Result};
use std::collections::HashMap;
use std::process::{Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

use super::TestHarness;

impl TestHarness {
    /// Build and start the mock control plane and bridge processes.
    ///
    /// 1. Builds both binaries via `cargo build`.
    /// 2. Starts the mock control plane on a random port and reads PORT= from stdout.
    /// 3. Starts the bridge pointing at the mock control plane on a random port.
    /// 4. Polls the bridge /health endpoint until it responds 200 (max 30s).
    pub async fn start() -> Result<Self> {
        Self::start_with_extra_env(&[]).await
    }

    /// Same as `start()` but allows the caller to inject extra environment variables
    /// into the spawned bridge process. Used by per-conversation MCP tests which need
    /// to flip `BRIDGE_ALLOW_STDIO_MCP_FROM_API=true` for that specific test run.
    pub async fn start_with_extra_env(extra_env: &[(&str, &str)]) -> Result<Self> {
        // Locate workspace root — we assume this crate lives at <workspace>/e2e/bridge-e2e
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

        if !cp_binary.exists() {
            return Err(anyhow!(
                "mock-control-plane binary not found at {}",
                cp_binary.display()
            ));
        }
        if !bridge_binary.exists() {
            return Err(anyhow!(
                "bridge binary not found at {}",
                bridge_binary.display()
            ));
        }

        let fixtures_dir = workspace_root.join("fixtures").join("agents");

        // 2. Start mock control plane with port 0 (random)
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

        // Read PORT= from stdout
        let cp_stdout = cp_process
            .stdout
            .take()
            .ok_or_else(|| anyhow!("cannot capture mock-control-plane stdout"))?;

        let (mock_cp_port, cp_drain) = Self::read_port_from_stdout(cp_stdout)?;
        let cp_base_url = format!("http://127.0.0.1:{}", mock_cp_port);

        tracing::info!(port = mock_cp_port, "mock control plane started");

        // Ensure /tmp/workspace exists for the MCP filesystem server fixture
        let _ = std::fs::create_dir_all("/tmp/workspace");

        // 3. Start bridge with env vars pointing to mock control plane, random listen port
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

        for (k, v) in extra_env {
            bridge_command.env(k, v);
        }

        let bridge_process = bridge_command.spawn().context("failed to start bridge")?;

        tracing::info!(port = bridge_port, "bridge process started");

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
            tool_log_dir: None,
            _cp_stdout_drain: Some(cp_drain),
            log_dir,
            conversation_agents: Mutex::new(HashMap::new()),
        };

        // 4. Poll /health until 200.
        let health_timeout = if std::env::var("BRIDGE_STORAGE_PATH").is_ok() {
            Duration::from_secs(60)
        } else {
            Duration::from_secs(30)
        };
        harness
            .wait_for_bridge_healthy_with_timeout(health_timeout)
            .await?;

        // 5. Fetch agents from mock CP and push them to the bridge
        harness.push_agents_from_cp().await?;

        Ok(harness)
    }
}
