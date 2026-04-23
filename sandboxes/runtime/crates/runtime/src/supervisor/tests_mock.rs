use std::any::Any;
use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use tools::ToolExecutor;

/// Minimal mock tool executor for testing.
pub(super) struct MockTool {
    name: String,
}

impl MockTool {
    pub(super) fn new_arc(name: &str) -> Arc<dyn ToolExecutor> {
        Arc::new(Self {
            name: name.to_string(),
        })
    }
}

#[async_trait::async_trait]
impl ToolExecutor for MockTool {
    fn name(&self) -> &str {
        &self.name
    }
    fn description(&self) -> &str {
        "mock tool"
    }
    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::json!({})
    }
    async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
        Ok("ok".to_string())
    }
    fn as_any(&self) -> &dyn Any {
        self
    }
}

/// Helper: build a tool_names set and tool_executors map from a list of names.
pub(super) fn make_tools(
    names: &[&str],
) -> (HashSet<String>, HashMap<String, Arc<dyn ToolExecutor>>) {
    let tool_names: HashSet<String> = names.iter().map(|n| n.to_string()).collect();
    let tool_executors: HashMap<String, Arc<dyn ToolExecutor>> = names
        .iter()
        .map(|n| (n.to_string(), MockTool::new_arc(n)))
        .collect();
    (tool_names, tool_executors)
}
