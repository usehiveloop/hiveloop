use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use domain::BashConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::operations::{BashExecOptions, BashOperations};
use crate::process_registry::ProcessRegistry;
use crate::truncate::{truncate_tail, TruncationReason};
use crate::{schema_for, JsonTool, ToolDefinition};

const TOOL_NAME: &str = "bash";
const TOOL_DESCRIPTION: &str =
    "Run a shell command in the workspace and return its combined stdout/stderr. \
     Output is truncated to the last 2000 lines or 50KB, whichever comes first. \
     Set run_in_background=true for commands that take a long time. Use \
     check_bash_status to poll progress. \
     Commands matching a denied pattern are rejected before execution.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct BashArgs {
    pub command: String,
    #[serde(default)]
    pub timeout_seconds: Option<u32>,
    #[serde(default)]
    pub run_in_background: bool,
}

pub struct BashTool {
    config: BashConfig,
    workspace_root: PathBuf,
    operations: Arc<dyn BashOperations>,
    runtime_env: Arc<HashMap<String, String>>,
    process_registry: Option<Arc<ProcessRegistry>>,
    session_id: Option<String>,
}

impl BashTool {
    pub fn new(
        config: BashConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn BashOperations>,
        runtime_env: Arc<HashMap<String, String>>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            operations,
            runtime_env,
            process_registry: None,
            session_id: None,
        }
    }

    pub fn with_process_registry(mut self, registry: Arc<ProcessRegistry>) -> Self {
        self.process_registry = Some(registry);
        self
    }

    pub fn with_session_id(mut self, session_id: impl Into<String>) -> Self {
        self.session_id = Some(session_id.into());
        self
    }

    pub fn into_tool(self) -> Arc<dyn JsonTool> {
        Arc::new(self)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: BashArgs =
            serde_json::from_value(args).map_err(|e| anyhow!("invalid arguments: {e}"))?;
        let command = parsed.command.trim();
        if command.is_empty() {
            return Err(anyhow!("`command` must not be empty"));
        }
        if let Some(matched) = command_matches_deny_pattern(command, &self.config.deny_patterns) {
            return Err(anyhow!(
                "command rejected: matches deny pattern `{matched}`"
            ));
        }

        let workdir = resolve_workdir(&self.workspace_root, &self.config.workdir);
        if !workdir.exists() {
            return Err(anyhow!("workdir does not exist: {}", workdir.display()));
        }

        let timeout = parsed
            .timeout_seconds
            .map(|seconds| seconds.max(1))
            .unwrap_or(self.config.timeout_seconds.max(1));

        let mut env: HashMap<String, String> = HashMap::new();
        for key in &self.config.env_passthrough {
            if let Some(value) = self
                .runtime_env
                .get(key)
                .cloned()
                .or_else(|| std::env::var(key).ok())
            {
                env.insert(key.clone(), value);
            }
        }
        env.entry("HOME".into())
            .or_insert_with(|| std::env::var("HOME").unwrap_or_default());
        env.entry("PATH".into())
            .or_insert_with(|| std::env::var("PATH").unwrap_or_default());

        if parsed.run_in_background {
            let registry = self
                .process_registry
                .as_ref()
                .ok_or_else(|| anyhow!("background processes not available"))?;
            let process_id = registry.spawn(command, env, timeout as u64, self.session_id.clone());
            return Ok(json!({
                "background": true,
                "process_id": process_id,
                "command": command,
            }));
        }

        let options = BashExecOptions {
            workdir,
            env,
            timeout: Some(Duration::from_secs(timeout as u64)),
            max_output_bytes: self.config.max_output_bytes,
        };

        let result = self
            .operations
            .exec(command, options)
            .await
            .map_err(|e| anyhow!("bash exec: {e}"))?;
        let output_text = String::from_utf8_lossy(&result.stdout_combined).to_string();
        let truncated = truncate_tail(&output_text, 2000, 50 * 1024);

        Ok(json!({
            "command": command,
            "exit_code": result.exit_code,
            "timed_out": result.timed_out,
            "truncated": truncated.truncated || result.truncated,
            "truncated_by": match truncated.truncated_by {
                TruncationReason::NotTruncated => "none",
                TruncationReason::Lines => "lines",
                TruncationReason::Bytes => "bytes",
            },
            "shown_lines": truncated.output_lines,
            "shown_bytes": truncated.output_bytes,
            "total_lines": truncated.total_lines,
            "total_bytes": truncated.total_bytes,
            "output": truncated.content,
        }))
    }
}

