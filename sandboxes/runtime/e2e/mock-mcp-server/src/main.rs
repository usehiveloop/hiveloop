use rmcp::{
    handler::server::{router::tool::ToolRouter, wrapper::Parameters},
    model::{ServerCapabilities, ServerInfo},
    schemars, tool, tool_router,
    transport::io::stdio,
    ServerHandler, ServiceExt,
};
use serde::Deserialize;

/// Parameters for the "echo" tool.
#[derive(Debug, Deserialize, schemars::JsonSchema)]
pub struct EchoParams {
    /// The message to echo back.
    #[schemars(description = "The message to echo back")]
    pub message: String,
}

/// Parameters for the "add_numbers" tool.
#[derive(Debug, Deserialize, schemars::JsonSchema)]
pub struct AddNumbersParams {
    /// The first number.
    #[schemars(description = "The first number")]
    pub a: f64,
    /// The second number.
    #[schemars(description = "The second number")]
    pub b: f64,
}

/// A minimal MCP server for E2E testing.
///
/// Exposes three deterministic tools:
/// - echo: returns the input message
/// - add_numbers: returns the sum of two numbers
/// - get_time: returns a fixed timestamp for deterministic testing
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct MockMcpServer {
    tool_router: ToolRouter<Self>,
}

impl MockMcpServer {
    pub fn new() -> Self {
        Self {
            tool_router: Self::tool_router(),
        }
    }
}

impl Default for MockMcpServer {
    fn default() -> Self {
        Self::new()
    }
}

#[tool_router]
impl MockMcpServer {
    /// Echo the provided message back to the caller.
    #[tool(name = "echo", description = "Echo the provided message back")]
    fn echo(&self, Parameters(params): Parameters<EchoParams>) -> String {
        params.message
    }

    /// Add two numbers and return the sum as a string.
    #[tool(
        name = "add_numbers",
        description = "Add two numbers and return the sum"
    )]
    fn add_numbers(&self, Parameters(params): Parameters<AddNumbersParams>) -> String {
        let sum = params.a + params.b;
        // Format without trailing zeros for clean integer results (e.g. "5" not "5.0")
        if sum.fract() == 0.0 {
            format!("{}", sum as i64)
        } else {
            format!("{sum}")
        }
    }

    /// Return the current UTC timestamp as an ISO 8601 string.
    #[tool(name = "get_time", description = "Get the current UTC timestamp")]
    fn get_time(&self) -> String {
        chrono::Utc::now().to_rfc3339()
    }
}

impl ServerHandler for MockMcpServer {
    fn get_info(&self) -> ServerInfo {
        ServerInfo {
            instructions: Some("A mock MCP server for E2E testing".into()),
            capabilities: ServerCapabilities::builder().enable_tools().build(),
            ..Default::default()
        }
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let server = MockMcpServer::new();
    let service = server.serve(stdio()).await?;
    service.waiting().await?;
    Ok(())
}
