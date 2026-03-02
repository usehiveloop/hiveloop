use anyhow::{anyhow, Context, Result};
use serde::{Deserialize, Serialize};
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::time::{Duration, Instant};

/// A single turn in a multi-turn conversation.
#[derive(Debug)]
pub struct ConversationTurn {
    pub conversation_id: String,
    pub response_text: String,
    pub sse_events: Vec<SseEvent>,
    pub duration: Duration,
}

/// Parsed SSE event from the bridge stream.
#[derive(Debug, Clone)]
pub struct SseEvent {
    pub event_type: String,
    pub data: serde_json::Value,
}

/// A tool call log entry from the mock Portal MCP server.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCallLogEntry {
    pub timestamp: String,
    pub tool_name: String,
    pub arguments: serde_json::Value,
    pub result: serde_json::Value,
}

/// End-to-end test harness that manages the mock control plane and bridge
/// processes. Each test should create its own harness to ensure isolation.
pub struct TestHarness {
    /// Port the mock control plane is listening on.
    pub mock_cp_port: u16,
    /// Port the bridge is listening on.
    pub bridge_port: u16,
    /// The mock control plane child process.
    mock_cp_process: Option<Child>,
    /// The bridge child process.
    bridge_process: Option<Child>,
    /// HTTP client for making requests.
    client: reqwest::Client,
    /// Full base URL for the bridge (e.g. "http://127.0.0.1:12345").
    bridge_base_url: String,
    /// Full base URL for the mock control plane.
    cp_base_url: String,
    /// Workspace root path.
    workspace_root: PathBuf,
    /// Tool call log directory (for real agent tests).
    tool_log_dir: Option<PathBuf>,
    /// Keeps the mock-control-plane stdout pipe alive so the process doesn't
    /// get EPIPE (broken pipe) when it writes after we've read the PORT= line.
    _cp_stdout_drain: Option<std::thread::JoinHandle<()>>,
}

