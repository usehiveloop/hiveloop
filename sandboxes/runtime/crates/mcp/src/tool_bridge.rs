use async_trait::async_trait;
use std::sync::Arc;

use crate::connection::{McpConnection, McpToolInfo};
use tools::ToolExecutor;

/// Wraps an MCP tool as a ToolExecutor so it can be used alongside built-in tools.
///
/// Each McpToolExecutor holds a reference to its MCP connection and the tool's
/// metadata, delegating execution to the remote MCP server.
pub struct McpToolExecutor {
    connection: Arc<McpConnection>,
    tool_info: McpToolInfo,
}

impl McpToolExecutor {
    /// Create a new MCP tool executor.
    pub fn new(connection: Arc<McpConnection>, tool_info: McpToolInfo) -> Self {
        Self {
            connection,
            tool_info,
        }
    }
}

#[async_trait]
impl ToolExecutor for McpToolExecutor {
    fn name(&self) -> &str {
        &self.tool_info.name
    }

    fn description(&self) -> &str {
        &self.tool_info.description
    }

    fn parameters_schema(&self) -> serde_json::Value {
        self.tool_info.input_schema.clone()
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let result = self
            .connection
            .call_tool(&self.tool_info.name, args)
            .await
            .map_err(|e| e.to_string())?;

        // Extract text content from the MCP result instead of serializing
        // the full MCP wrapper (which includes content array and is_error fields).
        // The LLM should receive just the tool output text, not protocol metadata.
        extract_text_from_mcp_result(&result)
    }
}

/// Bridge a list of MCP tools into ToolExecutor instances.
///
/// Takes a shared reference to the MCP connection and the tool metadata
/// discovered from that server, returning executor instances ready for
/// registration in a ToolRegistry.
pub fn bridge_mcp_tools(
    connection: Arc<McpConnection>,
    tools: Vec<McpToolInfo>,
) -> Vec<Arc<dyn ToolExecutor>> {
    tools
        .into_iter()
        .map(|tool_info| {
            Arc::new(McpToolExecutor::new(connection.clone(), tool_info)) as Arc<dyn ToolExecutor>
        })
        .collect()
}

/// Extract text content from an MCP call_tool result value.
///
/// The MCP result has the shape `{"content": [{"text": "...", "type": "text"}, ...], "is_error": bool}`.
/// This function extracts and concatenates all text entries, returning just the
/// meaningful content for the LLM. Falls back to full JSON serialization if no
/// text content is found.
fn extract_text_from_mcp_result(result: &serde_json::Value) -> Result<String, String> {
    if let Some(content) = result.get("content").and_then(|c| c.as_array()) {
        let texts: Vec<&str> = content
            .iter()
            .filter_map(|item| {
                if item.get("type").and_then(|t| t.as_str()) == Some("text") {
                    item.get("text").and_then(|t| t.as_str())
                } else {
                    None
                }
            })
            .collect();

        if !texts.is_empty() {
            return Ok(texts.join("\n"));
        }
    }

    // Fallback: serialize the full result if no text content found
    serde_json::to_string(result).map_err(|e| format!("failed to serialize result: {}", e))
}

#[cfg(test)]
mod tests {
    use super::*;
    use pretty_assertions::assert_eq;

    #[test]
    fn test_mcp_tool_info_name_and_description() {
        let tool_info = McpToolInfo {
            name: "test_tool".to_string(),
            description: "A test tool".to_string(),
            input_schema: serde_json::json!({"type": "object"}),
        };

        assert_eq!(tool_info.name, "test_tool");
        assert_eq!(tool_info.description, "A test tool");
    }

    #[test]
    fn test_mcp_tool_info_serialization_roundtrip() {
        let tool_info = McpToolInfo {
            name: "calculator".to_string(),
            description: "Performs math operations".to_string(),
            input_schema: serde_json::json!({
                "type": "object",
                "properties": {
                    "expression": { "type": "string" }
                }
            }),
        };

        let json = serde_json::to_string(&tool_info).expect("serialize");
        let deserialized: McpToolInfo = serde_json::from_str(&json).expect("deserialize");

        assert_eq!(deserialized.name, "calculator");
        assert_eq!(deserialized.description, "Performs math operations");
        assert_eq!(deserialized.input_schema, tool_info.input_schema);
    }

    #[test]
    fn test_mcp_tool_info_serialization_json_structure() {
        let tool_info = McpToolInfo {
            name: "my_tool".to_string(),
            description: "Does things".to_string(),
            input_schema: serde_json::json!({"type": "object"}),
        };

        let value = serde_json::to_value(&tool_info).expect("to_value");
        assert_eq!(value["name"], "my_tool");
        assert_eq!(value["description"], "Does things");
        assert_eq!(value["input_schema"]["type"], "object");
    }

