use bridge_core::BridgeError;
use rig::completion::ToolDefinition;
use rig::tool::Tool;
use std::sync::Arc;

use tools::ToolExecutor;

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
        ToolDefinition {
            name: self.executor.name().to_string(),
            description: self.executor.description().to_string(),
            parameters: self.executor.parameters_schema(),
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
mod tests {
    use super::*;
    use async_trait::async_trait;
    use serde_json::json;

    struct MockTool;

    #[async_trait]
    impl ToolExecutor for MockTool {
        fn name(&self) -> &str {
            "mock_tool"
        }
        fn description(&self) -> &str {
            "A mock tool for testing"
        }
        fn parameters_schema(&self) -> serde_json::Value {
            json!({
                "type": "object",
                "properties": {
                    "input": { "type": "string" }
                }
            })
        }
        async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
            let input = args.get("input").and_then(|v| v.as_str()).unwrap_or("none");
            Ok(format!("mock result: {}", input))
        }
    }

    #[tokio::test]
    async fn test_dynamic_tool_delegates_to_executor() {
        let executor: Arc<dyn ToolExecutor> = Arc::new(MockTool);
        let tool = DynamicTool::new(executor);

        assert_eq!(tool.name(), "mock_tool");

        let def = tool.definition("test".to_string()).await;
        assert_eq!(def.name, "mock_tool");
        assert_eq!(def.description, "A mock tool for testing");

        let result = tool.call(json!({"input": "hello"})).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), "mock result: hello");
    }

    #[test]
    fn test_adapt_tools() {
        let executors: Vec<Arc<dyn ToolExecutor>> = vec![Arc::new(MockTool)];
        let tools = adapt_tools(executors).unwrap();
        assert_eq!(tools.len(), 1);
    }
}
