use std::sync::Arc;

use adk_rust::prelude::*;
use mcp::McpRegistry;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;

const TOOL_NAME: &str = "load_tools";
const TOOL_DESCRIPTION: &str =
    "Load MCP tool schemas into the agent for use. Tool names are listed in the system prompt. \
     Load ALL tools you need for the current task in ONE call to minimize latency. \
     Example: load_tools(tool_names=[\"linear_get_issue\", \"linear_search_issues\"])";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct LoadToolsArgs {
    pub tool_names: Vec<String>,
}

pub struct LoadToolsTool {
    registry: Arc<McpRegistry>,
}

impl LoadToolsTool {
    pub fn new(registry: Arc<McpRegistry>) -> Self {
        Self { registry }
    }

    pub fn into_adk_tool(self) -> Arc<dyn Tool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<LoadToolsArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: LoadToolsArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        if parsed.tool_names.is_empty() {
            return Err(AdkError::tool("tool_names must not be empty"));
        }
        let loaded = self.registry.load_tools(&parsed.tool_names);
        Ok(serde_json::json!({
            "loaded": loaded,
            "total_loaded": self.registry.loaded_tool_names().len(),
            "still_unloaded": self.registry.unloaded_tool_names().len(),
        }))
    }
}
