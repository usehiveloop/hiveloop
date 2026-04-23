use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;

// Re-export for convenience
extern crate strsim;

#[cfg(test)]
mod tests;

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
    ///
    /// **Ordering**: results are sorted by tool name. This is load-bearing for
    /// prompt caching — any reorder of the tools array invalidates the entire
    /// cache prefix, so the underlying HashMap iteration order must not leak.
    pub fn list(&self) -> Vec<(&str, &str)> {
        let mut out: Vec<(&str, &str)> = self
            .builtin_tools
            .values()
            .map(|t| (t.name(), t.description()))
            .collect();
        out.sort_by(|a, b| a.0.cmp(b.0));
        out
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

    /// Return all registered tool names, sorted alphabetically.
    ///
    /// Sort order is load-bearing for prompt caching — see `list`.
    pub fn tool_names(&self) -> Vec<String> {
        let mut names: Vec<String> = self.builtin_tools.keys().cloned().collect();
        names.sort();
        names
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
