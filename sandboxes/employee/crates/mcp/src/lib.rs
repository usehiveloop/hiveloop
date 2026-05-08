//! MCP server registry — spawns/supervises stdio child processes, manages HTTP
//! MCP clients, exposes their tools through adk-rust's `McpToolset`. Reconciles
//! against `AgentDefinition.mcp_servers` on every config update.
//!
//! Implementation lands in Phase 3B.
