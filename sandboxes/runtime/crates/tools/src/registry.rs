use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;

// Re-export for convenience
extern crate strsim;

/// Trait for executing tools. All built-in and MCP tools implement this.
#[async_trait]
pub trait ToolExecutor: Send + Sync + Any {
    /// The unique name of this tool
    fn name(&self) -> &str;
    /// Human-readable description
    fn description(&self) -> &str;
    /// JSON Schema for the tool's parameters
    fn parameters_schema(&self) -> serde_json::Value;
    /// Execute the tool with the given arguments
    async fn execute(&self, args: serde_json::Value) -> Result<String, String>;
    /// Return self as Any for downcasting
    fn as_any(&self) -> &dyn Any;
}

use std::any::Any;

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

    /// Remove a tool from the registry by name.
    pub fn remove(&mut self, name: &str) {
        self.builtin_tools.remove(name);
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

    /// Return a snapshot of all currently registered tools as a HashMap.
    /// Used by the batch tool to get access to other tools.
    pub fn snapshot(&self) -> HashMap<String, Arc<dyn ToolExecutor>> {
        self.builtin_tools.clone()
    }

    /// Return all registered tool names.
    pub fn tool_names(&self) -> Vec<String> {
        self.builtin_tools.keys().cloned().collect()
    }

    /// Case-insensitive lookup. If the exact name doesn't match, tries
    /// a case-insensitive comparison. Returns the tool if found.
    pub fn get_case_insensitive(&self, name: &str) -> Option<Arc<dyn ToolExecutor>> {
        // Try exact match first
        if let Some(tool) = self.builtin_tools.get(name) {
            return Some(tool.clone());
        }

        // Try case-insensitive match
        let lower = name.to_lowercase();
        for (key, tool) in &self.builtin_tools {
            if key.to_lowercase() == lower {
                return Some(tool.clone());
            }
        }

        None
    }

    /// Suggest the closest tool name for an unknown tool name using
    /// Levenshtein distance. Returns `None` if no close match is found.
    pub fn suggest_tool(&self, name: &str) -> Option<String> {
        let lower = name.to_lowercase();
        let mut best: Option<(String, f64)> = None;

        for key in self.builtin_tools.keys() {
            let distance = strsim::normalized_levenshtein(&lower, &key.to_lowercase());
            if distance > best.as_ref().map_or(0.0, |(_, d)| *d) {
                best = Some((key.clone(), distance));
            }
        }

        // Only suggest if similarity is above 0.4
        best.filter(|(_, d)| *d > 0.4).map(|(name, _)| name)
    }

    /// Format an error message for an unknown tool name.
    /// Includes a suggestion if a close match exists.
    pub fn unknown_tool_error(&self, name: &str) -> String {
        let names = self.tool_names();
        if let Some(suggestion) = self.suggest_tool(name) {
            format!(
                "Unknown tool '{}'. Did you mean '{}'? Available tools: [{}]",
                name,
                suggestion,
                names.join(", ")
            )
        } else {
            format!(
                "Unknown tool '{}'. Available tools: [{}]",
                name,
                names.join(", ")
            )
        }
    }
}

/// Format a validation error into a structured, helpful message.
///
/// Extracts required fields from the JSON schema and presents a clear
/// error message instead of a raw validation dump.
pub fn format_validation_error(tool_name: &str, error: &str, schema: &serde_json::Value) -> String {
    // Extract required field names from the schema
    let required_fields: Vec<&str> = schema
        .get("properties")
        .or_else(|| {
            schema
                .get("$defs")
                .and_then(|d| d.as_object())
                .and_then(|defs| defs.values().find(|v| v.get("properties").is_some()))
                .and_then(|v| v.get("properties"))
        })
        .and_then(|_| {
            schema.get("required").or_else(|| {
                schema
                    .get("$defs")
                    .and_then(|d| d.as_object())
                    .and_then(|defs| defs.values().find(|v| v.get("required").is_some()))
                    .and_then(|v| v.get("required"))
            })
        })
        .and_then(|r| r.as_array())
        .map(|arr| arr.iter().filter_map(|v| v.as_str()).collect())
        .unwrap_or_default();

    // Simplify the error message
    let specific_issue = if error.contains("missing") || error.contains("required") {
        "missing required field(s)".to_string()
    } else if error.contains("type") || error.contains("expected") {
        "wrong type for field(s)".to_string()
    } else {
        error.lines().next().unwrap_or(error).to_string()
    };

    if required_fields.is_empty() {
        format!(
            "Invalid arguments for tool '{}': {}. See tool description for usage.",
            tool_name, specific_issue
        )
    } else {
        format!(
            "Invalid arguments for tool '{}': {}. Required fields: [{}]. See tool description for usage.",
            tool_name,
            specific_issue,
            required_fields.join(", ")
        )
    }
}

