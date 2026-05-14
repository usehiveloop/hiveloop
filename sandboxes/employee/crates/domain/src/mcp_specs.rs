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
        default_enabled_tools: Vec<String>,
        #[serde(default)]
        default_enable_all_tools: bool,
        #[serde(default)]
        startup_timeout_seconds: Option<u32>,
    },
    Http {
        name: String,
        url: String,
        #[serde(default)]
        headers: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        default_enabled_tools: Vec<String>,
        #[serde(default)]
        default_enable_all_tools: bool,
    },
    StreamableHttp {
        name: String,
        url: String,
        #[serde(default)]
        headers: HashMap<String, String>,
        #[serde(default)]
        tool_filter: Option<ToolFilter>,
        #[serde(default)]
        default_enabled_tools: Vec<String>,
        #[serde(default)]
        default_enable_all_tools: bool,
    },
}

impl McpSpec {
    pub fn name(&self) -> &str {
        match self {
            Self::Stdio { name, .. }
            | Self::Http { name, .. }
            | Self::StreamableHttp { name, .. } => name,
        }
    }

    pub fn default_enabled_tools(&self) -> &[String] {
        match self {
            Self::Stdio {
                default_enabled_tools,
                ..
            }
            | Self::Http {
                default_enabled_tools,
                ..
            }
            | Self::StreamableHttp {
                default_enabled_tools,
                ..
            } => default_enabled_tools,
        }
    }

    pub fn default_enable_all_tools(&self) -> bool {
        match self {
            Self::Stdio {
                default_enable_all_tools,
                ..
            }
            | Self::Http {
                default_enable_all_tools,
                ..
            }
            | Self::StreamableHttp {
                default_enable_all_tools,
                ..
            } => *default_enable_all_tools,
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ToolFilter {
    #[serde(default)]
    pub allow: Option<Vec<String>>,
    #[serde(default)]
    pub deny: Option<Vec<String>>,
}