    #[test]
    fn test_mcp_tool_info_deserialization_from_json_string() {
        let json = r#"{
            "name": "web_search",
            "description": "Search the web",
            "input_schema": {
                "type": "object",
                "properties": {
                    "query": { "type": "string" }
                },
                "required": ["query"]
            }
        }"#;

        let tool_info: McpToolInfo = serde_json::from_str(json).expect("deserialize");
        assert_eq!(tool_info.name, "web_search");
        assert_eq!(tool_info.description, "Search the web");
        assert_eq!(tool_info.input_schema["type"], "object");
        assert_eq!(tool_info.input_schema["required"][0], "query");
    }

    #[test]
    fn test_mcp_tool_info_with_empty_schema() {
        let tool_info = McpToolInfo {
            name: "no_params".to_string(),
            description: "A tool with no parameters".to_string(),
            input_schema: serde_json::json!({}),
        };

        let json = serde_json::to_string(&tool_info).expect("serialize");
        let deserialized: McpToolInfo = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(deserialized.input_schema, serde_json::json!({}));
    }

    #[test]
    fn test_mcp_tool_info_with_complex_schema() {
        let schema = serde_json::json!({
            "type": "object",
            "properties": {
                "url": { "type": "string", "format": "uri" },
                "method": { "type": "string", "enum": ["GET", "POST", "PUT", "DELETE"] },
                "headers": {
                    "type": "object",
                    "additionalProperties": { "type": "string" }
                },
                "body": { "type": "string" }
            },
            "required": ["url", "method"]
        });

        let tool_info = McpToolInfo {
            name: "http_request".to_string(),
            description: "Make an HTTP request".to_string(),
            input_schema: schema.clone(),
        };

        let roundtripped: McpToolInfo =
            serde_json::from_str(&serde_json::to_string(&tool_info).unwrap()).unwrap();
        assert_eq!(roundtripped.input_schema, schema);
    }

    #[test]
    fn test_mcp_tool_info_with_empty_name_and_description() {
        let tool_info = McpToolInfo {
            name: String::new(),
            description: String::new(),
            input_schema: serde_json::Value::Null,
        };

        let json = serde_json::to_string(&tool_info).expect("serialize");
        let deserialized: McpToolInfo = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(deserialized.name, "");
        assert_eq!(deserialized.description, "");
        assert_eq!(deserialized.input_schema, serde_json::Value::Null);
    }

    #[test]
    fn test_mcp_tool_info_clone() {
        let tool_info = McpToolInfo {
            name: "cloneable".to_string(),
            description: "Can be cloned".to_string(),
            input_schema: serde_json::json!({"type": "object"}),
        };

        let cloned = tool_info.clone();
        assert_eq!(cloned.name, tool_info.name);
        assert_eq!(cloned.description, tool_info.description);
        assert_eq!(cloned.input_schema, tool_info.input_schema);
    }

    #[test]
    fn test_mcp_tool_info_debug() {
        let tool_info = McpToolInfo {
            name: "debug_tool".to_string(),
            description: "Debuggable".to_string(),
            input_schema: serde_json::json!({}),
        };

        let debug_str = format!("{:?}", tool_info);
        assert!(debug_str.contains("debug_tool"));
        assert!(debug_str.contains("Debuggable"));
    }

    #[test]
    fn test_mcp_tool_info_vec_serialization() {
        let tools = vec![
            McpToolInfo {
                name: "tool_a".to_string(),
                description: "First tool".to_string(),
                input_schema: serde_json::json!({"type": "object"}),
            },
            McpToolInfo {
                name: "tool_b".to_string(),
                description: "Second tool".to_string(),
                input_schema: serde_json::json!({"type": "object", "properties": {"x": {"type": "number"}}}),
            },
        ];

        let json = serde_json::to_string(&tools).expect("serialize vec");
        let deserialized: Vec<McpToolInfo> = serde_json::from_str(&json).expect("deserialize vec");
        assert_eq!(deserialized.len(), 2);
        assert_eq!(deserialized[0].name, "tool_a");
        assert_eq!(deserialized[1].name, "tool_b");
    }

    #[test]
    fn test_mcp_tool_info_with_unicode_name() {
        let tool_info = McpToolInfo {
            name: "recherche_web".to_string(),
            description: "Rechercher sur le web avec des caracteres speciaux: e, a, u".to_string(),
            input_schema: serde_json::json!({"type": "object"}),
        };

        let json = serde_json::to_string(&tool_info).expect("serialize");
        let deserialized: McpToolInfo = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(deserialized.name, tool_info.name);
        assert_eq!(deserialized.description, tool_info.description);
    }
}
