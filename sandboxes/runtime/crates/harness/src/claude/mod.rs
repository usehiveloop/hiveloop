//! Claude Code adapter — wraps `@agentclientprotocol/claude-agent-acp`.
//!
//! Spawns the Node binary as a child process and hands its stdio to the
//! shared [`AcpSession`] driver. claude-agent-acp specifics (settings.json,
//! `_meta.claudeCode.options`, the `IS_SANDBOX` env quirk) live here.

mod settings;

use crate::acp_session::{AcpSession, HarnessAdapter};
use crate::skills;
use bridge_core::{AgentDefinition, BridgeError};
use serde_json::{json, Value};
use std::path::PathBuf;
use std::process::Stdio;
use std::sync::Arc;
use tokio::process::Command;
use tracing::info;
use webhooks::{EventBus, PermissionManager};

/// Options used to launch the Claude ACP agent process.
pub struct ClaudeHarnessOptions {
    pub command: String,
    pub args: Vec<String>,
    pub working_dir: PathBuf,
    pub config_dir: PathBuf,
    pub extra_env: Vec<(String, String)>,
}

impl ClaudeHarnessOptions {
    pub fn from_env() -> Self {
        Self {
            command: std::env::var("BRIDGE_CLAUDE_ACP_COMMAND")
                .unwrap_or_else(|_| "claude-agent-acp".to_string()),
            args: std::env::var("BRIDGE_CLAUDE_ACP_ARGS")
                .ok()
                .map(|s| s.split_whitespace().map(String::from).collect())
                .unwrap_or_default(),
            working_dir: std::env::var("BRIDGE_WORKING_DIR")
                .map(PathBuf::from)
                .unwrap_or_else(|_| std::env::current_dir().unwrap_or_else(|_| PathBuf::from("/"))),
            config_dir: std::env::var("CLAUDE_CONFIG_DIR")
                .map(PathBuf::from)
                .unwrap_or_else(|_| PathBuf::from("/tmp/claude-state")),
            extra_env: Vec::new(),
        }
    }
}

/// Spawn the Claude ACP harness, run init, and return a handle the
/// supervisor can dispatch into.
pub async fn spawn(
    agent: AgentDefinition,
    opts: ClaudeHarnessOptions,
    event_bus: Arc<EventBus>,
    permission_manager: Arc<PermissionManager>,
) -> Result<Arc<AcpSession>, BridgeError> {
    settings::write_settings(&opts.config_dir, &agent);
    if !agent.skills.is_empty() {
        skills::write_skills(&opts.config_dir, &agent.skills);
    }

    let mut cmd = Command::new(&opts.command);
    cmd.args(&opts.args);
    cmd.current_dir(&opts.working_dir);
    for (k, v) in &opts.extra_env {
        cmd.env(k, v);
    }
    // claude-agent-acp downgrades bypassPermissions to default when running
    // as root unless IS_SANDBOX=1 is set. Bridge processes typically run as
    // root inside containers, so we opt the agent process into the sandbox
    // bypass when (and only when) the agent's config asks for it.
    if agent.config.permission_mode.as_deref() == Some("bypassPermissions") {
        cmd.env("IS_SANDBOX", "1");
    }
    cmd.stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());

    info!(
        command = %opts.command,
        args = ?opts.args,
        cwd = %opts.working_dir.display(),
        "spawning claude-agent-acp"
    );

    let mut child = cmd
        .spawn()
        .map_err(|e| BridgeError::HarnessError(format!("failed to spawn claude-agent-acp: {e}")))?;
    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| BridgeError::HarnessError("claude-agent-acp stdin missing".into()))?;
    let stdout = child
        .stdout
        .take()
        .ok_or_else(|| BridgeError::HarnessError("claude-agent-acp stdout missing".into()))?;
    if let Some(stderr) = child.stderr.take() {
        tokio::spawn(crate::stderr::pipe_claude(stderr));
    }

    AcpSession::start(
        agent,
        opts.working_dir.clone(),
        stdin,
        stdout,
        child,
        event_bus,
        permission_manager,
        Arc::new(ClaudeAdapter),
    )
    .await
}

struct ClaudeAdapter;

impl HarnessAdapter for ClaudeAdapter {
    fn name(&self) -> &'static str {
        "claude"
    }

    fn build_session_meta(
        &self,
        agent: &AgentDefinition,
        api_key_override: Option<&str>,
        provider_override: Option<&bridge_core::ProviderConfig>,
    ) -> Option<serde_json::Map<String, Value>> {
        let mut options = serde_json::Map::new();

        if !agent.system_prompt.trim().is_empty() {
            options.insert(
                "systemPrompt".to_string(),
                json!({ "append": agent.system_prompt }),
            );
        }
        if !agent.config.allowed_tools.is_empty() {
            options.insert(
                "allowedTools".to_string(),
                Value::Array(
                    agent
                        .config
                        .allowed_tools
                        .iter()
                        .map(|t| Value::String(t.clone()))
                        .collect(),
                ),
            );
        }
        if !agent.config.disabled_tools.is_empty() {
            options.insert(
                "disallowedTools".to_string(),
                Value::Array(
                    agent
                        .config
                        .disabled_tools
                        .iter()
                        .map(|t| Value::String(t.clone()))
                        .collect(),
                ),
            );
        }
        if let Some(mode) = &agent.config.permission_mode {
            options.insert("permissionMode".to_string(), json!(mode));
        }
        if let Some(model) = provider_override.map(|p| p.model.clone()) {
            options.insert("model".to_string(), json!(model));
        }
        if let Some(extra) = api_key_override {
            let mut env_obj = options
                .get("env")
                .and_then(|v| v.as_object().cloned())
                .unwrap_or_default();
            env_obj.insert("ANTHROPIC_API_KEY".to_string(), json!(extra));
            options.insert("env".to_string(), Value::Object(env_obj));
        }

        if options.is_empty() {
            None
        } else {
            let mut meta = serde_json::Map::new();
            meta.insert(
                "claudeCode".to_string(),
                json!({ "options": Value::Object(options) }),
            );
            Some(meta)
        }
    }
}