#[async_trait]
impl JsonTool for BashTool {
    fn definition(&self) -> ToolDefinition {
        ToolDefinition {
            name: TOOL_NAME.to_string(),
            description: TOOL_DESCRIPTION.to_string(),
            parameters: schema_for::<BashArgs>(),
        }
    }

    async fn call(&self, args: Value) -> Result<Value> {
        self.execute(args).await
    }
}

fn command_matches_deny_pattern<'a>(command: &str, deny_patterns: &'a [String]) -> Option<&'a str> {
    deny_patterns
        .iter()
        .find(|pattern| !pattern.is_empty() && command.contains(pattern.as_str()))
        .map(String::as_str)
}

fn resolve_workdir(workspace_root: &std::path::Path, configured: &str) -> PathBuf {
    if configured.trim().is_empty() {
        return workspace_root.to_path_buf();
    }
    let candidate = PathBuf::from(configured);
    if candidate.is_absolute() {
        candidate
    } else {
        workspace_root.join(candidate)
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::env;
    use std::sync::Arc;

    use async_trait::async_trait;

    use crate::operations::{BashError, BashExecOptions, BashExecResult, BashOperations};

    use super::BashConfig;

    struct EchoEnvOperations {
        key: &'static str,
    }

    #[async_trait]
    impl BashOperations for EchoEnvOperations {
        async fn exec(
            &self,
            _command: &str,
            options: BashExecOptions,
        ) -> Result<BashExecResult, BashError> {
            Ok(BashExecResult {
                stdout_combined: options
                    .env
                    .get(self.key)
                    .cloned()
                    .unwrap_or_default()
                    .into_bytes(),
                exit_code: Some(0),
                timed_out: false,
                truncated: false,
            })
        }
    }

    #[tokio::test]
    async fn runtime_env_overlays_process_for_bash_passthrough() {
        let runtime_env = Arc::new(HashMap::from([(
            "RUNTIME_ENV_OVERLAY".to_string(),
            "overlay-value".to_string(),
        )]));
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: vec!["RUNTIME_ENV_OVERLAY".to_string()],
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations {
                key: "RUNTIME_ENV_OVERLAY",
            }),
            runtime_env,
        );

        let original = env::var("RUNTIME_ENV_OVERLAY").ok();
        env::set_var("RUNTIME_ENV_OVERLAY", "process-value");
        let result = tool
            .execute(serde_json::json!({
                "command": "printf \"$RUNTIME_ENV_OVERLAY\"",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");
        match original {
            Some(value) => env::set_var("RUNTIME_ENV_OVERLAY", value),
            None => env::remove_var("RUNTIME_ENV_OVERLAY"),
        }

        assert_eq!(result["output"], "overlay-value");
    }

    #[tokio::test]
    async fn process_env_falls_back_when_runtime_overlay_missing() {
        let runtime_env = Arc::new(HashMap::new());
        let tool = super::BashTool::new(
            BashConfig {
                workdir: ".".to_string(),
                timeout_seconds: 1,
                max_output_bytes: 1024,
                deny_patterns: Vec::new(),
                env_passthrough: vec!["RUNTIME_ENV_FALLBACK".to_string()],
                sandbox: "process_isolated".to_string(),
            },
            env::temp_dir(),
            Arc::new(EchoEnvOperations {
                key: "RUNTIME_ENV_FALLBACK",
            }),
            runtime_env,
        );

        let original = env::var("RUNTIME_ENV_FALLBACK").ok();
        env::set_var("RUNTIME_ENV_FALLBACK", "process-fallback");
        let result = tool
            .execute(serde_json::json!({
                "command": "printf \"$RUNTIME_ENV_FALLBACK\"",
                "timeout_seconds": 1,
                "run_in_background": false,
            }))
            .await
            .expect("command should succeed");
        match original {
            Some(value) => env::set_var("RUNTIME_ENV_FALLBACK", value),
            None => env::remove_var("RUNTIME_ENV_FALLBACK"),
        }

        assert_eq!(result["output"], "process-fallback");
    }
}