impl Default for ToolRegistry {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A minimal test tool.
    struct StubTool {
        tool_name: String,
    }

    #[async_trait]
    impl ToolExecutor for StubTool {
        fn name(&self) -> &str {
            &self.tool_name
        }
        fn description(&self) -> &str {
            "stub"
        }
        fn parameters_schema(&self) -> serde_json::Value {
            serde_json::json!({})
        }
        async fn execute(&self, _args: serde_json::Value) -> Result<String, String> {
            Ok("ok".to_string())
        }
        fn as_any(&self) -> &dyn std::any::Any {
            self
        }
    }

    fn make_registry() -> ToolRegistry {
        let mut reg = ToolRegistry::new();
        reg.register(Arc::new(StubTool {
            tool_name: "bash".to_string(),
        }));
        reg.register(Arc::new(StubTool {
            tool_name: "Read".to_string(),
        }));
        reg.register(Arc::new(StubTool {
            tool_name: "edit".to_string(),
        }));
        reg.register(Arc::new(StubTool {
            tool_name: "Grep".to_string(),
        }));
        reg
    }

    #[test]
    fn test_case_insensitive_lookup_exact() {
        let reg = make_registry();
        assert!(reg.get_case_insensitive("bash").is_some());
    }

    #[test]
    fn test_case_insensitive_lookup_wrong_case() {
        let reg = make_registry();
        // "Bash" should match "bash"
        assert!(reg.get_case_insensitive("Bash").is_some());
        // "read" should match "Read"
        assert!(reg.get_case_insensitive("read").is_some());
    }

    #[test]
    fn test_suggest_tool_close_match() {
        let reg = make_registry();
        // "rread" is close to "Read"
        let suggestion = reg.suggest_tool("rread");
        assert!(suggestion.is_some());
    }

    #[test]
    fn test_suggest_tool_no_match() {
        let reg = make_registry();
        let suggestion = reg.suggest_tool("zzzzzzzzz");
        assert!(suggestion.is_none());
    }

    #[test]
    fn test_unknown_tool_error_with_suggestion() {
        let reg = make_registry();
        let err = reg.unknown_tool_error("bassh");
        assert!(err.contains("Did you mean"));
        assert!(err.contains("bash"));
    }

    #[test]
    fn test_unknown_tool_error_no_suggestion() {
        let reg = make_registry();
        let err = reg.unknown_tool_error("zzzzzzzzz");
        assert!(err.contains("Unknown tool 'zzzzzzzzz'"));
        assert!(err.contains("Available tools:"));
        assert!(!err.contains("Did you mean"));
    }

    #[test]
    fn test_tool_names() {
        let reg = make_registry();
        let names = reg.tool_names();
        assert_eq!(names.len(), 4);
        assert!(names.contains(&"bash".to_string()));
        assert!(names.contains(&"Read".to_string()));
    }

    #[test]
    fn test_format_validation_error_missing_fields() {
        let schema = serde_json::json!({
            "properties": {
                "command": {"type": "string"},
                "timeout": {"type": "number"}
            },
            "required": ["command"]
        });
        let msg = format_validation_error("bash", "missing required property", &schema);
        assert!(msg.contains("Invalid arguments for tool 'bash'"));
        assert!(msg.contains("missing required"));
        assert!(msg.contains("command"));
    }

    #[test]
    fn test_format_validation_error_wrong_type() {
        let schema = serde_json::json!({
            "properties": {
                "command": {"type": "string"}
            },
            "required": ["command"]
        });
        let msg = format_validation_error("bash", "expected string, got number", &schema);
        assert!(msg.contains("wrong type"));
        assert!(msg.contains("command"));
    }

    #[test]
    fn test_format_validation_error_no_required_fields() {
        let schema = serde_json::json!({
            "properties": {
                "optional_field": {"type": "string"}
            }
        });
        let msg = format_validation_error("test", "some error", &schema);
        assert!(msg.contains("Invalid arguments for tool 'test'"));
        assert!(msg.contains("See tool description"));
    }
}
