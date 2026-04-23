use super::*;
use pretty_assertions::assert_eq;
use std::collections::HashMap;

#[test]
fn test_mcp_tool_info_fields() {
    let info = McpToolInfo {
        name: "test".to_string(),
        description: "A test tool".to_string(),
        input_schema: serde_json::json!({"type": "object"}),
    };

    assert_eq!(info.name, "test");
    assert_eq!(info.description, "A test tool");
    assert_eq!(info.input_schema["type"], "object");
}

#[test]
fn test_mcp_tool_info_serialize_deserialize() {
    let info = McpToolInfo {
        name: "fetch".to_string(),
        description: "Fetch a URL".to_string(),
        input_schema: serde_json::json!({
            "type": "object",
            "properties": {
                "url": {"type": "string", "format": "uri"},
                "timeout": {"type": "integer", "minimum": 0}
            },
            "required": ["url"]
        }),
    };

    let serialized = serde_json::to_value(&info).expect("serialize to value");
    assert_eq!(serialized["name"], "fetch");
    assert_eq!(serialized["description"], "Fetch a URL");
    assert_eq!(serialized["input_schema"]["type"], "object");
    assert_eq!(
        serialized["input_schema"]["properties"]["url"]["format"],
        "uri"
    );
    assert_eq!(serialized["input_schema"]["required"][0], "url");

    let deserialized: McpToolInfo =
        serde_json::from_value(serialized).expect("deserialize from value");
    assert_eq!(deserialized.name, info.name);
    assert_eq!(deserialized.description, info.description);
    assert_eq!(deserialized.input_schema, info.input_schema);
}

#[test]
fn test_mcp_tool_info_clone_independence() {
    let original = McpToolInfo {
        name: "original".to_string(),
        description: "Original".to_string(),
        input_schema: serde_json::json!({"type": "object"}),
    };

    let mut cloned = original.clone();
    cloned.name = "cloned".to_string();
    cloned.description = "Cloned".to_string();

    // Original should be unaffected
    assert_eq!(original.name, "original");
    assert_eq!(original.description, "Original");
    assert_eq!(cloned.name, "cloned");
    assert_eq!(cloned.description, "Cloned");
}

#[tokio::test]
async fn test_connect_stdio_with_nonexistent_binary() {
    let result = McpConnection::connect_stdio(
        "test_server",
        "/nonexistent/binary/path",
        &[],
        &HashMap::new(),
    )
    .await;

    assert!(result.is_err());
    match result {
        Err(e) => {
            let err_msg = e.to_string();
            assert!(
                err_msg.contains("failed to spawn MCP server"),
                "Expected spawn error, got: {}",
                err_msg
            );
        }
        Ok(_) => panic!("Expected error"),
    }
}

#[tokio::test]
async fn test_connect_with_stdio_transport_nonexistent() {
    let transport = bridge_core::mcp::McpTransport::Stdio {
        command: "/nonexistent/binary".to_string(),
        args: vec![],
        env: HashMap::new(),
    };

    let result = McpConnection::connect("test_server", &transport).await;
    assert!(result.is_err());
}

#[tokio::test]
async fn test_connect_http_with_invalid_url() {
    let result =
        McpConnection::connect_http("test_server", "http://localhost:1", &HashMap::new()).await;

    // Connection to a non-listening port should fail
    assert!(result.is_err());
}

#[tokio::test]
async fn test_connect_http_with_invalid_header_name() {
    let mut headers = HashMap::new();
    headers.insert("invalid header\n".to_string(), "value".to_string());

    let result =
        McpConnection::connect_http("test_server", "http://localhost:9999", &headers).await;

    assert!(result.is_err());
    match result {
        Err(e) => {
            let err_msg = e.to_string();
            assert!(
                err_msg.contains("invalid header name"),
                "Expected header name error, got: {}",
                err_msg
            );
        }
        Ok(_) => panic!("Expected error"),
    }
}
