use anyhow::{anyhow, Context, Result};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};
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

/// A received webhook entry from the mock control plane.
///
/// Provides typed access to the webhook payload fields so tests can assert
/// on event types, agent/conversation IDs, data, and HMAC headers without
/// manually navigating raw JSON.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookEntry {
    /// Server-assigned timestamp when the webhook was received.
    pub timestamp: String,
    /// HTTP headers from the webhook request.
    pub headers: HashMap<String, String>,
    /// Parsed JSON body of the webhook payload.
    pub body: serde_json::Value,
}

impl WebhookEntry {
    /// The webhook event type (e.g. "conversation_created", "response_started").
    pub fn event_type(&self) -> Option<&str> {
        self.body.get("event_type").and_then(|v| v.as_str())
    }

    /// The agent ID from the webhook payload.
    pub fn agent_id(&self) -> Option<&str> {
        self.body.get("agent_id").and_then(|v| v.as_str())
    }

    /// The conversation ID from the webhook payload.
    pub fn conversation_id(&self) -> Option<&str> {
        self.body.get("conversation_id").and_then(|v| v.as_str())
    }

    /// The event-specific data payload.
    pub fn data(&self) -> Option<&serde_json::Value> {
        self.body.get("data")
    }

    /// Whether the `X-Webhook-Signature` header is present.
    pub fn has_signature(&self) -> bool {
        self.headers.contains_key("x-webhook-signature")
    }

    /// Whether the `X-Webhook-Timestamp` header is present.
    pub fn has_timestamp_header(&self) -> bool {
        self.headers.contains_key("x-webhook-timestamp")
    }
}

/// A collected set of webhook entries with query/assertion helpers.
#[derive(Debug, Clone)]
pub struct WebhookLog {
    pub entries: Vec<WebhookEntry>,
}

impl WebhookLog {
    /// All distinct event types present in the log.
    pub fn event_types(&self) -> Vec<String> {
        self.entries
            .iter()
            .filter_map(|e| e.event_type().map(|s| s.to_string()))
            .collect()
    }

    /// All distinct event types, deduplicated.
    pub fn unique_event_types(&self) -> Vec<String> {
        let mut types = self.event_types();
        types.sort();
        types.dedup();
        types
    }

    /// Filter entries by event type.
    pub fn by_type(&self, event_type: &str) -> Vec<&WebhookEntry> {
        self.entries
            .iter()
            .filter(|e| e.event_type() == Some(event_type))
            .collect()
    }

    /// Whether any entry has the given event type.
    pub fn has_type(&self, event_type: &str) -> bool {
        self.entries
            .iter()
            .any(|e| e.event_type() == Some(event_type))
    }

    /// Filter entries by conversation ID.
    pub fn by_conversation(&self, conv_id: &str) -> Vec<&WebhookEntry> {
        self.entries
            .iter()
            .filter(|e| e.conversation_id() == Some(conv_id))
            .collect()
    }

    /// Assert that a given event type is present, with a descriptive panic message.
    pub fn assert_has_type(&self, event_type: &str) {
        assert!(
            self.has_type(event_type),
            "expected webhook event type '{}' not found in log; got: {:?}",
            event_type,
            self.unique_event_types()
        );
    }

    /// Assert that every entry has a valid `agent_id` field.
    pub fn assert_all_have_agent_id(&self) {
        for entry in &self.entries {
            assert!(
                entry.agent_id().is_some(),
                "webhook body should have agent_id: {:?}",
                entry.body
            );
        }
    }

    /// Assert that every entry has a valid `conversation_id` field.
    pub fn assert_all_have_conversation_id(&self) {
        for entry in &self.entries {
            assert!(
                entry.conversation_id().is_some(),
                "webhook body should have conversation_id: {:?}",
                entry.body
            );
        }
    }

    /// Assert that at least one entry has the `X-Webhook-Signature` header.
    pub fn assert_has_signature_header(&self) {
        assert!(
            self.entries.iter().any(|e| e.has_signature()),
            "at least one webhook should have x-webhook-signature header"
        );
    }

