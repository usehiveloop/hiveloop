use pretty_assertions::assert_eq;

use crate::agent::AgentDefinition;
use crate::mcp::McpTransport;
use crate::provider::ProviderType;

#[test]
fn agent_definition_deserialize_from_realistic_json() {
    let json = r#"{
        "id": "prod-agent-001",
        "name": "Production Agent",
        "system_prompt": "You are a production-grade assistant.",
        "provider": {
            "provider_type": "anthropic",
            "model": "claude-sonnet-4-20250514",
            "api_key": "<anthropic-api-key>",
            "base_url": "https://api.anthropic.com/v1"
        },
        "tools": [
            {
                "name": "database_query",
                "description": "Runs a read-only SQL query",
                "parameters_schema": {
                    "type": "object",
                    "properties": {
                        "sql": { "type": "string" }
                    },
                    "required": ["sql"]
                }
            }
        ],
        "mcp_servers": [
            {
                "name": "code-server",
                "transport": {
                    "type": "streamable_http",
                    "url": "https://mcp.internal.com/sse",
                    "headers": {
                        "Authorization": "Bearer internal-token"
                    }
                }
            }
        ],
        "skills": [
            {
                "id": "code-review",
                "title": "Code Review",
                "description": "Reviews pull requests",
                "content": "You are a code review expert."
            }
        ],
        "config": {
            "max_tokens": 8192,
            "temperature": 0.3,
            "rate_limit_rpm": 100
        },
        "subagents": [],
        "webhook_url": "https://hooks.prod.com/bridge",
        "webhook_secret": "<webhook-secret>",
        "version": "2.1.0",
        "updated_at": "2026-03-01T12:00:00Z"
    }"#;

    let agent: AgentDefinition = serde_json::from_str(json).expect("deserialize");
    assert_eq!(agent.id, "prod-agent-001");
    assert_eq!(agent.provider.provider_type, ProviderType::Anthropic);
    assert_eq!(agent.tools.len(), 1);
    assert_eq!(agent.tools[0].name, "database_query");
    assert_eq!(agent.mcp_servers.len(), 1);
    if let McpTransport::StreamableHttp { url, headers } = &agent.mcp_servers[0].transport {
        assert_eq!(url, "https://mcp.internal.com/sse");
        assert_eq!(
            headers.get("Authorization").unwrap(),
            "Bearer internal-token"
        );
    } else {
        panic!("Expected StreamableHttp transport");
    }
    assert_eq!(agent.skills.len(), 1);
    assert_eq!(agent.config.max_tokens, Some(8192));
    assert_eq!(agent.config.temperature, Some(0.3));
    assert!(agent.config.max_turns.is_none());
    assert!(agent.config.json_schema.is_none());
    assert_eq!(agent.config.rate_limit_rpm, Some(100));
    assert_eq!(
        agent.webhook_url,
        Some("https://hooks.prod.com/bridge".to_string())
    );
    assert_eq!(agent.version, Some("2.1.0".to_string()));

    // Roundtrip
    let json2 = serde_json::to_string_pretty(&agent).expect("re-serialize");
    let agent2: AgentDefinition = serde_json::from_str(&json2).expect("re-deserialize");
    assert_eq!(agent, agent2);
}

