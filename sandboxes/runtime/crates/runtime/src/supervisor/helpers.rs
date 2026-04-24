use bridge_core::{AgentDefinition, BridgeError};
use dashmap::DashMap;
use llm::{adapt_tools, build_agent};
use mcp::McpManager;
use std::sync::Arc;
use storage::StorageBackend;
use tools::ToolRegistry;
use tracing::error;

use crate::agent_runner::SubAgentEntry;

/// Default maximum concurrent LLM calls when not configured.
pub(super) const DEFAULT_MAX_CONCURRENT_LLM_CALLS: usize = 500;

pub(super) fn definitions_equivalent(
    existing: &AgentDefinition,
    incoming: &AgentDefinition,
) -> bool {
    match (&existing.version, &incoming.version) {
        (Some(existing_version), Some(incoming_version)) => existing_version == incoming_version,
        _ => existing == incoming,
    }
}

/// Apply optional tool/MCP scoping filters to a conversation's tool set.
///
/// - `mcp_server_names`: if provided, only tools from these MCP servers are kept.
/// - `tool_names_filter`: if provided, only these tools are kept (applied after MCP filter).
///
/// Returns `Err(BridgeError::InvalidRequest)` if any name is unrecognized.
pub(crate) fn filter_conversation_tools(
    agent_id: &str,
    tool_names: &mut std::collections::HashSet<String>,
    tool_executors: &mut std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
    mcp_server_tools: &std::collections::HashMap<String, Vec<String>>,
    filter_mcp_server_names: Option<&Vec<String>>,
    filter_tool_names: Option<&Vec<String>>,
) -> Result<(), BridgeError> {
    // Apply MCP server name filter: keep only tools from specified servers
    if let Some(server_names) = filter_mcp_server_names {
        for name in server_names {
            if !mcp_server_tools.contains_key(name) {
                return Err(BridgeError::InvalidRequest(format!(
                    "MCP server '{}' not found on agent '{}'",
                    name, agent_id
                )));
            }
        }
        let allowed_mcp_tools: std::collections::HashSet<String> = server_names
            .iter()
            .filter_map(|name| mcp_server_tools.get(name))
            .flat_map(|tools| tools.iter().cloned())
            .collect();
        let all_mcp_tools: std::collections::HashSet<String> = mcp_server_tools
            .values()
            .flat_map(|v| v.iter().cloned())
            .collect();
        let disallowed: std::collections::HashSet<String> = all_mcp_tools
            .difference(&allowed_mcp_tools)
            .cloned()
            .collect();
        tool_names.retain(|n| !disallowed.contains(n));
        tool_executors.retain(|n, _| !disallowed.contains(n));
    }

    // Apply tool name filter: retain only the requested tools
    if let Some(requested) = filter_tool_names {
        let requested_set: std::collections::HashSet<String> = requested.iter().cloned().collect();
        for name in &requested_set {
            if !tool_names.contains(name) {
                return Err(BridgeError::InvalidRequest(format!(
                    "tool '{}' not found on agent '{}'",
                    name, agent_id
                )));
            }
        }
        tool_names.retain(|n| requested_set.contains(n));
        tool_executors.retain(|n, _| requested_set.contains(n));
    }

    Ok(())
}

