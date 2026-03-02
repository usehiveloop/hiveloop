use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Definition of an MCP server that an agent connects to.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct McpServerDefinition {
    /// Name of the MCP server
    pub name: String,
    /// Transport configuration for connecting to the server
    pub transport: McpTransport,
}

/// Transport configuration for MCP server connections.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum McpTransport {
    /// Stdio transport — spawns a child process
    Stdio {
        /// Command to execute
        command: String,
        /// Command arguments
        #[serde(default)]
        args: Vec<String>,
        /// Environment variables for the process
        #[serde(default)]
        env: HashMap<String, String>,
    },
    /// Streamable HTTP transport — connects to an HTTP endpoint
    StreamableHttp {
        /// Server URL
        url: String,
        /// Additional HTTP headers
        #[serde(default)]
        headers: HashMap<String, String>,
    },
}
