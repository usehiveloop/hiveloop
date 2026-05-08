use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", content = "config")]
pub enum ToolSpec {
    #[serde(rename = "builtin.bash")]
    Bash(BashConfig),
    #[serde(rename = "builtin.read_file")]
    ReadFile(ReadFileConfig),
    #[serde(rename = "builtin.write_file")]
    WriteFile(WriteFileConfig),
    #[serde(rename = "builtin.post_status_update")]
    PostStatusUpdate,
    #[serde(rename = "builtin.post_to_channel")]
    PostToChannel,
    #[serde(rename = "builtin.cron")]
    Cron,
    #[serde(rename = "builtin.delegate")]
    Delegate,
    #[serde(rename = "builtin.check_delegated_status")]
    CheckDelegatedStatus,
    #[serde(rename = "builtin.check_bash_status")]
    CheckBashStatus,
    #[serde(rename = "builtin.wake")]
    Wake,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BashConfig {
    pub workdir: String,
    pub timeout_seconds: u32,
    pub max_output_bytes: u64,
    #[serde(default)]
    pub deny_patterns: Vec<String>,
    #[serde(default)]
    pub env_passthrough: Vec<String>,
    #[serde(default = "default_sandbox")]
    pub sandbox: String,
}

fn default_sandbox() -> String {
    "process_isolated".into()
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReadFileConfig {
    pub allowed_roots: Vec<String>,
    pub max_file_size_bytes: u64,
    #[serde(default)]
    pub deny_globs: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WriteFileConfig {
    pub allowed_roots: Vec<String>,
    pub max_file_size_bytes: u64,
    #[serde(default)]
    pub deny_globs: Vec<String>,
    #[serde(default = "default_atomic")]
    pub atomic: bool,
}

fn default_atomic() -> bool {
    true
}



