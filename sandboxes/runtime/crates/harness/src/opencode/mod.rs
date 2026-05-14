//! OpenCode adapter — wraps the `opencode acp` subcommand.
//!
//! Spawns opencode (typically `opencode acp`) and hands its stdio to the
//! shared [`AcpSession`] driver. Per-harness state — model id, MCP
//! servers, permission rules, system prompt — is materialized into a
//! per-agent `opencode.json` referenced via the `OPENCODE_CONFIG`
//! env var. opencode's ACP layer doesn't read meta from
//! `NewSessionRequest`, so [`build_session_meta`] returns `None`.

mod settings;

use crate::acp_session::{AcpSession, HarnessAdapter};
use crate::skills;
use bridge_core::{AgentDefinition, BridgeError};
use std::path::PathBuf;
use std::process::Stdio;
use std::sync::Arc;
use tokio::process::Command;
use tracing::info;
use webhooks::{EventBus, PermissionManager};

/// Options used to launch the OpenCode ACP agent process.
pub struct OpenCodeHarnessOptions {
    pub command: String,
    pub args: Vec<String>,
    pub working_dir: PathBuf,
    /// Directory bridge owns for opencode-side state (config, instructions
    /// file, etc.). Ends up exported as `OPENCODE_CONFIG_DIR`.
    pub config_dir: PathBuf,
    pub extra_env: Vec<(String, String)>,
}

impl OpenCodeHarnessOptions {
    pub fn from_env() -> Self {
        Self {
            command: std::env::var("BRIDGE_OPENCODE_COMMAND")
                .unwrap_or_else(|_| "opencode".to_string()),
            args: std::env::var("BRIDGE_OPENCODE_ARGS")
                .ok()
                .map(|s| s.split_whitespace().map(String::from).collect())
                .unwrap_or_else(|| vec!["acp".to_string()]),
            working_dir: std::env::var("BRIDGE_WORKING_DIR")
                .map(PathBuf::from)
                .unwrap_or_else(|_| std::env::current_dir().unwrap_or_else(|_| PathBuf::from("/"))),
            config_dir: std::env::var("OPENCODE_CONFIG_DIR")
                .map(PathBuf::from)
                .unwrap_or_else(|_| PathBuf::from("/work/.opencode")),
            extra_env: Vec::new(),
        }
    }
}

/// Spawn the OpenCode ACP harness, run init, and return a handle the
/// supervisor can dispatch into.
pub async fn spawn(
    agent: AgentDefinition,
    opts: OpenCodeHarnessOptions,
    event_bus: Arc<EventBus>,
    permission_manager: Arc<PermissionManager>,
) -> Result<Arc<AcpSession>, BridgeError> {
    // Materialize skills onto disk (opencode auto-discovers SKILL.md
    // files inside `<OPENCODE_CONFIG_DIR>/skills/`; we also reference the
    // directory explicitly via `skills.paths` in the JSON below for
    // belt-and-suspenders).
    if !agent.skills.is_empty() {
        skills::write_skills(&opts.config_dir, &agent.skills);
    }

    // Materialize the agent definition into opencode's config + instructions
    // files before spawn. opencode reads these via OPENCODE_CONFIG_DIR.
    let config_path = settings::write_config(&opts.config_dir, &opts.working_dir, &agent)
        .map_err(BridgeError::HarnessError)?;

    let mut cmd = Command::new(&opts.command);
    cmd.args(&opts.args);
    cmd.current_dir(&opts.working_dir);
    // Tell opencode where to read its config from. OPENCODE_CONFIG points
    // at the exact file, OPENCODE_CONFIG_DIR points at the parent. We set
    // both so either lookup path resolves to bridge's per-agent file.
    cmd.env("OPENCODE_CONFIG", &config_path);
    cmd.env("OPENCODE_CONFIG_DIR", &opts.config_dir);
    for (k, v) in &opts.extra_env {
        cmd.env(k, v);
    }
    cmd.stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    info!(
        command = %opts.command,
        args = ?opts.args,
        cwd = %opts.working_dir.display(),
        config = %config_path.display(),
        "spawning opencode acp"
    );

    let mut child = cmd
        .spawn()
        .map_err(|e| BridgeError::HarnessError(format!("failed to spawn opencode acp: {e}")))?;
    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| BridgeError::HarnessError("opencode acp stdin missing".into()))?;
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| BridgeError::HarnessError("opencode acp stdout missing".into()))?;
    if let Some(stderr) = child.stderr.take() {
        tokio::spawn(crate::stderr::pipe_opencode(stderr));
    }

    AcpSession::start(
        agent,
        opts.working_dir.clone(),
        stdin,
        stdout,
        child,
        event_bus,
        permission_manager,
        Arc::new(OpenCodeAdapter),
    )
    .await
}

struct OpenCodeAdapter;

impl HarnessAdapter for OpenCodeAdapter {
    fn name(&self) -> &'static str {
        "opencode"
    }

    fn build_session_meta(
        &self,
        _agent: &AgentDefinition,
        _api_key_override: Option<&str>,
        _provider_override: Option<&bridge_core::ProviderConfig>,
    ) -> Option<serde_json::Map<String, serde_json::Value>> {
        // opencode's ACP layer doesn't honor `_meta` on NewSessionRequest;
        // all per-agent config is driven by the on-disk config file written
        // by `settings::write_config`.
        None
    }
}
