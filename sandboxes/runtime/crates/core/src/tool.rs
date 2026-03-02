use serde::{Deserialize, Serialize};

/// Definition of a tool that an agent can use.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ToolDefinition {
    /// Unique name of the tool
    pub name: String,
    /// Human-readable description of what the tool does
    pub description: String,
    /// JSON Schema for the tool's parameters
    pub parameters_schema: serde_json::Value,
}