    /// Assert that at least one entry has the `X-Webhook-Timestamp` header.
    pub fn assert_has_timestamp_header(&self) {
        assert!(
            self.entries.iter().any(|e| e.has_timestamp_header()),
            "at least one webhook should have x-webhook-timestamp header"
        );
    }

    /// Number of entries in the log.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Whether the log is empty.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
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
    /// Directory for conversation log files (one per agent).
    log_dir: PathBuf,
    /// Maps conversation_id → agent_id for log file routing.
    conversation_agents: Mutex<HashMap<String, String>>,
}

/// Returns a UTC timestamp string for log entries.
fn now_str() -> String {
    let d = std::time::SystemTime::now()
        .duration_since(std::time::SystemTime::UNIX_EPOCH)
        .unwrap_or_default();
    let total_secs = d.as_secs();
    let hours = (total_secs / 3600) % 24;
    let mins = (total_secs / 60) % 60;
    let secs = total_secs % 60;
    let millis = d.subsec_millis();
    format!("{:02}:{:02}:{:02}.{:03}", hours, mins, secs, millis)
}

/// Format an SSE event for human-readable logging.
fn format_sse_for_log(event_type: &str, data: &serde_json::Value) -> String {
    match event_type {
        "tool_call_start" => {
            let name = data.get("name").and_then(|v| v.as_str()).unwrap_or("?");
            let id = data.get("id").and_then(|v| v.as_str()).unwrap_or("?");
            let args = data
                .get("arguments")
                .map(|a| serde_json::to_string_pretty(a).unwrap_or_else(|_| a.to_string()))
                .unwrap_or_default();
            format!("Tool: {} (id: {})\nArguments:\n{}", name, id, args)
        }
        "tool_call_result" => {
            let id = data.get("id").and_then(|v| v.as_str()).unwrap_or("?");
            let is_error = data
                .get("is_error")
                .and_then(|v| v.as_bool())
                .unwrap_or(false);
            let result_str = data.get("result").and_then(|v| v.as_str()).unwrap_or("");
            let formatted = serde_json::from_str::<serde_json::Value>(result_str)
                .map(|v| {
                    serde_json::to_string_pretty(&v).unwrap_or_else(|_| result_str.to_string())
                })
                .unwrap_or_else(|_| result_str.to_string());
            // Truncate very long results for readability
            let truncated = if formatted.len() > 4000 {
                format!(
                    "{}...\n[truncated, {} total chars]",
                    &formatted[..4000],
                    formatted.len()
                )
            } else {
                formatted
            };
            format!("id: {}, is_error: {}\nResult:\n{}", id, is_error, truncated)
        }
        "content_delta" => {
            let delta = data.get("delta").and_then(|v| v.as_str()).unwrap_or("");
            format!("\"{}\"", delta)
        }
        _ => serde_json::to_string_pretty(data).unwrap_or_else(|_| data.to_string()),
    }
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
            .env("BRIDGE_LOG_LEVEL", "debug")
            .env(
                "BRIDGE_WEBHOOK_URL",
                format!("{}/webhooks/receive", cp_base_url),
            )
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()
            .context("failed to start bridge")?;

        tracing::info!(port = bridge_port, "bridge process started");

        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(10))
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

        // 4. Poll /health until 200 (max 30s)
        harness.wait_for_bridge_healthy().await?;

        // 5. Fetch agents from mock CP and push them to the bridge
        harness.push_agents_from_cp().await?;

        Ok(harness)
    }

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
        let bridge_stdout_log =
            std::fs::File::create(std::env::temp_dir().join(format!("bridge-e2e-stdout-{}.log", bridge_port)))
                .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());
        let bridge_stderr_log =
            std::fs::File::create(std::env::temp_dir().join(format!("bridge-e2e-stderr-{}.log", bridge_port)))
                .unwrap_or_else(|_| std::fs::File::create("/dev/null").unwrap());

        eprintln!(
            "[harness] Bridge logs: stdout={}/bridge-e2e-stdout-{}.log stderr={}/bridge-e2e-stderr-{}.log",
            std::env::temp_dir().display(), bridge_port,
            std::env::temp_dir().display(), bridge_port,
        );

        let bridge_process = Command::new(&bridge_binary)
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
            .stderr(Stdio::from(bridge_stderr_log))
            .spawn()
            .context("failed to start bridge")?;

        tracing::info!(port = bridge_port, "bridge process started (real agents)");

        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
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

        // Register conversation and log header + system prompt
        self.register_conversation(&conversation_id, agent_id).await;

        // Send message (logging happens inside send_message)
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

        // Connect to SSE stream and collect events (logging happens inside)
        let (events, response_text) = self
            .stream_sse_until_done(&conversation_id, timeout)
            .await?;

        // Log the assembled assistant response
        let label = self.log_label(&conversation_id);
        self.append_log(
            &label,
            &format!(
                "\n================================================================================\n\
                 ASSISTANT RESPONSE (complete)\n\
                 ================================================================================\n\
                 {}\n\n\
                 ================================================================================\n\
                 TURN COMPLETED ({:.1}s)\n\
                 ================================================================================\n\n",
                if response_text.is_empty() {
                    "[empty response]"
                } else {
                    &response_text
                },
                start.elapsed().as_secs_f64()
            ),
        );

        Ok(ConversationTurn {
            conversation_id,
            response_text,
            sse_events: events,
            duration: start.elapsed(),
        })
    }

    /// Connect to SSE stream and collect events until Done or timeout.
    pub async fn stream_sse_until_done(
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
            return Err(anyhow!("stream endpoint returned {}: {}", status, body));
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

                    let data: serde_json::Value = serde_json::from_str(data_str)
                        .unwrap_or_else(|_| serde_json::Value::String(data_str.to_string()));

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

                    // Log the SSE event
                    let last = events.last().unwrap();
                    self.log_sse_event(conv_id, &last.event_type, &last.data);

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
                    .map(|e| format!(
                        "{}:{}",
                        e.event_type,
                        &e.data.to_string()[..e.data.to_string().len().min(100)]
                    ))
                    .collect::<Vec<_>>()
            );
        }

        Ok((events, response_text))
    }

    /// Connect to SSE stream and collect events across multiple turns.
    /// Keeps reading past "done" events until `done_count` "done" events
    /// have been received, or the timeout expires.
    pub async fn stream_sse_until_done_count(
        &self,
        conv_id: &str,
        done_count: usize,
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
            return Err(anyhow!("stream endpoint returned {}: {}", status, body));
        }

        let mut events = Vec::new();
        let mut response_text = String::new();
        let mut current_event_type = String::new();
        let mut done_seen = 0usize;

        let deadline = Instant::now() + timeout;

        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();

        loop {
            let remaining = deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                eprintln!(
                    "[harness] SSE stream timed out after {:?} (done_seen={}/{})",
                    timeout, done_seen, done_count
                );
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
                    eprintln!(
                        "[harness] SSE stream timed out after {:?} (done_seen={}/{})",
                        timeout, done_seen, done_count
                    );
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

                    let data: serde_json::Value = serde_json::from_str(data_str)
                        .unwrap_or_else(|_| serde_json::Value::String(data_str.to_string()));

                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

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

                    // Log the SSE event
                    let last = events.last().unwrap();
                    self.log_sse_event(conv_id, &last.event_type, &last.data);

                    if event_type == "done" {
                        done_seen += 1;
                        if done_seen >= done_count {
                            return Ok((events, response_text));
                        }
                    }

                    current_event_type.clear();
                }
            }
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

    // ---- Conversation logging ----

    /// Returns the log directory path.
    pub fn log_dir(&self) -> &Path {
        &self.log_dir
    }

    /// Register a conversation_id → agent_id mapping and log the conversation
    /// header (agent info, system prompt, tools).
    pub async fn register_conversation(&self, conv_id: &str, agent_id: &str) {
        self.conversation_agents
            .lock()
            .unwrap()
            .insert(conv_id.to_string(), agent_id.to_string());

        let mut log = format!(
            "================================================================================\n\
             CONVERSATION STARTED\n\
             ================================================================================\n\
             Timestamp:       {}\n\
             Agent ID:        {}\n\
             Conversation ID: {}\n\n",
            now_str(),
            agent_id,
            conv_id
        );

        // Fetch agent info to log system prompt and tools
        if let Ok(resp) = self.get_agent(agent_id).await {
            if resp.status().is_success() {
                if let Ok(body) = resp.json::<serde_json::Value>().await {
                    if let Some(prompt) = body.get("system_prompt").and_then(|v| v.as_str()) {
                        log.push_str(&format!(
                            "================================================================================\n\
                             SYSTEM PROMPT\n\
                             ================================================================================\n\
                             {}\n\n",
                            prompt
                        ));
                    }
                    if let Some(tools) = body.get("tools").and_then(|v| v.as_array()) {
                        let tool_names: Vec<&str> = tools
                            .iter()
                            .filter_map(|t| t.get("name").and_then(|n| n.as_str()))
                            .collect();
                        if !tool_names.is_empty() {
                            log.push_str(&format!("Built-in Tools: {}\n\n", tool_names.join(", ")));
                        }
                    }
                    if let Some(mcp) = body.get("mcp_servers").and_then(|v| v.as_array()) {
                        let server_names: Vec<&str> = mcp
                            .iter()
                            .filter_map(|s| s.get("name").and_then(|n| n.as_str()))
                            .collect();
                        if !server_names.is_empty() {
                            log.push_str(&format!("MCP Servers: {}\n\n", server_names.join(", ")));
                        }
                    }
                    if let Some(subagents) = body.get("subagents").and_then(|v| v.as_array()) {
                        if !subagents.is_empty() {
                            let sub_ids: Vec<&str> = subagents
                                .iter()
                                .filter_map(|s| s.get("id").and_then(|n| n.as_str()))
                                .collect();
                            log.push_str(&format!("Subagents: {}\n\n", sub_ids.join(", ")));
                        }
                    }
                }
            }
        }

        self.append_log(agent_id, &log);
    }

    /// Get the log label (agent_id) for a conversation, or fall back to conv_id.
    fn log_label(&self, conv_id: &str) -> String {
        self.conversation_agents
            .lock()
            .unwrap()
            .get(conv_id)
            .cloned()
            .unwrap_or_else(|| conv_id.to_string())
    }

    /// Stream log content for the given label (agent_id) to stderr so it
    /// appears in real-time during test runs.
    fn append_log(&self, label: &str, content: &str) {
        for line in content.lines() {
            eprintln!("[{}] {}", label, line);
        }
    }

    /// Log a single SSE event to the appropriate log file.
    fn log_sse_event(&self, conv_id: &str, event_type: &str, data: &serde_json::Value) {
        let label = self.log_label(conv_id);
        let formatted = format_sse_for_log(event_type, data);
        self.append_log(
            &label,
            &format!(
                "[{}] --- SSE: {} ---\n{}\n\n",
                now_str(),
                event_type,
                formatted
            ),
        );
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
                return Err(anyhow!("mock-control-plane exited without printing PORT="));
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
        let label = self.log_label(conv_id);
        self.append_log(
            &label,
            &format!(
                "[{}] ================================================================================\n\
                 USER MESSAGE\n\
                 ================================================================================\n\
                 {}\n\n",
                now_str(),
                content
            ),
        );

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
        let label = self.log_label(conv_id);
        self.append_log(
            &label,
            &format!(
                "[{}] ================================================================================\n\
                 CONVERSATION ENDED\n\
                 ================================================================================\n\n",
                now_str()
            ),
        );

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

    /// POST /conversations/{conv_id}/abort — abort the current in-flight turn.
    pub async fn abort_conversation(&self, conv_id: &str) -> Result<reqwest::Response> {
        let label = self.log_label(conv_id);
        self.append_log(
            &label,
            &format!(
                "[{}] ================================================================================\n\
                 CONVERSATION ABORTED\n\
                 ================================================================================\n\n",
                now_str()
            ),
        );

        let resp = self
            .client
            .post(format!(
                "{}/conversations/{}/abort",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("POST abort conversation request failed")?;

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

    // ---- Push helpers (control plane → bridge) ----

    /// Fetch agents from mock CP via GET /agents, then push them to bridge via POST /push/agents.
    pub async fn push_agents_from_cp(&self) -> Result<()> {
        // Fetch agent definitions from mock control plane
        let resp = self
            .client
            .get(format!("{}/agents", self.cp_base_url))
            .send()
            .await
            .context("GET /agents from CP failed")?;

        let agents: Vec<serde_json::Value> = resp
            .json()
            .await
            .context("failed to parse CP /agents response")?;

        tracing::info!(
            count = agents.len(),
            "fetched agents from mock CP, pushing to bridge"
        );

        // Push to bridge
        let push_resp = self
            .client
            .post(format!("{}/push/agents", self.bridge_base_url))
            .header("authorization", "Bearer e2e-test-key")
            .json(&serde_json::json!({"agents": agents}))
            .send()
            .await
            .context("POST /push/agents to bridge failed")?;

        if !push_resp.status().is_success() {
            let status = push_resp.status();
            let body = push_resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to push agents to bridge: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// Push a diff to the bridge via POST /push/diff.
    pub async fn push_diff_to_bridge(
        &self,
        added: &[serde_json::Value],
        updated: &[serde_json::Value],
        removed: &[&str],
    ) -> Result<()> {
        let resp = self
            .client
            .post(format!("{}/push/diff", self.bridge_base_url))
            .header("authorization", "Bearer e2e-test-key")
            .json(&serde_json::json!({
                "added": added,
                "updated": updated,
                "removed": removed,
            }))
            .send()
            .await
            .context("POST /push/diff to bridge failed")?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!(
                "failed to push diff to bridge: status={}, body={}",
                status,
                body
            ));
        }

        Ok(())
    }

    /// Push a single agent to the bridge via PUT /push/agents/{agent_id}.
    pub async fn push_agent_to_bridge(
        &self,
        agent: &serde_json::Value,
    ) -> Result<reqwest::Response> {
        let agent_id = agent
            .get("id")
            .and_then(|v| v.as_str())
            .ok_or_else(|| anyhow!("agent has no id field"))?;

        let resp = self
            .client
            .put(format!("{}/push/agents/{}", self.bridge_base_url, agent_id))
            .header("authorization", "Bearer e2e-test-key")
            .json(agent)
            .send()
            .await
            .context("PUT /push/agents/{id} to bridge failed")?;

        Ok(resp)
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

    /// GET /webhooks/log on the mock control plane — retrieve received webhooks
    /// as raw JSON values (kept for backwards compatibility).
    pub async fn get_webhook_log_raw(&self) -> Result<Vec<serde_json::Value>> {
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

    /// GET /webhooks/log on the mock control plane — retrieve received webhooks
    /// as typed [`WebhookLog`] with query helpers.
    pub async fn get_webhook_log(&self) -> Result<WebhookLog> {
        let raw = self.get_webhook_log_raw().await?;
        let entries: Vec<WebhookEntry> = raw
            .into_iter()
            .map(|v| serde_json::from_value(v).expect("failed to deserialize WebhookEntry"))
            .collect();
        Ok(WebhookLog { entries })
    }

    /// Poll the webhook log until at least `min_count` entries are present, or
    /// until `timeout` elapses. Returns the final [`WebhookLog`].
    ///
    /// Useful because webhook dispatch is fire-and-forget with retries, so
    /// there is a brief delivery delay.
    pub async fn wait_for_webhooks(
        &self,
        min_count: usize,
        timeout: Duration,
    ) -> Result<WebhookLog> {
        let deadline = Instant::now() + timeout;
        loop {
            let log = self.get_webhook_log().await?;
            if log.len() >= min_count {
                return Ok(log);
            }
            if Instant::now() >= deadline {
                return Ok(log);
            }
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
    }

    /// Poll the webhook log until a specific event type is present, or until
    /// `timeout` elapses.
    pub async fn wait_for_webhook_type(
        &self,
        event_type: &str,
        timeout: Duration,
    ) -> Result<WebhookLog> {
        let deadline = Instant::now() + timeout;
        loop {
            let log = self.get_webhook_log().await?;
            if log.has_type(event_type) {
                return Ok(log);
            }
            if Instant::now() >= deadline {
                return Ok(log);
            }
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
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

    // ---- Tool Approval helpers ----

    /// GET /agents/{agent_id}/conversations/{conv_id}/approvals — list pending approvals.
    pub async fn list_approvals(
        &self,
        agent_id: &str,
        conv_id: &str,
    ) -> Result<Vec<serde_json::Value>> {
        let resp = self
            .client
            .get(format!(
                "{}/agents/{}/conversations/{}/approvals",
                self.bridge_base_url, agent_id, conv_id
            ))
            .send()
            .await
            .context("GET approvals request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!("list approvals failed: status={}, body={}", status, body));
        }

        let approvals: Vec<serde_json::Value> = resp.json().await
            .context("failed to parse approvals response")?;
        Ok(approvals)
    }

    /// POST /agents/{agent_id}/conversations/{conv_id}/approvals/{request_id}
    /// — resolve a single approval request.
    pub async fn resolve_approval(
        &self,
        agent_id: &str,
        conv_id: &str,
        request_id: &str,
        decision: &str,
    ) -> Result<reqwest::Response> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations/{}/approvals/{}",
                self.bridge_base_url, agent_id, conv_id, request_id
            ))
            .json(&serde_json::json!({"decision": decision}))
            .send()
            .await
            .context("POST resolve approval request failed")?;

        Ok(resp)
    }

    /// POST /agents/{agent_id}/conversations/{conv_id}/approvals
    /// — bulk resolve multiple approval requests.
    pub async fn bulk_resolve_approvals(
        &self,
        agent_id: &str,
        conv_id: &str,
        request_ids: &[String],
        decision: &str,
    ) -> Result<serde_json::Value> {
        let resp = self
            .client
            .post(format!(
                "{}/agents/{}/conversations/{}/approvals",
                self.bridge_base_url, agent_id, conv_id
            ))
            .json(&serde_json::json!({
                "request_ids": request_ids,
                "decision": decision,
            }))
            .send()
            .await
            .context("POST bulk resolve approvals request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!("bulk resolve failed: status={}, body={}", status, body));
        }

        resp.json().await.context("failed to parse bulk resolve response")
    }

    /// Stream SSE events collecting them until a specific event type is seen,
    /// then return all events collected so far (including the target event).
    ///
    /// Useful for waiting until `tool_approval_required` fires before interacting
    /// with the approval API.
    pub async fn stream_sse_until_event(
        &self,
        conv_id: &str,
        target_event_type: &str,
        timeout: Duration,
    ) -> Result<Vec<SseEvent>> {
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
            return Err(anyhow!("stream endpoint returned {}: {}", status, body));
        }

        let mut events = Vec::new();
        let mut current_event_type = String::new();
        let deadline = Instant::now() + timeout;
        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();

        loop {
            let remaining = deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                eprintln!(
                    "[harness] SSE stream timed out waiting for '{}'",
                    target_event_type
                );
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
                Ok(None) => break,
                Err(_) => {
                    eprintln!(
                        "[harness] SSE stream timed out waiting for '{}'",
                        target_event_type
                    );
                    break;
                }
            }

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

                    let data: serde_json::Value = serde_json::from_str(data_str)
                        .unwrap_or_else(|_| serde_json::Value::String(data_str.to_string()));

                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

                    self.log_sse_event(conv_id, &event_type, &data);

                    let event = SseEvent {
                        event_type: event_type.clone(),
                        data,
                    };
                    events.push(event);

                    if event_type == target_event_type || event_type == "done" {
                        return Ok(events);
                    }

                    current_event_type.clear();
                }
            }
        }

        Ok(events)
    }

    /// Stop the harness — gracefully terminate processes so logs are flushed.
    pub fn stop(&mut self) {
        for (name, proc_opt) in [
            ("bridge", &mut self.bridge_process),
            ("mock-cp", &mut self.mock_cp_process),
        ] {
            if let Some(ref mut proc) = proc_opt {
                // Send SIGTERM first for graceful shutdown (flushes logs)
                #[cfg(unix)]
                {
                    unsafe {
                        libc::kill(proc.id() as i32, libc::SIGTERM);
                    }
                    // Give the process a moment to flush and exit
                    match proc.try_wait() {
                        Ok(Some(_)) => {}
                        _ => {
                            std::thread::sleep(std::time::Duration::from_millis(500));
                            match proc.try_wait() {
                                Ok(Some(_)) => {}
                                _ => {
                                    eprintln!("[harness] {} did not exit after SIGTERM, killing", name);
                                    let _ = proc.kill();
                                    let _ = proc.wait();
                                }
                            }
                        }
                    }
                }
                #[cfg(not(unix))]
                {
                    let _ = name;
                    let _ = proc.kill();
                    let _ = proc.wait();
                }
            }
            *proc_opt = None;
        }
    }
}

impl Drop for TestHarness {
    fn drop(&mut self) {
        self.stop();
    }
}

/// A long-lived SSE stream reader that collects events in a background task.
///
/// Unlike `stream_sse_until_done`, this keeps the connection alive so the test
/// can interact with the approval API while events continue to arrive.
pub struct SseStream {
    events: Arc<std::sync::Mutex<Vec<SseEvent>>>,
    _handle: tokio::task::JoinHandle<()>,
}

impl SseStream {
    /// Connect to the SSE stream for a conversation and start collecting events
    /// in a background task. Events are logged to the console as they arrive.
    pub async fn connect(bridge_base_url: &str, conv_id: &str) -> Result<Self> {
        let stream_client = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .build()
            .context("failed to build stream client")?;

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("GET stream request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!("stream endpoint returned {}: {}", status, body));
        }

        let events: Arc<std::sync::Mutex<Vec<SseEvent>>> =
            Arc::new(std::sync::Mutex::new(Vec::new()));
        let events_clone = events.clone();
        let log_conv_id = conv_id.to_string();

        let handle = tokio::spawn(async move {
            use futures::StreamExt;
            let mut stream = resp.bytes_stream();
            let mut buffer = String::new();
            let mut current_event_type = String::new();

            loop {
                match stream.next().await {
                    Some(Ok(chunk)) => {
                        buffer.push_str(&String::from_utf8_lossy(&chunk));
                    }
                    _ => break,
                }

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

                        let data: serde_json::Value = serde_json::from_str(data_str)
                            .unwrap_or_else(|_| {
                                serde_json::Value::String(data_str.to_string())
                            });

                        let event_type = if !current_event_type.is_empty() {
                            current_event_type.clone()
                        } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                            t.to_string()
                        } else {
                            "message".to_string()
                        };

                        let event = SseEvent {
                            event_type,
                            data,
                        };

                        // Log to console like stream_sse_until_done does
                        let formatted = format_sse_for_log(&event.event_type, &event.data);
                        let short_id = if log_conv_id.len() > 8 {
                            &log_conv_id[..8]
                        } else {
                            &log_conv_id
                        };
                        for line in formatted.lines() {
                            eprintln!("[conv:{}] [SSE:{}] {}", short_id, event.event_type, line);
                        }

                        events_clone.lock().unwrap().push(event);
                        current_event_type.clear();
                    }
                }
            }
        });

        Ok(Self {
            events,
            _handle: handle,
        })
    }

    /// Wait until an event of the given type appears, or timeout.
    pub async fn wait_for_event(
        &self,
        event_type: &str,
        timeout: Duration,
    ) -> Option<SseEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                if let Some(e) = events.iter().find(|e| e.event_type == event_type) {
                    return Some(e.clone());
                }
            }
            if Instant::now() >= deadline {
                return None;
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Wait until the "done" event appears, or timeout. Returns all collected events.
    pub async fn wait_for_done(&self, timeout: Duration) -> Vec<SseEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                if events.iter().any(|e| e.event_type == "done") {
                    return events.clone();
                }
            }
            if Instant::now() >= deadline {
                let events = self.events.lock().unwrap();
                return events.clone();
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Get a snapshot of all events collected so far.
    pub fn events(&self) -> Vec<SseEvent> {
        self.events.lock().unwrap().clone()
    }
}
