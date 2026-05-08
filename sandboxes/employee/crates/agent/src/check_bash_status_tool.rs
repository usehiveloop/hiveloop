use std::sync::Arc;

use adk_rust::prelude::*;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tools::ProcessRegistry;

const TOOL_NAME: &str = "check_bash_status";
const TOOL_DESCRIPTION: &str =
    "Check the status of a background bash process. Returns whether it's running, \
     its exit code (if finished), and the last 10KB of output.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct CheckBashStatusArgs {
    pub process_id: String,
}

pub struct CheckBashStatusTool {
    registry: Arc<ProcessRegistry>,
}

impl CheckBashStatusTool {
    pub fn new(registry: Arc<ProcessRegistry>) -> Self {
        Self { registry }
    }

    pub fn into_adk_tool(self) -> Arc<dyn Tool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<CheckBashStatusArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: CheckBashStatusArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let Some(status) = self.registry.status(&parsed.process_id) else {
            return Err(AdkError::tool(format!(
                "process '{}' not found (may have expired)",
                parsed.process_id
            )));
        };
        Ok(serde_json::json!({
            "process_id": parsed.process_id,
            "running": status.running,
            "exit_code": status.exit_code,
            "output": status.output,
        }))
    }
}