impl TestHarness {
    /// Build and start the mock control plane and bridge processes.
    ///
    /// 1. Builds both binaries via `cargo build`.
    /// 2. Starts the mock control plane on a random port and reads PORT= from stdout.
    /// 3. Starts the bridge pointing at the mock control plane on a random port.
    /// 4. Polls the bridge /health endpoint until it responds 200 (max 30s).
    pub async fn start() -> Result<Self> {
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

        // 3. Start bridge with env vars pointing to mock control plane, random listen port
        let bridge_port = Self::find_free_port()?;
        let bridge_listen_addr = format!("127.0.0.1:{}", bridge_port);
        let bridge_base_url = format!("http://127.0.0.1:{}", bridge_port);

        let bridge_process = Command::new(&bridge_binary)
            .env("BRIDGE_CONTROL_PLANE_URL", &cp_base_url)
            .env("BRIDGE_CONTROL_PLANE_API_KEY", "e2e-test-key")
            .env("BRIDGE_LISTEN_ADDR", &bridge_listen_addr)
            .env("BRIDGE_SYNC_INTERVAL_SECS", "5")
            .env("BRIDGE_LOG_LEVEL", "debug")
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()
            .context("failed to start bridge")?;

        tracing::info!(port = bridge_port, "bridge process started");

        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(10))
            .build()
            .context("failed to build reqwest client")?;

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
        };

        // 4. Poll /health until 200 (max 30s)
        harness.wait_for_bridge_healthy().await?;

        Ok(harness)
    }

    /// Start with real agents and OpenRouter. Requires OPENROUTER_API_KEY env.
    /// Builds: bridge, mock-control-plane, mock-portal-mcp.
    /// Loads real agent fixtures from e2e/fixtures/real-agents/.
    pub async fn start_real() -> Result<Self> {
        let openrouter_key = std::env::var("OPENROUTER_API_KEY")
            .context("OPENROUTER_API_KEY environment variable not set")?;

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

        let fixtures_dir = workspace_root.join("e2e").join("fixtures").join("real-agents");
        let tool_log_dir = std::env::temp_dir().join("portal-mcp-logs");
        let _ = std::fs::create_dir_all(&tool_log_dir);

        // 2. Start mock control plane with real agent fixtures and OpenRouter
        let mut cp_process = Command::new(&cp_binary)
            .args([
                "--port",
                "0",
                "--fixtures-dir",
                fixtures_dir.to_str().unwrap(),
                "--openrouter-key",
                &openrouter_key,
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

        tracing::info!(port = mock_cp_port, "mock control plane started (real agents)");

        // 3. Start bridge
        let bridge_port = Self::find_free_port()?;
        let bridge_listen_addr = format!("127.0.0.1:{}", bridge_port);
        let bridge_base_url = format!("http://127.0.0.1:{}", bridge_port);

        // Redirect bridge stdout+stderr to files instead of piping.
        // CRITICAL: if stdout is piped but never read, the pipe buffer fills up
        // (~64KB on macOS) and blocks the bridge process when it writes logs,
        // which deadlocks the async runtime.
        let bridge_stdout_log = std::fs::File::create(
            std::env::temp_dir().join("bridge-e2e-stdout.log"),
        )
        .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());
        let bridge_stderr_log = std::fs::File::create(
            std::env::temp_dir().join("bridge-e2e-stderr.log"),
        )
        .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());

        let bridge_process = Command::new(&bridge_binary)
            .env("BRIDGE_CONTROL_PLANE_URL", &cp_base_url)
            .env("BRIDGE_CONTROL_PLANE_API_KEY", "e2e-test-key")
            .env("BRIDGE_LISTEN_ADDR", &bridge_listen_addr)
            .env("BRIDGE_SYNC_INTERVAL_SECS", "300") // avoid sync during tests
            .env("BRIDGE_LOG_LEVEL", "info")
            .stdout(Stdio::from(bridge_stdout_log))
            .stderr(Stdio::from(bridge_stderr_log))
            .spawn()
            .context("failed to start bridge")?;

        tracing::info!(port = bridge_port, "bridge process started (real agents)");

        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .context("failed to build reqwest client")?;

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
        };

        // 4. Poll /health until 200 (max 60s for real agents — MCP connections take longer)
        harness
            .wait_for_bridge_healthy_with_timeout(Duration::from_secs(60))
            .await?;

        // 5. Wait for agents to be synced and MCP connections established
        harness.wait_for_agents_loaded(5).await?;

        Ok(harness)
    }

    /// Single-turn conversation helper.
    /// Creates a new conversation, sends one message, collects the full response.
    /// For multi-turn, use `converse_multi_turn`.
    pub async fn converse(
        &self,
        agent_id: &str,
        _conv_id: Option<&str>,
        message: &str,
        timeout: Duration,
    ) -> Result<ConversationTurn> {
        let start = Instant::now();

        // Always create a new conversation for simplicity
        // (the bridge SSE receiver is consumed once, so multi-turn on the same
        //  conversation requires keeping the stream open)
        let resp = self.create_conversation(agent_id).await?;
        let status = resp.status();
        let body: serde_json::Value = resp
            .json()
            .await
            .context("failed to parse create conversation response")?;

        if !status.is_success() {
            return Err(anyhow!(
                "failed to create conversation: status={}, body={}",
                status,
                body
            ));
        }

        let conversation_id = body
            .get("conversation_id")
            .and_then(|v| v.as_str())
            .ok_or_else(|| anyhow!("no conversation_id in response: {}", body))?
            .to_string();

        // Send message
        let msg_resp = self.send_message(&conversation_id, message).await?;
        if !msg_resp.status().is_success() && msg_resp.status().as_u16() != 202 {
            let status = msg_resp.status();
            let body = msg_resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to send message: status={}, body={}",
                status,
                body
            ));
        }

        // Connect to SSE stream and collect events
        let (events, response_text) = self
            .stream_sse_until_done(&conversation_id, timeout)
            .await?;

        Ok(ConversationTurn {
            conversation_id,
            response_text,
            sse_events: events,
            duration: start.elapsed(),
        })
    }

    /// Connect to SSE stream and collect events until Done or timeout.
    async fn stream_sse_until_done(
        &self,
        conv_id: &str,
        timeout: Duration,
    ) -> Result<(Vec<SseEvent>, String)> {
        use futures::StreamExt;

        let stream_client = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .build()
            .context("failed to build stream client")?;

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("GET stream request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "stream endpoint returned {}: {}",
                status,
                body
            ));
        }

        let mut events = Vec::new();
        let mut response_text = String::new();
        let mut current_event_type = String::new();

        let deadline = Instant::now() + timeout;

        let mut stream = resp.bytes_stream();

        let mut buffer = String::new();

        loop {
            let remaining = deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                eprintln!("[harness] SSE stream timed out after {:?}", timeout);
                break;
            }

            match tokio::time::timeout(remaining, stream.next()).await {
                Ok(Some(Ok(chunk))) => {
                    buffer.push_str(&String::from_utf8_lossy(&chunk));
                }
                Ok(Some(Err(e))) => {
                    eprintln!("[harness] SSE stream chunk error: {}", e);
                    break;
                }
                Ok(None) => {
                    // Stream ended
                    break;
                }
                Err(_) => {
                    eprintln!("[harness] SSE stream timed out after {:?}", timeout);
                    break;
                }
            }

            // Process complete lines
            while let Some(newline_pos) = buffer.find('\n') {
                let line = buffer[..newline_pos].trim_end().to_string();
                buffer = buffer[newline_pos + 1..].to_string();

                if line.is_empty() {
                    continue;
                }

                if let Some(event_name) = line.strip_prefix("event:") {
                    current_event_type = event_name.trim().to_string();
                } else if let Some(data_str) = line.strip_prefix("data:") {
                    let data_str = data_str.trim();
                    if data_str.is_empty() {
                        continue;
                    }

                    let data: serde_json::Value =
                        serde_json::from_str(data_str).unwrap_or_else(|_| {
                            serde_json::Value::String(data_str.to_string())
                        });

                    // Determine event type from event: line or from data.type
                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

                    // Collect content deltas into response text
                    if event_type == "content_delta" {
                        if let Some(delta) = data.get("delta").and_then(|d| d.as_str()) {
                            response_text.push_str(delta);
                        }
                    }

                    let event = SseEvent {
                        event_type: event_type.clone(),
                        data,
                    };
                    events.push(event);

                    // Stop when we get a Done event
                    if event_type == "done" {
                        return Ok((events, response_text));
                    }

                    current_event_type.clear();
                }
            }
        }

        // If we have events but no response text, try to extract from error events
        if response_text.is_empty() && !events.is_empty() {
            eprintln!(
                "[harness] Warning: no content_delta events found. Events received: {:?}",
                events
                    .iter()
                    .map(|e| format!("{}:{}", e.event_type, &e.data.to_string()[..e.data.to_string().len().min(100)]))
                    .collect::<Vec<_>>()
            );
        }

        Ok((events, response_text))
    }

    /// Read the mock Portal MCP tool call log files.
    pub fn read_tool_call_log(&self) -> Result<Vec<ToolCallLogEntry>> {
        let log_dir = self
            .tool_log_dir
            .as_ref()
            .ok_or_else(|| anyhow!("no tool log directory configured (not using real agents?)"))?;

        let mut entries = Vec::new();

        if let Ok(dir_entries) = std::fs::read_dir(log_dir) {
            for entry in dir_entries.flatten() {
                let path = entry.path();
                if path.extension().is_some_and(|e| e == "jsonl") {
                    if let Ok(content) = std::fs::read_to_string(&path) {
                        for line in content.lines() {
                            let line = line.trim();
                            if line.is_empty() {
                                continue;
                            }
                            if let Ok(entry) = serde_json::from_str::<ToolCallLogEntry>(line) {
                                entries.push(entry);
                            }
                        }
                    }
                }
            }
        }

        Ok(entries)
    }

    /// Assert that a specific tool was called at least once.
    pub fn assert_tool_called(&self, tool_name: &str) -> Result<()> {
        let entries = self.read_tool_call_log()?;
        if entries.iter().any(|e| e.tool_name == tool_name) {
            Ok(())
        } else {
            let called_tools: Vec<&str> = entries.iter().map(|e| e.tool_name.as_str()).collect();
            Err(anyhow!(
                "tool '{}' was never called. Tools called: {:?}",
                tool_name,
                called_tools
            ))
        }
    }

    /// Assert that at least one of the given tools was called.
    pub fn assert_any_tool_called(&self, tool_names: &[&str]) -> Result<()> {
        let entries = self.read_tool_call_log()?;
        if entries
            .iter()
            .any(|e| tool_names.contains(&e.tool_name.as_str()))
        {
            Ok(())
        } else {
            let called_tools: Vec<&str> = entries.iter().map(|e| e.tool_name.as_str()).collect();
            Err(anyhow!(
                "none of {:?} were called. Tools called: {:?}",
                tool_names,
                called_tools
            ))
        }
    }

    /// Assert tool called with args matching a predicate.
    pub fn assert_tool_called_with(
        &self,
        tool_name: &str,
        predicate: impl Fn(&serde_json::Value) -> bool,
    ) -> Result<()> {
        let entries = self.read_tool_call_log()?;
        let matching: Vec<_> = entries
            .iter()
            .filter(|e| e.tool_name == tool_name)
            .collect();

        if matching.is_empty() {
            let called_tools: Vec<&str> = entries.iter().map(|e| e.tool_name.as_str()).collect();
            return Err(anyhow!(
                "tool '{}' was never called. Tools called: {:?}",
                tool_name,
                called_tools
            ));
        }

        if matching.iter().any(|e| predicate(&e.arguments)) {
            Ok(())
        } else {
            Err(anyhow!(
                "tool '{}' was called {} times but no call matched the predicate",
                tool_name,
                matching.len()
            ))
        }
    }

    /// Read the PORT={port} line from the mock control plane stdout.
    ///
    /// Returns the port and a background thread that drains remaining stdout.
    /// The drain thread keeps the pipe alive so the child process doesn't get
    /// EPIPE when it writes after PORT=.
    fn read_port_from_stdout(
        stdout: std::process::ChildStdout,
    ) -> Result<(u16, std::thread::JoinHandle<()>)> {
        let mut reader = BufReader::new(stdout);
        let start = Instant::now();
        let timeout = Duration::from_secs(30);

        let mut line_buf = String::new();
        loop {
            if start.elapsed() > timeout {
                return Err(anyhow!(
                    "timed out waiting for PORT= from mock-control-plane"
                ));
            }

            line_buf.clear();
            let bytes_read = reader
                .read_line(&mut line_buf)
                .context("failed to read stdout line")?;
            if bytes_read == 0 {
                return Err(anyhow!(
                    "mock-control-plane exited without printing PORT="
                ));
            }

            let line = line_buf.trim();
            if let Some(port_str) = line.strip_prefix("PORT=") {
                let port: u16 = port_str
                    .trim()
                    .parse()
                    .context("failed to parse port number")?;

                // Drain remaining stdout in background to prevent EPIPE
                let drain_handle = std::thread::spawn(move || {
                    let mut sink = Vec::new();
                    let _ = std::io::Read::read_to_end(&mut reader, &mut sink);
                });

                return Ok((port, drain_handle));
            }
        }
    }

    /// Find a free TCP port by binding to port 0 and reading the assigned port.
    fn find_free_port() -> Result<u16> {
        let listener = std::net::TcpListener::bind("127.0.0.1:0")
            .context("failed to bind to find free port")?;
        let port = listener.local_addr()?.port();
        drop(listener);
        Ok(port)
    }

    /// Poll the bridge /health endpoint until it returns 200 or timeout.
    async fn wait_for_bridge_healthy(&mut self) -> Result<()> {
        self.wait_for_bridge_healthy_with_timeout(Duration::from_secs(30))
            .await
    }

    async fn wait_for_bridge_healthy_with_timeout(&mut self, timeout: Duration) -> Result<()> {
        let start = Instant::now();
        let poll_interval = Duration::from_millis(100);

        loop {
            if start.elapsed() > timeout {
                // Check if bridge process is still alive
                if let Some(ref mut proc) = self.bridge_process {
                    match proc.try_wait() {
                        Ok(Some(status)) => {
                            return Err(anyhow!(
                                "bridge process exited with status {} before becoming healthy",
                                status
                            ));
                        }
                        Ok(None) => {} // still running
                        Err(e) => {
                            return Err(anyhow!("failed to check bridge process status: {}", e));
                        }
                    }
                }
                return Err(anyhow!(
                    "timed out waiting for bridge /health ({:.0}s elapsed)",
                    timeout.as_secs_f64()
                ));
            }

            match self
                .client
                .get(format!("{}/health", self.bridge_base_url))
                .send()
                .await
            {
                Ok(resp) if resp.status().is_success() => {
                    tracing::info!(
                        elapsed_ms = start.elapsed().as_millis() as u64,
                        "bridge is healthy"
                    );
                    return Ok(());
                }
                _ => {
                    tokio::time::sleep(poll_interval).await;
                }
            }
        }
    }

    /// Wait until at least `min_count` agents are loaded in the bridge.
    async fn wait_for_agents_loaded(&self, min_count: usize) -> Result<()> {
        let start = Instant::now();
        let timeout = Duration::from_secs(60);
        let poll_interval = Duration::from_secs(2);

        loop {
            if start.elapsed() > timeout {
                return Err(anyhow!(
                    "timed out waiting for {} agents to load",
                    min_count
                ));
            }

            match self.get_agents().await {
                Ok(agents) if agents.len() >= min_count => {
                    tracing::info!(
                        count = agents.len(),
                        elapsed_ms = start.elapsed().as_millis() as u64,
                        "agents loaded"
                    );
                    return Ok(());
                }
                Ok(agents) => {
                    tracing::debug!(
                        count = agents.len(),
                        target = min_count,
                        "waiting for agents to load..."
                    );
                }
                Err(_) => {}
            }

            tokio::time::sleep(poll_interval).await;
        }
    }

    // ---- Accessors ----

    /// Returns the bridge base URL (e.g. "http://127.0.0.1:12345").
    pub fn bridge_url(&self) -> &str {
        &self.bridge_base_url
    }

    /// Returns the mock control plane base URL.
    pub fn cp_url(&self) -> &str {
        &self.cp_base_url
    }

    /// Returns the workspace root path.
    pub fn workspace_root(&self) -> &std::path::Path {
        &self.workspace_root
    }

    // ---- Bridge API helpers ----

    /// GET /health
    pub async fn health(&self) -> Result<serde_json::Value> {
        let resp = self
            .client
            .get(format!("{}/health", self.bridge_base_url))
            .send()
            .await
            .context("GET /health request failed")?;

        let body = resp.json().await.context("failed to parse /health body")?;
        Ok(body)
    }

    /// GET /agents — returns list of agents.
    pub async fn get_agents(&self) -> Result<Vec<serde_json::Value>> {
        let resp = self
            .client
            .get(format!("{}/agents", self.bridge_base_url))
            .send()
            .await
            .context("GET /agents request failed")?;

        let body = resp.json().await.context("failed to parse /agents body")?;
        Ok(body)
    }

    /// GET /agents/{id} — returns agent details or error.
    pub async fn get_agent(&self, id: &str) -> Result<reqwest::Response> {
        let resp = self
            .client
            .get(format!("{}/agents/{}", self.bridge_base_url, id))
            .send()
            .await
            .context("GET /agents/{id} request failed")?;

        Ok(resp)
    }

    /// POST /agents/{agent_id}/conversations — create a new conversation.
    pub async fn create_conversation(&self, agent_id: &str) -> Result<reqwest::Response> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations",
                self.bridge_base_url, agent_id
            ))
            .send()
            .await
            .context("POST create conversation request failed")?;

        Ok(resp)
    }

    /// POST /conversations/{conv_id}/messages — send a message.
    pub async fn send_message(&self, conv_id: &str, content: &str) -> Result<reqwest::Response> {
        let resp = self
            .client
            .post(format!(
                "{}/conversations/{}/messages",
                self.bridge_base_url, conv_id
            ))
            .json(&serde_json::json!({"content": content}))
            .send()
            .await
            .context("POST send message request failed")?;

        Ok(resp)
    }

    /// DELETE /conversations/{conv_id} — end a conversation.
    pub async fn end_conversation(&self, conv_id: &str) -> Result<reqwest::Response> {
        let resp = self
            .client
            .delete(format!(
                "{}/conversations/{}",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("DELETE end conversation request failed")?;

        Ok(resp)
    }

    /// GET /metrics
    pub async fn get_metrics(&self) -> Result<serde_json::Value> {
        let resp = self
            .client
            .get(format!("{}/metrics", self.bridge_base_url))
            .send()
            .await
            .context("GET /metrics request failed")?;

        let body = resp.json().await.context("failed to parse /metrics body")?;
        Ok(body)
    }

    /// Connect to the SSE stream for a conversation and collect events
    /// until the stream ends or a timeout is reached.
    pub async fn stream_events(&self, conv_id: &str, timeout: Duration) -> Result<Vec<String>> {
        let stream_client = reqwest::Client::builder()
            .timeout(timeout)
            .build()
            .context("failed to build stream client")?;

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("GET stream request failed")?;

        let mut events = Vec::new();
        let body = resp.text().await.context("failed to read stream body")?;

        // Parse SSE format: lines starting with "data:" contain event data
        for line in body.lines() {
            if let Some(data) = line.strip_prefix("data:") {
                let data = data.trim();
                if !data.is_empty() {
                    events.push(data.to_string());
                }
            }
        }

        Ok(events)
    }

    // ---- Mock Control Plane helpers ----

    /// POST /agents on the mock control plane — add a new agent definition.
    pub async fn add_agent_to_cp(&self, def: &bridge_core::AgentDefinition) -> Result<()> {
        let resp = self
            .client
            .post(format!("{}/agents", self.cp_base_url))
            .json(def)
            .send()
            .await
            .context("POST agent to CP failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to add agent to CP: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// PUT /agents/{id} on the mock control plane — update an agent definition.
    pub async fn update_agent_in_cp(
        &self,
        id: &str,
        def: &bridge_core::AgentDefinition,
    ) -> Result<()> {
        let resp = self
            .client
            .put(format!("{}/agents/{}", self.cp_base_url, id))
            .json(def)
            .send()
            .await
            .context("PUT agent in CP failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to update agent in CP: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// DELETE /agents/{id} on the mock control plane — remove an agent definition.
    pub async fn delete_agent_from_cp(&self, id: &str) -> Result<()> {
        let resp = self
            .client
            .delete(format!("{}/agents/{}", self.cp_base_url, id))
            .send()
            .await
            .context("DELETE agent from CP failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to delete agent from CP: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// GET /webhooks/log on the mock control plane — retrieve received webhooks.
    pub async fn get_webhook_log(&self) -> Result<Vec<serde_json::Value>> {
        let resp = self
            .client
            .get(format!("{}/webhooks/log", self.cp_base_url))
            .send()
            .await
            .context("GET webhook log failed")?;

        let body = resp
            .json()
            .await
            .context("failed to parse webhook log body")?;
        Ok(body)
    }

    /// DELETE /webhooks/log on the mock control plane — clear the webhook log.
    pub async fn clear_webhook_log(&self) -> Result<()> {
        let resp = self
            .client
            .delete(format!("{}/webhooks/log", self.cp_base_url))
            .send()
            .await
            .context("DELETE webhook log failed")?;

        if !resp.status().is_success() {
            return Err(anyhow!("failed to clear webhook log"));
        }

        Ok(())
    }

    /// Stop the harness — kill both processes and wait for them to exit.
    pub fn stop(&mut self) {
        if let Some(ref mut proc) = self.bridge_process {
            let _ = proc.kill();
            let _ = proc.wait();
        }
        self.bridge_process = None;

        if let Some(ref mut proc) = self.mock_cp_process {
            let _ = proc.kill();
            let _ = proc.wait();
        }
        self.mock_cp_process = None;
    }
}

impl Drop for TestHarness {
    fn drop(&mut self) {
        self.stop();
    }
}
