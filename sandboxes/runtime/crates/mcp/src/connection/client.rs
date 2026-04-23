use bridge_core::mcp::McpTransport;
use bridge_core::BridgeError;
use rmcp::service::{Peer, RoleClient, RunningService};
use rmcp::ServiceExt;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Information about a tool discovered from an MCP server.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpToolInfo {
    /// Tool name
    pub name: String,
    /// Tool description
    pub description: String,
    /// JSON Schema for the tool's input
    pub input_schema: serde_json::Value,
}

/// A connection to a single MCP server.
pub struct McpConnection {
    running: RunningService<RoleClient, ()>,
    server_name: String,
}

impl McpConnection {
    /// Connect to an MCP server using stdio transport.
    pub async fn connect_stdio(
        server_name: &str,
        command: &str,
        args: &[String],
        env: &HashMap<String, String>,
    ) -> Result<Self, BridgeError> {
        use rmcp::transport::ConfigureCommandExt;
        use rmcp::transport::TokioChildProcess;

        let transport =
            TokioChildProcess::new(tokio::process::Command::new(command).configure(|cmd| {
                cmd.args(args);
                for (k, v) in env {
                    cmd.env(k, v);
                }
            }))
            .map_err(|e| {
                BridgeError::McpError(format!("failed to spawn MCP server '{}': {}", command, e))
            })?;

        let running = ().serve(transport).await.map_err(|e| {
            BridgeError::McpError(format!(
                "failed to initialize MCP connection '{}': {}",
                server_name, e
            ))
        })?;

        Ok(Self {
            running,
            server_name: server_name.to_string(),
        })
    }

    /// Connect to an MCP server using streamable HTTP transport.
    pub async fn connect_http(
        server_name: &str,
        url: &str,
        headers: &HashMap<String, String>,
    ) -> Result<Self, BridgeError> {
        use http::{HeaderName, HeaderValue};
        use rmcp::transport::streamable_http_client::StreamableHttpClientTransportConfig;
        use rmcp::transport::StreamableHttpClientTransport;

        let mut custom_headers = HashMap::new();
        for (k, v) in headers {
            let name = HeaderName::from_bytes(k.as_bytes()).map_err(|e| {
                BridgeError::McpError(format!("invalid header name '{}': {}", k, e))
            })?;
            let value = HeaderValue::from_str(v).map_err(|e| {
                BridgeError::McpError(format!("invalid header value for '{}': {}", k, e))
            })?;
            custom_headers.insert(name, value);
        }

        let config =
            StreamableHttpClientTransportConfig::with_uri(url).custom_headers(custom_headers);
        let transport = StreamableHttpClientTransport::from_config(config);

        let running = ().serve(transport).await.map_err(|e| {
            BridgeError::McpError(format!(
                "failed to initialize HTTP MCP connection '{}': {}",
                server_name, e
            ))
        })?;

        Ok(Self {
            running,
            server_name: server_name.to_string(),
        })
    }

    /// Connect using a McpTransport configuration.
    pub async fn connect(server_name: &str, transport: &McpTransport) -> Result<Self, BridgeError> {
        match transport {
            McpTransport::Stdio { command, args, env } => {
                Self::connect_stdio(server_name, command, args, env).await
            }
            McpTransport::StreamableHttp { url, headers } => {
                Self::connect_http(server_name, url, headers).await
            }
        }
    }

    /// List all tools provided by this MCP server.
    pub async fn list_tools(&self) -> Result<Vec<McpToolInfo>, BridgeError> {
        let tools = self.running.peer().list_all_tools().await.map_err(|e| {
            BridgeError::McpError(format!(
                "failed to list tools from '{}': {}",
                self.server_name, e
            ))
        })?;

        Ok(tools
            .into_iter()
            .map(|t| McpToolInfo {
                name: t.name.to_string(),
                description: t.description.unwrap_or_default().to_string(),
                input_schema: serde_json::to_value(&t.input_schema).unwrap_or_default(),
            })
            .collect())
    }

    /// Call a tool on this MCP server.
    pub async fn call_tool(
        &self,
        name: &str,
        args: serde_json::Value,
    ) -> Result<serde_json::Value, BridgeError> {
        use rmcp::model::CallToolRequestParams;

        let params = CallToolRequestParams {
            name: name.to_string().into(),
            arguments: args.as_object().cloned(),
            meta: None,
            task: None,
        };

        let result = self.running.peer().call_tool(params).await.map_err(|e| {
            BridgeError::McpError(format!(
                "failed to call tool '{}' on '{}': {}",
                name, self.server_name, e
            ))
        })?;

        let content_json: Vec<serde_json::Value> = result
            .content
            .iter()
            .map(|c| serde_json::to_value(c).unwrap_or_default())
            .collect();

        Ok(serde_json::json!({
            "content": content_json,
            "is_error": result.is_error.unwrap_or(false),
        }))
    }

    /// Get a reference to the peer for direct API calls.
    pub fn peer(&self) -> &Peer<RoleClient> {
        self.running.peer()
    }

    /// Get the server name.
    pub fn server_name(&self) -> &str {
        &self.server_name
    }

    /// Disconnect from the MCP server.
    pub async fn disconnect(self) {
        let ct = self.running.cancellation_token();
        ct.cancel();
    }
}
