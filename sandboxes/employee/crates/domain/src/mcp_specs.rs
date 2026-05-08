use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "transport", rename_all = "snake_case")]
pub enum McpSpec {
    Stdio {
        name: String,
        command: String,
        #[serde(default)]
        args: Vec<String>,
        #[serde(default)]
        env: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        startup_timeout_seconds: Option<u32>,
    },
    Http {
        name: String,
        url: String,
        #[serde(default)]
        auth: Option<McpAuth>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
    },
}

impl McpSpec {
    pub fn name(&self) -> &str {
        match self {
            Self::Stdio { name, .. } | Self::Http { name, .. } => name,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum McpAuth {
    Bearer { token_env: String },
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ToolFilter {
    #[serde(default)]
    pub allow: Option<Vec<String>>,
    #[serde(default)]
    pub deny: Option<Vec<String>>,
}
