use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;

/// Trait for executing tools. All built-in and MCP tools implement this.
#[async_trait]
pub trait ToolExecutor: Send + Sync {
    /// The unique name of this tool
    fn name(&self) -> &str;
    /// Human-readable description
    fn description(&self) -> &str;
    /// JSON Schema for the tool's parameters
    fn parameters_schema(&self) -> serde_json::Value;
    /// Execute the tool with the given arguments
    async fn execute(&self, args: serde_json::Value) -> Result<String, String>;
}

/// Registry of available tools, combining built-in and MCP-discovered tools.
pub struct ToolRegistry {
    builtin_tools: HashMap<String, Arc<dyn ToolExecutor>>,
}

impl ToolRegistry {
    /// Create a new empty registry.
    pub fn new() -> Self {
        Self {
            builtin_tools: HashMap::new(),
        }
    }

    /// Register a tool in the registry.
    pub fn register(&mut self, tool: Arc<dyn ToolExecutor>) {
        self.builtin_tools.insert(tool.name().to_string(), tool);
    }

    /// Look up a tool by name.
    pub fn get(&self, name: &str) -> Option<Arc<dyn ToolExecutor>> {
        self.builtin_tools.get(name).cloned()
    }

    /// List all registered tools as (name, description) pairs.
    pub fn list(&self) -> Vec<(&str, &str)> {
        self.builtin_tools
            .values()
            .map(|t| (t.name(), t.description()))
            .collect()
    }

    /// Merge another registry's tools into this one without overwriting existing tools.
    pub fn merge(&mut self, other: ToolRegistry) {
        for (name, tool) in other.builtin_tools {
            self.builtin_tools.entry(name).or_insert(tool);
        }
    }
}

impl Default for ToolRegistry {
    fn default() -> Self {
        Self::new()
    }
}