/// Build subagent entries from an agent definition's subagents list.
///
/// Each subagent gets built-in tools, the parent's integration tools
/// (with the same permissions), its own MCP server tools (if defined),
/// and its own skills (if defined). Skill files are written to disk
/// idempotently so shared skills between parent and subagents are safe.
/// No agent tool to prevent unbounded recursion at the configuration level.
pub(super) async fn build_subagents(
    definition: &AgentDefinition,
    parent_integration_tools: &[(
        Arc<dyn tools::ToolExecutor>,
        bridge_core::permission::ToolPermission,
    )],
    mcp_manager: &McpManager,
    working_dir: &std::path::Path,
) -> Result<Arc<DashMap<String, SubAgentEntry>>, BridgeError> {
    let subagent_map = Arc::new(DashMap::new());

    for subagent_def in &definition.subagents {
        let mut sub_registry = ToolRegistry::new();
        tools::builtin::register_builtin_tools_for_subagent(&mut sub_registry);

        // Inherit parent's integration tools with same permissions
        for (tool, _) in parent_integration_tools {
            sub_registry.register(tool.clone());
        }

        // Connect subagent to its own MCP servers (if any)
        if !subagent_def.mcp_servers.is_empty() {
            let subagent_mcp_id = format!("{}::subagent::{}", definition.id, subagent_def.name);
            mcp_manager
                .connect_agent(&subagent_mcp_id, &subagent_def.mcp_servers)
                .await?;

            let connections = mcp_manager.get_agent_connections(&subagent_mcp_id);
            for conn in &connections {
                if let Ok(tools) = conn.list_tools().await {
                    let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                    for tool in bridged {
                        sub_registry.register(tool);
                    }
                }
            }
        }

        // Register subagent's skills (if any)
        if !subagent_def.skills.is_empty() {
            tools::skill_files::write_skill_files(&subagent_def.skills, working_dir).await;
            sub_registry.register(Arc::new(tools::skill_tools::SkillTool::with_base_dir(
                subagent_def.skills.clone(),
                working_dir.to_path_buf(),
            )));
        }

        // Apply subagent's disabled_tools
        for name in &subagent_def.config.disabled_tools {
            sub_registry.remove(name);
        }

        // Capture tool list before consuming the registry
        let registered_tools: Vec<(String, String)> = sub_registry
            .list()
            .into_iter()
            .map(|(name, desc)| (name.to_string(), desc.to_string()))
            .collect();

        let sub_executors: Vec<Arc<dyn tools::ToolExecutor>> = sub_registry
            .list()
            .iter()
            .filter_map(|(name, _)| sub_registry.get(name))
            .collect();

        let sub_dynamic = adapt_tools(sub_executors)?;
        let sub_agent = build_agent(subagent_def, sub_dynamic)?;

        let description = subagent_def
            .description
            .clone()
            .unwrap_or_else(|| subagent_def.name.clone());

        let (fg_timeout, bg_timeout) =
            crate::agent_runner::resolve_subagent_timeouts(&subagent_def.config);

        subagent_map.insert(
            subagent_def.name.clone(),
            SubAgentEntry {
                name: subagent_def.name.clone(),
                description,
                agent: Arc::new(sub_agent),
                registered_tools,
                foreground_timeout: fg_timeout,
                background_timeout: bg_timeout,
            },
        );
    }

    Ok(subagent_map)
}

pub(super) async fn restore_agent_sessions(
    storage_backend: &dyn StorageBackend,
    agent_id: &str,
    session_store: &Arc<crate::agent_runner::AgentSessionStore>,
) {
    match storage_backend.load_sessions(agent_id).await {
        Ok(sessions) => {
            for (task_id, history_json) in sessions {
                match serde_json::from_slice::<Vec<rig::message::Message>>(&history_json) {
                    Ok(history) => session_store.restore(task_id, history),
                    Err(e) => {
                        error!(agent_id = %agent_id, error = %e, "failed to deserialize stored session history")
                    }
                }
            }
        }
        Err(e) => {
            error!(agent_id = %agent_id, error = %e, "failed to load stored sessions");
        }
    }
}

/// Extract `PingState` from the tool registry by downcasting the ping_me_back_in tool.
pub(super) fn get_ping_state_from_registry(
    tool_executors: &std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
) -> Option<tools::ping_me_back::PingState> {
    tool_executors
        .get("ping_me_back_in")
        .and_then(|tool| {
            tool.as_ref()
                .as_any()
                .downcast_ref::<tools::ping_me_back::PingMeBackTool>()
        })
        .map(|t| t.state().clone())
}
