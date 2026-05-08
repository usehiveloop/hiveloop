use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use domain::BashConfig;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};

use crate::operations::{BashExecOptions, BashOperations};
use crate::process_registry::ProcessRegistry;
use crate::truncate::{truncate_tail, TruncationReason};

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
    process_registry: Option<Arc<ProcessRegistry>>,
}

impl BashTool {
    pub fn new(
        config: BashConfig,
        workspace_root: PathBuf,
        operations: Arc<dyn BashOperations>,
    ) -> Self {
        Self {
            config,
            workspace_root,
            operations,
            process_registry: None,
        }
    }

    pub fn with_process_registry(mut self, registry: Arc<ProcessRegistry>) -> Self {
        self.process_registry = Some(registry);
        self
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<BashArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: BashArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let command = parsed.command.trim();
        if command.is_empty() {
            return Err(AdkError::tool("`command` must not be empty"));
        }
        if let Some(matched) = command_matches_deny_pattern(command, &self.config.deny_patterns) {
            return Err(AdkError::tool(format!(
                "command rejected: matches deny pattern `{matched}`"
            )));
        }

        let workdir = resolve_workdir(&self.workspace_root, &self.config.workdir);
        if !workdir.exists() {
            return Err(AdkError::tool(format!(
                "workdir does not exist: {}",
                workdir.display()
            )));
        }

        let timeout = parsed
            .timeout_seconds
            .map(|seconds| seconds.max(1))
            .unwrap_or(self.config.timeout_seconds.max(1));

        let mut env: HashMap<String, String> = HashMap::new();
        for key in &self.config.env_passthrough {
            if let Ok(value) = std::env::var(key) {
                env.insert(key.clone(), value);
            }
        }
        env.entry("HOME".into())
            .or_insert_with(|| std::env::var("HOME").unwrap_or_default());
        env.entry("PATH".into())
            .or_insert_with(|| std::env::var("PATH").unwrap_or_default());

        if parsed.run_in_background {
            let registry = self.process_registry.as_ref()
                .ok_or_else(|| AdkError::tool("background processes not available"))?;
            let process_id = registry.spawn(command, env, timeout as u64);
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
            .map_err(|e| AdkError::tool(format!("bash exec: {e}")))?;
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

fn command_matches_deny_pattern<'a>(
    command: &str,
    deny_patterns: &'a [String],
) -> Option<&'a str> {
    for pattern in deny_patterns {
        if !pattern.is_empty() && command.contains(pattern) {
            return Some(pattern);
        }
    }
    None
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
