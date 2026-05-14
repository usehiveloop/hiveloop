use pretty_assertions::assert_eq;
use std::collections::HashMap;

use crate::mcp::{McpServerDefinition, McpTransport};
use crate::skill::SkillDefinition;

#[test]
fn mcp_transport_stdio_roundtrip() {
    let transport = McpTransport::Stdio {
        command: "node".to_string(),
        args: vec![
            "server.js".to_string(),
            "--port".to_string(),
            "3000".to_string(),
        ],
        env: HashMap::from([
            ("NODE_ENV".to_string(), "production".to_string()),
            ("PORT".to_string(), "3000".to_string()),
        ]),
    };

    let json = serde_json::to_string_pretty(&transport).expect("serialize Stdio");
    let deserialized: McpTransport = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(transport, deserialized);

    // Verify the tagged format includes "type": "stdio"
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["type"], "stdio");
}

#[test]
fn mcp_transport_stdio_defaults_for_optional_fields() {
    let json = r#"{"type": "stdio", "command": "mcp-server"}"#;
    let transport: McpTransport = serde_json::from_str(json).expect("deserialize");
    if let McpTransport::Stdio { command, args, env } = &transport {
        assert_eq!(command, "mcp-server");
        assert!(args.is_empty());
        assert!(env.is_empty());
    } else {
        panic!("Expected Stdio variant");
    }
}

#[test]
fn mcp_transport_streamable_http_roundtrip() {
    let transport = McpTransport::StreamableHttp {
        url: "https://mcp.example.com/sse".to_string(),
        headers: HashMap::from([
            (
                "Authorization".to_string(),
                "Bearer <bearer-token>".to_string(),
            ),
            ("X-Custom".to_string(), "value".to_string()),
        ]),
    };

    let json = serde_json::to_string_pretty(&transport).expect("serialize StreamableHttp");
    let deserialized: McpTransport = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(transport, deserialized);

    // Verify the tagged format includes "type": "streamable_http"
    let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
    assert_eq!(value["type"], "streamable_http");
}

#[test]
fn mcp_transport_streamable_http_defaults_for_optional_fields() {
    let json = r#"{"type": "streamable_http", "url": "https://example.com"}"#;
    let transport: McpTransport = serde_json::from_str(json).expect("deserialize");
    if let McpTransport::StreamableHttp { url, headers } = &transport {
        assert_eq!(url, "https://example.com");
        assert!(headers.is_empty());
    } else {
        panic!("Expected StreamableHttp variant");
    }
}

// ──────────────────────────────────────────────
// McpServerDefinition
// ──────────────────────────────────────────────

#[test]
fn mcp_server_definition_roundtrip() {
    let server = McpServerDefinition {
        name: "my-server".to_string(),
        transport: McpTransport::Stdio {
            command: "mcp-server".to_string(),
            args: vec![],
            env: HashMap::new(),
        },
    };

    let json = serde_json::to_string_pretty(&server).expect("serialize");
    let deserialized: McpServerDefinition = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(server, deserialized);
}

// ──────────────────────────────────────────────
// SkillDefinition
// ──────────────────────────────────────────────

#[test]
fn skill_definition_roundtrip() {
    let skill = SkillDefinition {
        id: "skill-42".to_string(),
        title: "Data Analysis".to_string(),
        description: "Analyzes datasets and produces insights.".to_string(),
        content: "You are a data analysis expert. Analyze datasets and produce insights."
            .to_string(),
        ..Default::default()
    };

    let json = serde_json::to_string_pretty(&skill).expect("serialize");
    let deserialized: SkillDefinition = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(skill, deserialized);
}

// ──────────────────────────────────────────────
// Role
// ──────────────────────────────────────────────
