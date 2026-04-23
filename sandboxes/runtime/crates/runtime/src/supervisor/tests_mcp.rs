use super::tests_conv::{make_test_definition, make_test_supervisor};
use bridge_core::mcp::{McpServerDefinition, McpTransport};

// ── per-conversation MCP server validation ──────────────────────────────

#[tokio::test]
async fn per_conv_mcp_rejects_stdio_when_flag_disabled() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let servers = vec![McpServerDefinition {
        name: "local".to_string(),
        transport: McpTransport::Stdio {
            command: "/bin/echo".to_string(),
            args: vec![],
            env: std::collections::HashMap::new(),
        },
    }];

    let result = supervisor
        .create_conversation("agent1", None, None, None, None, None, Some(servers))
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(
        err.contains("stdio transport not allowed"),
        "error should describe stdio gate, got: {err}"
    );
    assert!(
        err.contains("'local'"),
        "error should name server, got: {err}"
    );

    // Nothing should have leaked into the MCP manager.
    assert_eq!(supervisor.mcp_manager.connection_count(), 0);
    // No dangling conversation handle.
    let state = supervisor.get_agent("agent1").unwrap();
    assert_eq!(state.conversations.len(), 0);
}

#[tokio::test]
async fn per_conv_mcp_rejects_empty_server_name() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let servers = vec![McpServerDefinition {
        name: "   ".to_string(),
        transport: McpTransport::StreamableHttp {
            url: "http://127.0.0.1:1".to_string(),
            headers: std::collections::HashMap::new(),
        },
    }];

    let result = supervisor
        .create_conversation("agent1", None, None, None, None, None, Some(servers))
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(
        err.contains("server name cannot be empty"),
        "error should describe empty-name rejection, got: {err}"
    );
}

#[tokio::test]
async fn per_conv_mcp_rejects_duplicate_server_names() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let servers = vec![
        McpServerDefinition {
            name: "dup".to_string(),
            transport: McpTransport::StreamableHttp {
                url: "http://127.0.0.1:1".to_string(),
                headers: std::collections::HashMap::new(),
            },
        },
        McpServerDefinition {
            name: "dup".to_string(),
            transport: McpTransport::StreamableHttp {
                url: "http://127.0.0.1:2".to_string(),
                headers: std::collections::HashMap::new(),
            },
        },
    ];

    let result = supervisor
        .create_conversation("agent1", None, None, None, None, None, Some(servers))
        .await;

    assert!(result.is_err());
    let err = result.unwrap_err().to_string();
    assert!(
        err.contains("duplicate server name 'dup'"),
        "error should describe duplicate, got: {err}"
    );
}

#[tokio::test]
async fn per_conv_mcp_empty_list_is_no_op() {
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    // Empty servers vec — should succeed and behave identically to None.
    let result = supervisor
        .create_conversation("agent1", None, None, None, None, None, Some(vec![]))
        .await;

    assert!(result.is_ok(), "empty mcp_servers list should be a no-op");
    let (conv_id, _sse_rx) = result.unwrap();
    assert!(!conv_id.is_empty());
    assert_eq!(supervisor.mcp_manager.connection_count(), 0);
    supervisor.end_conversation("agent1", &conv_id).unwrap();
}

#[tokio::test]
async fn per_conv_mcp_unreachable_http_rolls_back_cleanly() {
    // Point at an unreachable TCP port so the connect attempt fails.
    // Expected: InvalidRequest error, no leaked MCP connections, no dangling
    // conversation handle. This exercises the error-unwind path.
    let supervisor = make_test_supervisor();
    supervisor
        .load_agents(vec![make_test_definition("agent1")])
        .await
        .unwrap();

    let servers = vec![McpServerDefinition {
        name: "unreachable".to_string(),
        transport: McpTransport::StreamableHttp {
            url: "http://127.0.0.1:1".to_string(),
            headers: std::collections::HashMap::new(),
        },
    }];

    let result = supervisor
        .create_conversation("agent1", None, None, None, None, None, Some(servers))
        .await;

    assert!(
        result.is_err(),
        "unreachable MCP server should surface an error"
    );
    assert_eq!(
        supervisor.mcp_manager.connection_count(),
        0,
        "no leaked MCP connections after failed connect"
    );
    let state = supervisor.get_agent("agent1").unwrap();
    assert_eq!(
        state.conversations.len(),
        0,
        "no dangling conversation handle after failed connect"
    );
}
