use bridge_core::mcp::McpServerDefinition;
use bridge_core::BridgeError;
use dashmap::DashMap;
use std::sync::Arc;
use tracing::{error, info};

use crate::connection::{McpConnection, McpToolInfo};

/// Manages MCP server connections for all agents.
///
/// Connections are keyed by (agent_id, server_name) so each agent can have
/// its own set of MCP server connections that are independently managed.
pub struct McpManager {
    /// Map of (agent_id, server_name) → McpConnection
    connections: DashMap<(String, String), Arc<McpConnection>>,
}

impl McpManager {
    /// Create a new empty MCP manager.
    pub fn new() -> Self {
        Self {
            connections: DashMap::new(),
        }
    }

    /// Connect an agent to all its configured MCP servers.
    ///
    /// Returns the list of all discovered tools across all servers.
    pub async fn connect_agent(
        &self,
        agent_id: &str,
        servers: &[McpServerDefinition],
    ) -> Result<Vec<McpToolInfo>, BridgeError> {
        let mut all_tools = Vec::new();

        for server in servers {
            match McpConnection::connect(&server.name, &server.transport).await {
                Ok(conn) => {
                    info!(
                        agent_id = agent_id,
                        server = server.name,
                        "connected to MCP server"
                    );

                    match conn.list_tools().await {
                        Ok(tools) => {
                            info!(
                                agent_id = agent_id,
                                server = server.name,
                                tool_count = tools.len(),
                                "discovered MCP tools"
                            );
                            all_tools.extend(tools);
                        }
                        Err(e) => {
                            error!(
                                agent_id = agent_id,
                                server = server.name,
                                error = %e,
                                "failed to list tools from MCP server"
                            );
                        }
                    }

                    let key = (agent_id.to_string(), server.name.clone());
                    self.connections.insert(key, Arc::new(conn));
                }
                Err(e) => {
                    error!(
                        agent_id = agent_id,
                        server = server.name,
                        error = %e,
                        "failed to connect to MCP server"
                    );
                }
            }
        }

        Ok(all_tools)
    }

    /// Disconnect all MCP servers for a given agent.
    pub async fn disconnect_agent(&self, agent_id: &str) {
        let keys_to_remove: Vec<(String, String)> = self
            .connections
            .iter()
            .filter(|entry| entry.key().0 == agent_id)
            .map(|entry| entry.key().clone())
            .collect();

        for key in keys_to_remove {
            if let Some((_, conn)) = self.connections.remove(&key) {
                info!(
                    agent_id = agent_id,
                    server = key.1,
                    "disconnecting MCP server"
                );
                if let Ok(conn) = Arc::try_unwrap(conn) {
                    conn.disconnect().await;
                }
            }
        }
    }

    /// Get a connection to a specific MCP server for an agent.
    pub fn get_connection(&self, agent_id: &str, server_name: &str) -> Option<Arc<McpConnection>> {
        let key = (agent_id.to_string(), server_name.to_string());
        self.connections
            .get(&key)
            .map(|entry| entry.value().clone())
    }

    /// Get all connections for a given agent.
    pub fn get_agent_connections(&self, agent_id: &str) -> Vec<Arc<McpConnection>> {
        self.connections
            .iter()
            .filter(|entry| entry.key().0 == agent_id)
            .map(|entry| entry.value().clone())
            .collect()
    }

    /// Get the total number of active connections.
    pub fn connection_count(&self) -> usize {
        self.connections.len()
    }
}

impl Default for McpManager {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_new_manager_is_empty() {
        let manager = McpManager::new();
        assert_eq!(manager.connection_count(), 0);
    }

    #[test]
    fn test_default_manager_is_empty() {
        let manager = McpManager::default();
        assert_eq!(manager.connection_count(), 0);
    }

    #[test]
    fn test_get_connection_returns_none_for_unknown() {
        let manager = McpManager::new();
        assert!(manager.get_connection("agent1", "server1").is_none());
    }

    #[test]
    fn test_get_agent_connections_returns_empty_for_unknown() {
        let manager = McpManager::new();
        let conns = manager.get_agent_connections("agent1");
        assert!(conns.is_empty());
    }

    #[test]
    fn test_get_connection_returns_none_for_wrong_agent() {
        let manager = McpManager::new();
        // No connections exist, so querying any agent/server pair returns None
        assert!(manager.get_connection("agent1", "server_a").is_none());
        assert!(manager.get_connection("agent2", "server_a").is_none());
    }

    #[test]
    fn test_get_agent_connections_returns_empty_for_multiple_unknown_agents() {
        let manager = McpManager::new();
        assert!(manager.get_agent_connections("agent1").is_empty());
        assert!(manager.get_agent_connections("agent2").is_empty());
        assert!(manager.get_agent_connections("").is_empty());
    }

    #[test]
    fn test_dashmap_keying_different_agents_same_server() {
        // Verify the (agent_id, server_name) tuple keying logic:
        // Two different agents connecting to the same server name should produce
        // distinct keys.
        let key1 = ("agent1".to_string(), "server_a".to_string());
        let key2 = ("agent2".to_string(), "server_a".to_string());
        assert_ne!(key1, key2);
    }

    #[test]
    fn test_dashmap_keying_same_agent_different_servers() {
        let key1 = ("agent1".to_string(), "server_a".to_string());
        let key2 = ("agent1".to_string(), "server_b".to_string());
        assert_ne!(key1, key2);
    }

    #[test]
    fn test_dashmap_keying_identical_keys() {
        let key1 = ("agent1".to_string(), "server_a".to_string());
        let key2 = ("agent1".to_string(), "server_a".to_string());
        assert_eq!(key1, key2);
    }

    #[tokio::test]
    async fn test_disconnect_agent_on_empty_manager() {
        let manager = McpManager::new();
        // Should not panic when disconnecting a non-existent agent
        manager.disconnect_agent("nonexistent").await;
        assert_eq!(manager.connection_count(), 0);
    }

    #[tokio::test]
    async fn test_connect_agent_with_no_servers() {
        let manager = McpManager::new();
        let tools = manager.connect_agent("agent1", &[]).await.unwrap();
        assert!(tools.is_empty());
        assert_eq!(manager.connection_count(), 0);
    }

    #[tokio::test]
    async fn test_connect_agent_with_invalid_stdio_server() {
        let manager = McpManager::new();
        let servers = vec![McpServerDefinition {
            name: "bad_server".to_string(),
            transport: bridge_core::mcp::McpTransport::Stdio {
                command: "/nonexistent/binary/that/does/not/exist".to_string(),
                args: vec![],
                env: std::collections::HashMap::new(),
            },
        }];
        // Should not panic; the server fails to connect but the manager handles it gracefully
        let tools = manager.connect_agent("agent1", &servers).await.unwrap();
        assert!(tools.is_empty());
        assert_eq!(manager.connection_count(), 0);
    }
}
