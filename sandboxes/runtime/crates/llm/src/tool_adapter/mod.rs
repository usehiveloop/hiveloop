use bridge_core::BridgeError;
use rig::completion::ToolDefinition;
use rig::tool::Tool;
use std::sync::Arc;

use tools::ToolExecutor;

mod schema;
use schema::flatten_schema;

/// A dynamic tool that adapts our `ToolExecutor` trait to rig-core's `Tool` trait.
///
/// This allows bridge-tools executors (Read, Glob, Grep, etc.) and MCP tools
/// to be used with rig-core's agent builder.
pub struct DynamicTool {
    executor: Arc<dyn ToolExecutor>,
}

impl DynamicTool {
    /// Create a new DynamicTool wrapping a ToolExecutor.
    pub fn new(executor: Arc<dyn ToolExecutor>) -> Self {
        Self { executor }
    }

    /// Synchronous companion to rig's async `definition()` — produces the same
    /// `ToolDefinition` (name, description, flattened schema) the agent builder
    /// will send, but callable without async context. Used for prefix-hash
    /// computation at agent build time.
    pub fn definition_sync(&self) -> ToolDefinition {
        let mut schema = self.executor.parameters_schema();
        flatten_schema(&mut schema);
        ToolDefinition {
            name: self.executor.name().to_string(),
            description: self.executor.description().to_string(),
            parameters: schema,
        }
    }
}

/// Error type for dynamic tool execution.
#[derive(Debug, thiserror::Error)]
#[error("{0}")]
pub struct DynamicToolError(pub String);

impl Tool for DynamicTool {
    const NAME: &'static str = "dynamic";

    type Error = DynamicToolError;
    type Args = serde_json::Value;
    type Output = String;

    fn name(&self) -> String {
        self.executor.name().to_string()
    }

    async fn definition(&self, _prompt: String) -> ToolDefinition {
        let mut schema = self.executor.parameters_schema();
        flatten_schema(&mut schema);
        ToolDefinition {
            name: self.executor.name().to_string(),
            description: self.executor.description().to_string(),
            parameters: schema,
        }
    }

    async fn call(&self, args: Self::Args) -> Result<Self::Output, Self::Error> {
        self.executor.execute(args).await.map_err(DynamicToolError)
    }
}

/// Adapt a list of ToolExecutors into DynamicTools for use with rig-core.
pub fn adapt_tools(executors: Vec<Arc<dyn ToolExecutor>>) -> Result<Vec<DynamicTool>, BridgeError> {
    Ok(executors.into_iter().map(DynamicTool::new).collect())
}

#[cfg(test)]
mod tests;
