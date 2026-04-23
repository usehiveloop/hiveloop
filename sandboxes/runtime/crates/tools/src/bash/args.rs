use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Arguments for the Bash tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct BashArgs {
    /// The shell command to execute. Example: 'ls -la /tmp'
    #[schemars(description = "The shell command to execute. Example: 'ls -la /tmp'")]
    pub command: String,
    /// Timeout in milliseconds. Default: 120000 (2 minutes). Maximum: 600000 (10 minutes).
    #[schemars(
        description = "Timeout in milliseconds. Default: 120000 (2 minutes). Maximum: 600000 (10 minutes)"
    )]
    pub timeout: Option<u64>,
    /// Working directory for the command. Defaults to current directory. Use this instead of 'cd <dir> && <cmd>'.
    #[schemars(
        description = "Working directory for the command. Defaults to current directory. Use this instead of 'cd <dir> && <cmd>'"
    )]
    pub workdir: Option<String>,
    /// A short description of what this command does in 5-10 words.
    #[schemars(description = "A short description of what this command does in 5-10 words")]
    pub description: Option<String>,
    /// Run this command in the background. Returns immediately with a task_id.
    /// The agent will be notified when the command completes.
    #[schemars(
        description = "Set to true to run in the background. Returns immediately with a task_id; you will be notified on completion"
    )]
    #[serde(default)]
    pub background: bool,
}

/// Result returned by the Bash tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct BashResult {
    pub output: String,
    pub exit_code: Option<i32>,
    pub timed_out: bool,
}
