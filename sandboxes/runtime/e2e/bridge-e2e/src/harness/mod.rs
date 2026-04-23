use std::collections::HashMap;
use std::path::PathBuf;
use std::process::Child;
use std::sync::Mutex;

mod accessors;
mod approvals_api;
mod bridge_api;
mod converse;
mod logging;
mod process;
mod push_api;
mod sse_reader;
mod sse_stream;
mod sse_until_event;
mod start;
mod start_real;
mod start_websocket;
mod tool_log;
mod types;
mod webhooks_api;
mod ws_stream;

pub use sse_stream::SseStream;
pub use types::{ConversationTurn, SseEvent, ToolCallLogEntry, WebhookEntry, WebhookLog};
pub use ws_stream::{WsEvent, WsEventStream};

/// End-to-end test harness that manages the mock control plane and bridge
/// processes. Each test should create its own harness to ensure isolation.
pub struct TestHarness {
    /// Port the mock control plane is listening on.
    pub mock_cp_port: u16,
    /// Port the bridge is listening on.
    pub bridge_port: u16,
    /// The mock control plane child process.
    pub(crate) mock_cp_process: Option<Child>,
    /// The bridge child process.
    pub(crate) bridge_process: Option<Child>,
    /// HTTP client for making requests.
    pub(crate) client: reqwest::Client,
    /// Full base URL for the bridge (e.g. "http://127.0.0.1:12345").
    pub(crate) bridge_base_url: String,
    /// Full base URL for the mock control plane.
    pub(crate) cp_base_url: String,
    /// Workspace root path.
    pub(crate) workspace_root: PathBuf,
    /// Tool call log directory (for real agent tests).
    pub(crate) tool_log_dir: Option<PathBuf>,
    /// Keeps the mock-control-plane stdout pipe alive so the process doesn't
    /// get EPIPE (broken pipe) when it writes after we've read the PORT= line.
    pub(crate) _cp_stdout_drain: Option<std::thread::JoinHandle<()>>,
    /// Directory for conversation log files (one per agent).
    pub(crate) log_dir: PathBuf,
    /// Maps conversation_id → agent_id for log file routing.
    pub(crate) conversation_agents: Mutex<HashMap<String, String>>,
}

impl Drop for TestHarness {
    fn drop(&mut self) {
        self.stop();
    }
}
