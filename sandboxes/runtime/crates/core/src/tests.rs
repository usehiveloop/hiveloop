#[cfg(test)]
mod serde_roundtrip_tests {
    use pretty_assertions::assert_eq;
    use std::collections::HashMap;

    use crate::agent::{AgentConfig, AgentDefinition, AgentSummary};
    use crate::config::{LogFormat, RuntimeConfig};
    use crate::conversation::{ContentBlock, Message, Role, ToolCall, ToolResult};
    use crate::error::BridgeError;
    use crate::integration::{IntegrationAction, IntegrationDefinition};
    use crate::mcp::{McpServerDefinition, McpTransport};
    use crate::metrics::{AgentMetrics, GlobalMetrics, MetricsResponse, MetricsSnapshot};
    use crate::permission::ToolPermission;
    use crate::provider::{ProviderConfig, ProviderType};
    use crate::skill::SkillDefinition;
    use crate::tool::ToolDefinition;
    use crate::webhook::{WebhookEventType, WebhookPayload};

    // ──────────────────────────────────────────────
    // AgentDefinition
    // ──────────────────────────────────────────────

    #[test]
    fn agent_definition_roundtrip_all_fields_present() {
        let agent = AgentDefinition {
            id: "agent-001".to_string(),
            name: "Test Agent".to_string(),
            description: Some("A test agent for roundtrip testing".to_string()),
            system_prompt: "You are a helpful assistant.".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::Anthropic,
                model: "claude-sonnet-4-20250514".to_string(),
                api_key: "<provider-api-key>-key".to_string(),
                base_url: Some("https://api.anthropic.com".to_string()),
            },
            tools: vec![ToolDefinition {
                name: "calculator".to_string(),
                description: "Performs arithmetic".to_string(),
                parameters_schema: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "expression": { "type": "string" }
                    }
                }),
            }],
            mcp_servers: vec![McpServerDefinition {
                name: "filesystem".to_string(),
                transport: McpTransport::Stdio {
                    command: "mcp-fs".to_string(),
                    args: vec!["--root".to_string(), "/tmp".to_string()],
                    env: HashMap::from([("HOME".to_string(), "/root".to_string())]),
                },
            }],
            skills: vec![SkillDefinition {
                id: "skill-1".to_string(),
                title: "Code Review".to_string(),
                description: "Reviews code for quality".to_string(),
                content: "You are a code review expert.".to_string(),
                ..Default::default()
            }],
            integrations: vec![],
            config: AgentConfig {
                max_tokens: Some(4096),
                max_turns: Some(10),
                temperature: Some(0.7),
                json_schema: Some(serde_json::json!({"type": "object"})),
                rate_limit_rpm: Some(60),
                compaction: None,
                max_tasks_per_conversation: None,
                max_concurrent_conversations: None,
                tool_calls_only: None,
            },
            subagents: vec![AgentDefinition {
                id: "sub-agent-001".to_string(),
                name: "Sub Agent".to_string(),
                description: Some("A sub agent for testing".to_string()),
                system_prompt: "Sub agent prompt".to_string(),
                provider: ProviderConfig {
                    provider_type: ProviderType::OpenAI,
                    model: "gpt-4o".to_string(),
                    api_key: "sk-openai-key".to_string(),
                    base_url: None,
                },
                tools: vec![],
                mcp_servers: vec![],
                skills: vec![],
                config: AgentConfig::default(),
                subagents: vec![],
                integrations: vec![],
                permissions: HashMap::new(),
                webhook_url: None,
                webhook_secret: None,
                version: None,
                updated_at: None,
            }],
            permissions: HashMap::new(),
            webhook_url: Some("https://example.com/webhook".to_string()),
            webhook_secret: Some("<webhook-secret>".to_string()),
            version: Some("1.0.0".to_string()),
            updated_at: Some("2026-03-02T00:00:00Z".to_string()),
        };

        let json = serde_json::to_string_pretty(&agent).expect("serialize AgentDefinition");
        let deserialized: AgentDefinition =
            serde_json::from_str(&json).expect("deserialize AgentDefinition");
        assert_eq!(agent, deserialized);
    }

    #[test]
    fn agent_definition_roundtrip_optional_fields_absent() {
        let json = r#"{
            "id": "agent-002",
            "name": "Minimal Agent",
            "system_prompt": "Be helpful.",
            "provider": {
                "provider_type": "open_ai",
                "model": "gpt-4o",
                "api_key": "<provider-api-key>"
            }
        }"#;

        let agent: AgentDefinition =
            serde_json::from_str(json).expect("deserialize minimal AgentDefinition");
        assert_eq!(agent.id, "agent-002");
        assert_eq!(agent.name, "Minimal Agent");
        assert!(agent.tools.is_empty());
        assert!(agent.mcp_servers.is_empty());
        assert!(agent.skills.is_empty());
        assert!(agent.subagents.is_empty());
        assert!(agent.webhook_url.is_none());
        assert!(agent.webhook_secret.is_none());
        assert!(agent.version.is_none());
        assert!(agent.updated_at.is_none());
        assert_eq!(agent.config, AgentConfig::default());

        // Re-serialize and deserialize to confirm roundtrip
        let json2 = serde_json::to_string_pretty(&agent).expect("re-serialize");
        let agent2: AgentDefinition = serde_json::from_str(&json2).expect("re-deserialize");
        assert_eq!(agent, agent2);
    }

    #[test]
    fn agent_definition_skip_serializing_none_optional_fields() {
        let agent = AgentDefinition {
            id: "agent-003".to_string(),
            name: "No Optionals".to_string(),
            description: None,
            system_prompt: "Prompt".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::Google,
                model: "gemini-pro".to_string(),
                api_key: "key".to_string(),
                base_url: None,
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: AgentConfig::default(),
            subagents: vec![],
            permissions: HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        };

        let json = serde_json::to_string(&agent).expect("serialize");
        // None fields should be skipped
        assert!(!json.contains("description"));
        assert!(!json.contains("webhook_url"));
        assert!(!json.contains("webhook_secret"));
        assert!(!json.contains("version"));
        assert!(!json.contains("updated_at"));
    }

    // ──────────────────────────────────────────────
    // AgentConfig
    // ──────────────────────────────────────────────

    #[test]
    fn agent_config_default_is_all_none() {
        let config = AgentConfig::default();
        assert!(config.max_tokens.is_none());
        assert!(config.max_turns.is_none());
        assert!(config.temperature.is_none());
        assert!(config.json_schema.is_none());
        assert!(config.rate_limit_rpm.is_none());
    }

    #[test]
    fn agent_config_roundtrip_with_all_fields() {
        let config = AgentConfig {
            max_tokens: Some(2048),
            max_turns: Some(5),
            temperature: Some(0.9),
            json_schema: Some(serde_json::json!({"type": "string"})),
            rate_limit_rpm: Some(120),
            compaction: None,
            max_tasks_per_conversation: Some(100),
            max_concurrent_conversations: Some(50),
            tool_calls_only: None,
        };

        let json = serde_json::to_string_pretty(&config).expect("serialize AgentConfig");
        let deserialized: AgentConfig = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(config, deserialized);
    }

    #[test]
    fn agent_config_skip_serializing_none_fields() {
        let config = AgentConfig::default();
        let json = serde_json::to_string(&config).expect("serialize default AgentConfig");
        assert_eq!(json, "{}");
    }

    // ──────────────────────────────────────────────
    // AgentSummary
    // ──────────────────────────────────────────────

    #[test]
    fn agent_summary_roundtrip_with_version() {
        let summary = AgentSummary {
            id: "agent-1".to_string(),
            name: "Agent One".to_string(),
            version: Some("2.0.0".to_string()),
        };

        let json = serde_json::to_string_pretty(&summary).expect("serialize");
        let deserialized: AgentSummary = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(summary, deserialized);
    }

    #[test]
    fn agent_summary_roundtrip_without_version() {
        let json = r#"{"id": "agent-2", "name": "Agent Two"}"#;
        let summary: AgentSummary = serde_json::from_str(json).expect("deserialize");
        assert_eq!(summary.id, "agent-2");
        assert!(summary.version.is_none());

        let json2 = serde_json::to_string_pretty(&summary).expect("re-serialize");
        let summary2: AgentSummary = serde_json::from_str(&json2).expect("re-deserialize");
        assert_eq!(summary, summary2);
    }

    // ──────────────────────────────────────────────
    // ProviderType
    // ──────────────────────────────────────────────

    #[test]
    fn provider_type_all_variants_serialize_to_snake_case() {
        let cases = vec![
            (ProviderType::OpenAI, "\"open_ai\""),
            (ProviderType::Anthropic, "\"anthropic\""),
            (ProviderType::Google, "\"google\""),
            (ProviderType::Groq, "\"groq\""),
            (ProviderType::DeepSeek, "\"deep_seek\""),
            (ProviderType::Mistral, "\"mistral\""),
            (ProviderType::Cohere, "\"cohere\""),
            (ProviderType::XAi, "\"x_ai\""),
            (ProviderType::Together, "\"together\""),
            (ProviderType::Fireworks, "\"fireworks\""),
            (ProviderType::Ollama, "\"ollama\""),
            (ProviderType::Custom, "\"custom\""),
        ];

        for (variant, expected_json) in cases {
            let json = serde_json::to_string(&variant).expect("serialize ProviderType");
            assert_eq!(
                json, expected_json,
                "ProviderType::{:?} serialization",
                variant
            );
        }
    }

    #[test]
    fn provider_type_all_variants_roundtrip() {
        let variants = vec![
            ProviderType::OpenAI,
            ProviderType::Anthropic,
            ProviderType::Google,
            ProviderType::Groq,
            ProviderType::DeepSeek,
            ProviderType::Mistral,
            ProviderType::Cohere,
            ProviderType::XAi,
            ProviderType::Together,
            ProviderType::Fireworks,
            ProviderType::Ollama,
            ProviderType::Custom,
        ];

        for variant in variants {
            let json = serde_json::to_string(&variant).expect("serialize");
            let deserialized: ProviderType = serde_json::from_str(&json).expect("deserialize");
            assert_eq!(
                variant, deserialized,
                "ProviderType roundtrip for {:?}",
                variant
            );
        }
    }

    #[test]
    fn provider_type_display_matches_serde() {
        let variants = vec![
            ProviderType::OpenAI,
            ProviderType::Anthropic,
            ProviderType::Google,
            ProviderType::Groq,
            ProviderType::DeepSeek,
            ProviderType::Mistral,
            ProviderType::Cohere,
            ProviderType::XAi,
            ProviderType::Together,
            ProviderType::Fireworks,
            ProviderType::Ollama,
            ProviderType::Custom,
        ];

        for variant in variants {
            let display = format!("{}", variant);
            // Display output should be the same as the JSON value without quotes
            let json = serde_json::to_string(&variant).expect("serialize");
            let json_unquoted = json.trim_matches('"');
            assert_eq!(
                display, json_unquoted,
                "Display and serde should match for {:?}",
                variant
            );
        }
    }

    #[test]
    fn provider_type_from_str_accepts_aliases() {
        use std::str::FromStr;

        assert_eq!(
            ProviderType::from_str("openai").unwrap(),
            ProviderType::OpenAI
        );
        assert_eq!(
            ProviderType::from_str("open_ai").unwrap(),
            ProviderType::OpenAI
        );
        assert_eq!(
            ProviderType::from_str("deepseek").unwrap(),
            ProviderType::DeepSeek
        );
        assert_eq!(
            ProviderType::from_str("deep_seek").unwrap(),
            ProviderType::DeepSeek
        );
        assert_eq!(ProviderType::from_str("xai").unwrap(), ProviderType::XAi);
        assert_eq!(ProviderType::from_str("x_ai").unwrap(), ProviderType::XAi);
    }

    #[test]
    fn provider_type_from_str_rejects_unknown() {
        use std::str::FromStr;
        assert!(ProviderType::from_str("nonexistent").is_err());
    }

    // ──────────────────────────────────────────────
    // ProviderConfig
    // ──────────────────────────────────────────────

    #[test]
    fn provider_config_roundtrip_with_base_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::Custom,
            model: "custom-model".to_string(),
            api_key: "custom-key".to_string(),
            base_url: Some("https://custom.api.com/v1".to_string()),
        };

        let json = serde_json::to_string_pretty(&config).expect("serialize");
        let deserialized: ProviderConfig = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(config, deserialized);
    }

    #[test]
    fn provider_config_roundtrip_without_base_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::Anthropic,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: "<anthropic-api-key>".to_string(),
            base_url: None,
        };

        let json = serde_json::to_string_pretty(&config).expect("serialize");
        assert!(!json.contains("base_url"));
        let deserialized: ProviderConfig = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(config, deserialized);
    }

    // ──────────────────────────────────────────────
    // McpTransport
    // ──────────────────────────────────────────────

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
                ("Authorization".to_string(), "Bearer <bridge-control-plane-api-key>".to_string()),
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

    #[test]
    fn role_all_variants_roundtrip() {
        let roles = [Role::User, Role::Assistant, Role::System, Role::Tool];

        let expected_json = ["\"user\"", "\"assistant\"", "\"system\"", "\"tool\""];

        for (role, expected) in roles.iter().zip(expected_json.iter()) {
            let json = serde_json::to_string(role).expect("serialize Role");
            assert_eq!(&json, expected, "Role::{:?} serialization", role);

            let deserialized: Role = serde_json::from_str(&json).expect("deserialize");
            assert_eq!(role, &deserialized);
        }
    }

    // ──────────────────────────────────────────────
    // ContentBlock
    // ──────────────────────────────────────────────

    #[test]
    fn content_block_text_roundtrip() {
        let block = ContentBlock::Text {
            text: "Hello, world!".to_string(),
        };

        let json = serde_json::to_string_pretty(&block).expect("serialize");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
        assert_eq!(value["type"], "text");
        assert_eq!(value["text"], "Hello, world!");

        let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(block, deserialized);
    }

    #[test]
    fn content_block_tool_call_roundtrip() {
        let block = ContentBlock::ToolCall(ToolCall {
            id: "call-123".to_string(),
            name: "calculator".to_string(),
            arguments: serde_json::json!({"expression": "2 + 2"}),
        });

        let json = serde_json::to_string_pretty(&block).expect("serialize");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
        assert_eq!(value["type"], "tool_call");
        assert_eq!(value["id"], "call-123");
        assert_eq!(value["name"], "calculator");

        let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(block, deserialized);
    }

    #[test]
    fn content_block_tool_result_roundtrip() {
        let block = ContentBlock::ToolResult(ToolResult {
            tool_call_id: "call-123".to_string(),
            content: "4".to_string(),
            is_error: false,
        });

        let json = serde_json::to_string_pretty(&block).expect("serialize");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
        assert_eq!(value["type"], "tool_result");
        assert_eq!(value["tool_call_id"], "call-123");
        assert_eq!(value["content"], "4");

        let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(block, deserialized);
    }

    #[test]
    fn content_block_tool_result_with_error_roundtrip() {
        let block = ContentBlock::ToolResult(ToolResult {
            tool_call_id: "call-456".to_string(),
            content: "Division by zero".to_string(),
            is_error: true,
        });

        let json = serde_json::to_string_pretty(&block).expect("serialize");
        let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(block, deserialized);

        if let ContentBlock::ToolResult(result) = &deserialized {
            assert!(result.is_error);
        } else {
            panic!("Expected ToolResult variant");
        }
    }

    #[test]
    fn content_block_tool_result_is_error_defaults_to_false() {
        let json = r#"{"type": "tool_result", "tool_call_id": "call-789", "content": "ok"}"#;
        let block: ContentBlock = serde_json::from_str(json).expect("deserialize");
        if let ContentBlock::ToolResult(result) = &block {
            assert!(!result.is_error);
        } else {
            panic!("Expected ToolResult variant");
        }
    }

    #[test]
    fn content_block_image_roundtrip() {
        let block = ContentBlock::Image {
            media_type: "image/png".to_string(),
            data: "iVBORw0KGgoAAAANSUhEUg==".to_string(),
        };

        let json = serde_json::to_string_pretty(&block).expect("serialize");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
        assert_eq!(value["type"], "image");
        assert_eq!(value["media_type"], "image/png");

        let deserialized: ContentBlock = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(block, deserialized);
    }

    // ──────────────────────────────────────────────
    // ToolCall & ToolResult (standalone)
    // ──────────────────────────────────────────────

    #[test]
    fn tool_call_roundtrip() {
        let call = ToolCall {
            id: "tc-1".to_string(),
            name: "web_search".to_string(),
            arguments: serde_json::json!({"query": "Rust serde", "limit": 10}),
        };

        let json = serde_json::to_string_pretty(&call).expect("serialize");
        let deserialized: ToolCall = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(call, deserialized);
    }

    #[test]
    fn tool_result_roundtrip() {
        let result = ToolResult {
            tool_call_id: "tc-1".to_string(),
            content: "Found 42 results".to_string(),
            is_error: false,
        };

        let json = serde_json::to_string_pretty(&result).expect("serialize");
        let deserialized: ToolResult = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(result, deserialized);
    }

    // ──────────────────────────────────────────────
    // Message
    // ──────────────────────────────────────────────

    #[test]
    fn message_with_text_content_roundtrip() {
        let msg = Message {
            role: Role::User,
            content: vec![ContentBlock::Text {
                text: "What is Rust?".to_string(),
            }],
            timestamp: chrono::Utc::now(),
        };

        let json = serde_json::to_string_pretty(&msg).expect("serialize Message");
        let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, deserialized);
    }

    #[test]
    fn message_with_tool_call_content_roundtrip() {
        let msg = Message {
            role: Role::Assistant,
            content: vec![ContentBlock::ToolCall(ToolCall {
                id: "tc-100".to_string(),
                name: "read_file".to_string(),
                arguments: serde_json::json!({"path": "/etc/hosts"}),
            })],
            timestamp: chrono::Utc::now(),
        };

        let json = serde_json::to_string_pretty(&msg).expect("serialize");
        let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, deserialized);
    }

    #[test]
    fn message_with_tool_result_content_roundtrip() {
        let msg = Message {
            role: Role::Tool,
            content: vec![ContentBlock::ToolResult(ToolResult {
                tool_call_id: "tc-100".to_string(),
                content: "127.0.0.1 localhost".to_string(),
                is_error: false,
            })],
            timestamp: chrono::Utc::now(),
        };

        let json = serde_json::to_string_pretty(&msg).expect("serialize");
        let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, deserialized);
    }

    #[test]
    fn message_with_image_content_roundtrip() {
        let msg = Message {
            role: Role::User,
            content: vec![ContentBlock::Image {
                media_type: "image/jpeg".to_string(),
                data: "/9j/4AAQSkZJRg==".to_string(),
            }],
            timestamp: chrono::Utc::now(),
        };

        let json = serde_json::to_string_pretty(&msg).expect("serialize");
        let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, deserialized);
    }

    #[test]
    fn message_with_multiple_content_blocks_roundtrip() {
        let msg = Message {
            role: Role::Assistant,
            content: vec![
                ContentBlock::Text {
                    text: "Let me check that file.".to_string(),
                },
                ContentBlock::ToolCall(ToolCall {
                    id: "tc-200".to_string(),
                    name: "read_file".to_string(),
                    arguments: serde_json::json!({"path": "/tmp/test.txt"}),
                }),
            ],
            timestamp: chrono::Utc::now(),
        };

        let json = serde_json::to_string_pretty(&msg).expect("serialize");
        let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, deserialized);
        assert_eq!(deserialized.content.len(), 2);
    }

    #[test]
    fn message_with_empty_content_roundtrip() {
        let msg = Message {
            role: Role::System,
            content: vec![],
            timestamp: chrono::Utc::now(),
        };

        let json = serde_json::to_string_pretty(&msg).expect("serialize");
        let deserialized: Message = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(msg, deserialized);
        assert!(deserialized.content.is_empty());
    }

    // ──────────────────────────────────────────────
    // ToolDefinition
    // ──────────────────────────────────────────────

    #[test]
    fn tool_definition_roundtrip() {
        let tool = ToolDefinition {
            name: "web_search".to_string(),
            description: "Searches the web".to_string(),
            parameters_schema: serde_json::json!({
                "type": "object",
                "properties": {
                    "query": { "type": "string", "description": "Search query" },
                    "limit": { "type": "integer", "default": 10 }
                },
                "required": ["query"]
            }),
        };

        let json = serde_json::to_string_pretty(&tool).expect("serialize ToolDefinition");
        let deserialized: ToolDefinition = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(tool, deserialized);
    }

    #[test]
    fn tool_definition_with_empty_schema_roundtrip() {
        let tool = ToolDefinition {
            name: "noop".to_string(),
            description: "Does nothing".to_string(),
            parameters_schema: serde_json::json!({}),
        };

        let json = serde_json::to_string_pretty(&tool).expect("serialize");
        let deserialized: ToolDefinition = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(tool, deserialized);
    }

    // ──────────────────────────────────────────────
    // MetricsSnapshot
    // ──────────────────────────────────────────────

    #[test]
    fn metrics_snapshot_serializes_correctly() {
        let snapshot = MetricsSnapshot {
            agent_id: "agent-1".to_string(),
            agent_name: "Test Agent".to_string(),
            input_tokens: 500,
            output_tokens: 200,
            total_tokens: 700,
            total_requests: 10,
            failed_requests: 1,
            active_conversations: 3,
            total_conversations: 15,
            tool_calls: 25,
            avg_latency_ms: 150.5,
            tool_call_details: vec![],
        };

        let json = serde_json::to_string_pretty(&snapshot).expect("serialize MetricsSnapshot");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");

        assert_eq!(value["agent_id"], "agent-1");
        assert_eq!(value["agent_name"], "Test Agent");
        assert_eq!(value["input_tokens"], 500);
        assert_eq!(value["output_tokens"], 200);
        assert_eq!(value["total_tokens"], 700);
        assert_eq!(value["total_requests"], 10);
        assert_eq!(value["failed_requests"], 1);
        assert_eq!(value["active_conversations"], 3);
        assert_eq!(value["total_conversations"], 15);
        assert_eq!(value["tool_calls"], 25);
        assert_eq!(value["avg_latency_ms"], 150.5);
    }

    #[test]
    fn metrics_snapshot_total_tokens_equals_input_plus_output() {
        let metrics = AgentMetrics::new();
        metrics
            .input_tokens
            .store(1234, std::sync::atomic::Ordering::Relaxed);
        metrics
            .output_tokens
            .store(5678, std::sync::atomic::Ordering::Relaxed);

        let snap = metrics.snapshot("a", "A");
        assert_eq!(snap.total_tokens, 1234 + 5678);
        assert_eq!(snap.total_tokens, snap.input_tokens + snap.output_tokens);
    }

    #[test]
    fn metrics_snapshot_avg_latency_computation() {
        let metrics = AgentMetrics::new();
        metrics
            .latency_sum_ms
            .store(3000, std::sync::atomic::Ordering::Relaxed);
        metrics
            .latency_count
            .store(12, std::sync::atomic::Ordering::Relaxed);

        let snap = metrics.snapshot("a", "A");
        assert!((snap.avg_latency_ms - 250.0).abs() < f64::EPSILON);
    }

    #[test]
    fn metrics_snapshot_avg_latency_zero_when_no_measurements() {
        let metrics = AgentMetrics::new();
        let snap = metrics.snapshot("a", "A");
        assert!((snap.avg_latency_ms - 0.0).abs() < f64::EPSILON);
    }

    // ──────────────────────────────────────────────
    // GlobalMetrics
    // ──────────────────────────────────────────────

    #[test]
    fn global_metrics_serializes_correctly() {
        let global = GlobalMetrics {
            total_agents: 5,
            total_active_conversations: 12,
            uptime_secs: 3600,
        };

        let json = serde_json::to_string_pretty(&global).expect("serialize");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
        assert_eq!(value["total_agents"], 5);
        assert_eq!(value["total_active_conversations"], 12);
        assert_eq!(value["uptime_secs"], 3600);
    }

    // ──────────────────────────────────────────────
    // MetricsResponse
    // ──────────────────────────────────────────────

    #[test]
    fn metrics_response_serializes_correctly() {
        let response = MetricsResponse {
            timestamp: chrono::Utc::now(),
            agents: vec![MetricsSnapshot {
                agent_id: "agent-1".to_string(),
                agent_name: "Agent One".to_string(),
                input_tokens: 100,
                output_tokens: 50,
                total_tokens: 150,
                total_requests: 5,
                failed_requests: 0,
                active_conversations: 1,
                total_conversations: 3,
                tool_calls: 10,
                avg_latency_ms: 200.0,
                tool_call_details: vec![],
            }],
            global: GlobalMetrics {
                total_agents: 1,
                total_active_conversations: 1,
                uptime_secs: 120,
            },
        };

        let json = serde_json::to_string_pretty(&response).expect("serialize MetricsResponse");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");
        assert!(value["timestamp"].is_string());
        assert!(value["agents"].is_array());
        assert_eq!(value["agents"].as_array().unwrap().len(), 1);
        assert!(value["global"].is_object());
    }

    // ──────────────────────────────────────────────
    // WebhookEventType
    // ──────────────────────────────────────────────

    #[test]
    fn webhook_event_type_all_variants_roundtrip() {
        let variants = vec![
            (
                WebhookEventType::ConversationCreated,
                "\"conversation_created\"",
            ),
            (WebhookEventType::MessageReceived, "\"message_received\""),
            (WebhookEventType::ResponseStarted, "\"response_started\""),
            (WebhookEventType::ResponseChunk, "\"response_chunk\""),
            (
                WebhookEventType::ResponseCompleted,
                "\"response_completed\"",
            ),
            (WebhookEventType::ToolCallStarted, "\"tool_call_started\""),
            (
                WebhookEventType::ToolCallCompleted,
                "\"tool_call_completed\"",
            ),
            (
                WebhookEventType::ConversationEnded,
                "\"conversation_ended\"",
            ),
            (WebhookEventType::AgentError, "\"agent_error\""),
        ];

        for (variant, expected_json) in variants {
            let json = serde_json::to_string(&variant).expect("serialize WebhookEventType");
            assert_eq!(
                json, expected_json,
                "WebhookEventType::{:?} serialization",
                variant
            );

            let deserialized: WebhookEventType = serde_json::from_str(&json).expect("deserialize");
            assert_eq!(variant, deserialized);
        }
    }

    // ──────────────────────────────────────────────
    // WebhookPayload
    // ──────────────────────────────────────────────

    #[test]
    fn webhook_payload_roundtrip() {
        let payload = WebhookPayload {
            event_id: "evt-1".to_string(),
            event_type: WebhookEventType::MessageReceived,
            agent_id: "agent-1".to_string(),
            conversation_id: "conv-123".to_string(),
            timestamp: chrono::Utc::now(),
            sequence_number: 0,
            data: serde_json::json!({"message": "Hello"}),
            webhook_url: "https://example.com/hook".to_string(),
            webhook_secret: "<webhook-secret>".to_string(),
        };

        let json = serde_json::to_string_pretty(&payload).expect("serialize WebhookPayload");
        let deserialized: WebhookPayload = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(payload.event_type, deserialized.event_type);
        assert_eq!(payload.agent_id, deserialized.agent_id);
        assert_eq!(payload.conversation_id, deserialized.conversation_id);
        assert_eq!(payload.timestamp, deserialized.timestamp);
        assert_eq!(payload.data, deserialized.data);
        assert_eq!(payload.webhook_url, deserialized.webhook_url);
        assert_eq!(payload.webhook_secret, deserialized.webhook_secret);
    }

    #[test]
    fn webhook_payload_json_shape() {
        let payload = WebhookPayload {
            event_id: "evt-2".to_string(),
            event_type: WebhookEventType::ToolCallStarted,
            agent_id: "agent-5".to_string(),
            conversation_id: "conv-999".to_string(),
            timestamp: chrono::Utc::now(),
            sequence_number: 0,
            data: serde_json::json!({"tool_name": "calculator", "args": {}}),
            webhook_url: "https://hooks.example.com".to_string(),
            webhook_secret: "secret".to_string(),
        };

        let json = serde_json::to_string_pretty(&payload).expect("serialize");
        let value: serde_json::Value = serde_json::from_str(&json).expect("parse as Value");

        assert_eq!(value["event_type"], "tool_call_started");
        assert_eq!(value["agent_id"], "agent-5");
        assert_eq!(value["conversation_id"], "conv-999");
        assert!(value["timestamp"].is_string());
        assert!(value["data"].is_object());
        assert_eq!(value["webhook_url"], "https://hooks.example.com");
        assert_eq!(value["webhook_secret"], "secret");
    }

    // ──────────────────────────────────────────────
    // RuntimeConfig
    // ──────────────────────────────────────────────

    #[test]
    fn runtime_config_default_impl() {
        let config = RuntimeConfig::default();
        assert_eq!(config.control_plane_url, "");
        assert_eq!(config.control_plane_api_key, "");
        assert_eq!(config.listen_addr, "0.0.0.0:8080");
        assert_eq!(config.drain_timeout_secs, 60);
        assert!(config.max_concurrent_conversations.is_none());
        assert_eq!(config.log_level, "info");
        assert_eq!(config.log_format, LogFormat::Text);
    }

    #[test]
    fn runtime_config_roundtrip_all_fields() {
        let config = RuntimeConfig {
            control_plane_url: "https://api.example.com".to_string(),
            control_plane_api_key: "cpk-test-key".to_string(),
            listen_addr: "127.0.0.1:9090".to_string(),
            drain_timeout_secs: 120,
            max_concurrent_conversations: Some(100),
            log_level: "debug".to_string(),
            log_format: LogFormat::Json,
            lsp: None,
            webhook_url: None,
            max_concurrent_llm_calls: Some(500),
            webhook_config: None,
            websocket_enabled: false,
            codedb_enabled: false,
            codedb_binary: "codedb".to_string(),
            otel_endpoint: None,
            otel_service_name: "bridge".to_string(),
        };

        let json = serde_json::to_string_pretty(&config).expect("serialize RuntimeConfig");
        let deserialized: RuntimeConfig = serde_json::from_str(&json).expect("deserialize");

        assert_eq!(config.control_plane_url, deserialized.control_plane_url);
        assert_eq!(
            config.control_plane_api_key,
            deserialized.control_plane_api_key
        );
        assert_eq!(config.listen_addr, deserialized.listen_addr);
        assert_eq!(config.drain_timeout_secs, deserialized.drain_timeout_secs);
        assert_eq!(
            config.max_concurrent_conversations,
            deserialized.max_concurrent_conversations
        );
        assert_eq!(config.log_level, deserialized.log_level);
        assert_eq!(config.log_format, deserialized.log_format);
    }

    #[test]
    fn runtime_config_deserialize_with_missing_optional_fields() {
        let json = r#"{
            "control_plane_url": "https://api.example.com",
            "control_plane_api_key": "key",
            "listen_addr": "0.0.0.0:8080",
            "drain_timeout_secs": 60,
            "log_level": "info",
            "log_format": "text"
        }"#;

        let config: RuntimeConfig = serde_json::from_str(json).expect("deserialize");
        assert!(config.max_concurrent_conversations.is_none());
    }

    #[test]
    fn runtime_config_skip_serializing_none_max_concurrent() {
        let config = RuntimeConfig {
            max_concurrent_conversations: None,
            ..RuntimeConfig::default()
        };

        let json = serde_json::to_string(&config).expect("serialize");
        assert!(!json.contains("max_concurrent_conversations"));
    }

    // ──────────────────────────────────────────────
    // LogFormat
    // ──────────────────────────────────────────────

    #[test]
    fn log_format_roundtrip() {
        let text_json = serde_json::to_string(&LogFormat::Text).expect("serialize");
        assert_eq!(text_json, "\"text\"");
        let text: LogFormat = serde_json::from_str(&text_json).expect("deserialize");
        assert_eq!(text, LogFormat::Text);

        let json_json = serde_json::to_string(&LogFormat::Json).expect("serialize");
        assert_eq!(json_json, "\"json\"");
        let json_val: LogFormat = serde_json::from_str(&json_json).expect("deserialize");
        assert_eq!(json_val, LogFormat::Json);
    }

    // ──────────────────────────────────────────────
    // BridgeError Display
    // ──────────────────────────────────────────────

    #[test]
    fn bridge_error_display_for_each_variant() {
        assert_eq!(
            BridgeError::AgentNotFound("a1".into()).to_string(),
            "agent not found: a1"
        );
        assert_eq!(
            BridgeError::ConversationNotFound("c1".into()).to_string(),
            "conversation not found: c1"
        );
        assert_eq!(
            BridgeError::ConversationEnded("c1".into()).to_string(),
            "conversation ended: c1"
        );
        assert_eq!(
            BridgeError::InvalidRequest("bad".into()).to_string(),
            "invalid request: bad"
        );
        assert_eq!(
            BridgeError::ProviderError("fail".into()).to_string(),
            "provider error: fail"
        );
        assert_eq!(
            BridgeError::McpError("fail".into()).to_string(),
            "mcp error: fail"
        );
        assert_eq!(
            BridgeError::ToolError("fail".into()).to_string(),
            "tool error: fail"
        );
        assert_eq!(
            BridgeError::ConfigError("fail".into()).to_string(),
            "config error: fail"
        );
        assert_eq!(
            BridgeError::WebhookError("fail".into()).to_string(),
            "webhook error: fail"
        );
        assert_eq!(
            BridgeError::Internal("fail".into()).to_string(),
            "internal error: fail"
        );
        assert_eq!(BridgeError::RateLimited.to_string(), "rate limited");
        assert_eq!(
            BridgeError::Unauthorized("bad token".into()).to_string(),
            "unauthorized: bad token"
        );
        assert_eq!(
            BridgeError::Conflict("active conversations".into()).to_string(),
            "conflict: active conversations"
        );
    }

    // ──────────────────────────────────────────────
    // BridgeError IntoResponse status codes
    // ──────────────────────────────────────────────

    #[test]
    fn bridge_error_into_response_agent_not_found_is_404() {
        use axum::response::IntoResponse;

        let err = BridgeError::AgentNotFound("x".into());
        let response = err.into_response();
        assert_eq!(response.status(), axum::http::StatusCode::NOT_FOUND);
    }

    #[test]
    fn bridge_error_into_response_conversation_not_found_is_404() {
        use axum::response::IntoResponse;

        let err = BridgeError::ConversationNotFound("x".into());
        let response = err.into_response();
        assert_eq!(response.status(), axum::http::StatusCode::NOT_FOUND);
    }

    #[test]
    fn bridge_error_into_response_conversation_ended_is_400() {
        use axum::response::IntoResponse;

        let err = BridgeError::ConversationEnded("x".into());
        let response = err.into_response();
        assert_eq!(response.status(), axum::http::StatusCode::BAD_REQUEST);
    }

    #[test]
    fn bridge_error_into_response_invalid_request_is_400() {
        use axum::response::IntoResponse;

        let err = BridgeError::InvalidRequest("bad".into());
        let response = err.into_response();
        assert_eq!(response.status(), axum::http::StatusCode::BAD_REQUEST);
    }

    #[test]
    fn bridge_error_into_response_provider_error_is_500() {
        use axum::response::IntoResponse;

        let err = BridgeError::ProviderError("fail".into());
        let response = err.into_response();
        assert_eq!(
            response.status(),
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        );
    }

    #[test]
    fn bridge_error_into_response_mcp_error_is_500() {
        use axum::response::IntoResponse;

        let err = BridgeError::McpError("fail".into());
        let response = err.into_response();
        assert_eq!(
            response.status(),
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        );
    }

    #[test]
    fn bridge_error_into_response_tool_error_is_500() {
        use axum::response::IntoResponse;

        let err = BridgeError::ToolError("fail".into());
        let response = err.into_response();
        assert_eq!(
            response.status(),
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        );
    }

    #[test]
    fn bridge_error_into_response_config_error_is_500() {
        use axum::response::IntoResponse;

        let err = BridgeError::ConfigError("fail".into());
        let response = err.into_response();
        assert_eq!(
            response.status(),
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        );
    }

    #[test]
    fn bridge_error_into_response_webhook_error_is_500() {
        use axum::response::IntoResponse;

        let err = BridgeError::WebhookError("fail".into());
        let response = err.into_response();
        assert_eq!(
            response.status(),
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        );
    }

    #[test]
    fn bridge_error_into_response_internal_is_500() {
        use axum::response::IntoResponse;

        let err = BridgeError::Internal("fail".into());
        let response = err.into_response();
        assert_eq!(
            response.status(),
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        );
    }

    #[test]
    fn bridge_error_into_response_rate_limited_is_429() {
        use axum::response::IntoResponse;

        let err = BridgeError::RateLimited;
        let response = err.into_response();
        assert_eq!(response.status(), axum::http::StatusCode::TOO_MANY_REQUESTS);
    }

    #[test]
    fn bridge_error_into_response_body_contains_error_code() {
        use axum::body::to_bytes;
        use axum::response::IntoResponse;

        let err = BridgeError::AgentNotFound("agent-99".into());
        let response = err.into_response();

        let rt = tokio::runtime::Runtime::new().unwrap();
        let body_bytes = rt
            .block_on(to_bytes(response.into_body(), usize::MAX))
            .unwrap();
        let body: serde_json::Value = serde_json::from_slice(&body_bytes).unwrap();

        assert_eq!(body["error"]["code"], "agent_not_found");
        assert_eq!(body["error"]["message"], "agent not found: agent-99");
    }

    // ──────────────────────────────────────────────
    // Cross-module integration: full AgentDefinition from JSON string
    // ──────────────────────────────────────────────

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

    // ──────────────────────────────────────────────
    // IntegrationDefinition
    // ──────────────────────────────────────────────

    #[test]
    fn integration_definition_roundtrip() {
        let integration = IntegrationDefinition {
            name: "github".to_string(),
            description: "GitHub integration".to_string(),
            actions: vec![
                IntegrationAction {
                    name: "create_pull_request".to_string(),
                    description: "Create a new pull request".to_string(),
                    parameters_schema: serde_json::json!({
                        "type": "object",
                        "properties": {
                            "title": { "type": "string" },
                            "head": { "type": "string" },
                            "base": { "type": "string" }
                        },
                        "required": ["title", "head", "base"]
                    }),
                    permission: ToolPermission::RequireApproval,
                },
                IntegrationAction {
                    name: "list_issues".to_string(),
                    description: "List issues".to_string(),
                    parameters_schema: serde_json::json!({"type": "object"}),
                    permission: ToolPermission::Allow,
                },
                IntegrationAction {
                    name: "delete_repository".to_string(),
                    description: "Delete a repository".to_string(),
                    parameters_schema: serde_json::json!({"type": "object"}),
                    permission: ToolPermission::Deny,
                },
            ],
        };

        let json = serde_json::to_string_pretty(&integration).expect("serialize");
        let deserialized: IntegrationDefinition = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(integration, deserialized);
    }

    #[test]
    fn integration_action_permissions_serialize_correctly() {
        let action = IntegrationAction {
            name: "test".to_string(),
            description: "test".to_string(),
            parameters_schema: serde_json::json!({}),
            permission: ToolPermission::RequireApproval,
        };
        let json = serde_json::to_string(&action).expect("serialize");
        assert!(json.contains("\"require_approval\""));

        let action2 = IntegrationAction {
            name: "test".to_string(),
            description: "test".to_string(),
            parameters_schema: serde_json::json!({}),
            permission: ToolPermission::Deny,
        };
        let json2 = serde_json::to_string(&action2).expect("serialize");
        assert!(json2.contains("\"deny\""));
    }

    #[test]
    fn agent_definition_with_integrations_roundtrip() {
        let agent = AgentDefinition {
            id: "agent-int".to_string(),
            name: "Integration Agent".to_string(),
            description: None,
            system_prompt: "You have integrations.".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "key".to_string(),
                base_url: None,
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![IntegrationDefinition {
                name: "slack".to_string(),
                description: "Slack".to_string(),
                actions: vec![IntegrationAction {
                    name: "send_message".to_string(),
                    description: "Send a message".to_string(),
                    parameters_schema: serde_json::json!({
                        "type": "object",
                        "properties": {
                            "channel": { "type": "string" },
                            "text": { "type": "string" }
                        },
                        "required": ["channel", "text"]
                    }),
                    permission: ToolPermission::Allow,
                }],
            }],
            config: AgentConfig::default(),
            subagents: vec![],
            permissions: HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        };

        let json = serde_json::to_string_pretty(&agent).expect("serialize");
        assert!(json.contains("integrations"));
        assert!(json.contains("slack"));
        assert!(json.contains("send_message"));

        let deserialized: AgentDefinition = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(agent, deserialized);
    }

    #[test]
    fn agent_definition_empty_integrations_omitted_in_json() {
        let agent = AgentDefinition {
            id: "agent-no-int".to_string(),
            name: "No Integrations".to_string(),
            description: None,
            system_prompt: "Prompt".to_string(),
            provider: ProviderConfig {
                provider_type: ProviderType::OpenAI,
                model: "gpt-4o".to_string(),
                api_key: "key".to_string(),
                base_url: None,
            },
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: AgentConfig::default(),
            subagents: vec![],
            permissions: HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        };

        let json = serde_json::to_string(&agent).expect("serialize");
        assert!(
            !json.contains("integrations"),
            "empty integrations should be omitted via skip_serializing_if"
        );
    }
}
