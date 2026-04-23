use bridge_core::mcp::{McpServerDefinition, McpTransport};
use bridge_core::BridgeError;
use std::sync::Arc;

use super::AgentSupervisor;
use crate::agent_state::AgentState;

pub(super) fn validate_per_conv_mcp_servers(
    supervisor: &AgentSupervisor,
    servers: Option<&[McpServerDefinition]>,
) -> Result<(), BridgeError> {
    let Some(servers) = servers else {
        return Ok(());
    };
    let mut seen = std::collections::HashSet::new();
    for server in servers {
        if server.name.trim().is_empty() {
            return Err(BridgeError::InvalidRequest(
                "mcp_servers: server name cannot be empty".to_string(),
            ));
        }
        if !seen.insert(server.name.clone()) {
            return Err(BridgeError::InvalidRequest(format!(
                "mcp_servers: duplicate server name '{}'",
                server.name
            )));
        }
        if matches!(server.transport, McpTransport::Stdio { .. })
            && !supervisor.allow_stdio_mcp_from_api
        {
            return Err(BridgeError::InvalidRequest(format!(
                "mcp_servers: stdio transport not allowed from API (server '{}'); \
                 enable allow_stdio_mcp_from_api in runtime config to permit it",
                server.name
            )));
        }
    }
    Ok(())
}

pub(super) async fn connect_per_conv_mcp(
    supervisor: &AgentSupervisor,
    state: &Arc<AgentState>,
    conv_id: &str,
    per_conversation_mcp_servers: Option<&[McpServerDefinition]>,
    tool_names: &mut std::collections::HashSet<String>,
    tool_executors: &mut std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
) -> Result<Option<String>, BridgeError> {
    match per_conversation_mcp_servers {
        Some(servers) if !servers.is_empty() => {
            let scope_id = conv_id.to_string();
            if let Err(e) = supervisor
                .mcp_manager
                .connect_agent(&scope_id, servers)
                .await
            {
                supervisor.mcp_manager.disconnect_agent(&scope_id).await;
                state.conversations.remove(conv_id);
                return Err(e);
            }

            let expected: std::collections::HashSet<&str> =
                servers.iter().map(|s| s.name.as_str()).collect();
            let connected: Vec<Arc<mcp::McpConnection>> =
                supervisor.mcp_manager.get_agent_connections(&scope_id);
            let connected_names: std::collections::HashSet<&str> =
                connected.iter().map(|c| c.server_name()).collect();
            for name in &expected {
                if !connected_names.contains(name) {
                    supervisor.mcp_manager.disconnect_agent(&scope_id).await;
                    state.conversations.remove(conv_id);
                    return Err(BridgeError::InvalidRequest(format!(
                        "mcp_servers: failed to connect to '{}'",
                        name
                    )));
                }
            }

            for conn in connected {
                let server_name = conn.server_name().to_string();
                let tools = match conn.list_tools().await {
                    Ok(t) => t,
                    Err(e) => {
                        supervisor.mcp_manager.disconnect_agent(&scope_id).await;
                        state.conversations.remove(conv_id);
                        return Err(BridgeError::InvalidRequest(format!(
                            "mcp_servers: failed to list tools from '{}': {}",
                            server_name, e
                        )));
                    }
                };
                let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                for tool in bridged {
                    let tool_name = tool.name().to_string();
                    if tool_names.contains(&tool_name) {
                        supervisor.mcp_manager.disconnect_agent(&scope_id).await;
                        state.conversations.remove(conv_id);
                        return Err(BridgeError::InvalidRequest(format!(
                            "mcp_servers: tool '{}' from server '{}' collides with an \
                             existing agent tool",
                            tool_name, server_name
                        )));
                    }
                    tool_names.insert(tool_name.clone());
                    tool_executors.insert(tool_name, tool);
                }
            }

            Ok(Some(scope_id))
        }
        _ => Ok(None),
    }
}
