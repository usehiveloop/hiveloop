use bridge_core::{AgentDefinition, BridgeError};
use dashmap::DashMap;
use llm::{adapt_tools, build_agent, DynamicTool};
use std::sync::Arc;
use tools::ToolRegistry;

use super::helpers::build_subagents;
use super::AgentSupervisor;
use crate::agent_runner::SubAgentEntry;

/// Output of [`build_agent_state_common`] — contains everything needed
/// to construct an `AgentState` (minus the persist/insert step).
pub(super) struct BuiltAgentState {
    pub(super) definition: AgentDefinition,
    pub(super) rig_agent: llm::BridgeAgent,
    pub(super) tool_registry: ToolRegistry,
    pub(super) subagent_map: Arc<DashMap<String, SubAgentEntry>>,
    pub(super) mcp_server_tools: std::collections::HashMap<String, Vec<String>>,
}

impl AgentSupervisor {
    /// Build the tool registry, rig agent, and subagent map for a definition.
    ///
    /// `is_update` is true when replacing an existing agent (triggers cleanup
    /// of old skill files / subagent MCP connections).
    pub(super) async fn build_agent_state_common(
        &self,
        mut definition: AgentDefinition,
        is_update: bool,
    ) -> Result<BuiltAgentState, BridgeError> {
        let agent_id = definition.id.clone();

        self.mcp_manager
            .connect_agent(&agent_id, &definition.mcp_servers)
            .await?;

        // Extract tool allow-list early — used for both MCP and built-in tool filtering.
        // Priority: if definition.tools is non-empty, use those names (legacy behavior).
        // Otherwise, if definition.permissions is non-empty, treat its keys as the
        // allow-list for built-in tools. Unknown keys (MCP tool names, integration
        // tool names) are harmlessly ignored by the built-in filter.
        let builtin_tool_names: Vec<String> = if !definition.tools.is_empty() {
            definition.tools.iter().map(|t| t.name.clone()).collect()
        } else if !definition.permissions.is_empty() {
            definition.permissions.keys().cloned().collect()
        } else {
            Vec::new()
        };

        let mut tool_registry = ToolRegistry::new();
        let mut mcp_server_tools: std::collections::HashMap<String, Vec<String>> =
            std::collections::HashMap::new();
        let connections = self.mcp_manager.get_agent_connections(&agent_id);
        for conn in &connections {
            if let Ok(tools) = conn.list_tools().await {
                let server_name = conn.server_name().to_string();
                let tool_names: Vec<String> = tools.iter().map(|t| t.name.clone()).collect();
                mcp_server_tools.insert(server_name, tool_names);
                let bridged = mcp::bridge_mcp_tools(conn.clone(), tools);
                for tool in bridged {
                    tool_registry.register(tool);
                }
            }
        }

        // Register built-in tools — filtered by the agent's tool list if non-empty
        if builtin_tool_names.is_empty() {
            tools::builtin::register_builtin_tools_with_lsp(
                &mut tool_registry,
                self.lsp_manager.clone(),
            );
        } else {
            tools::builtin::register_filtered_builtin_tools_with_lsp(
                &mut tool_registry,
                &builtin_tool_names,
                self.lsp_manager.clone(),
            );
        }

        // Merge control-plane skills with locally discovered skills
        let all_skills = self
            .merge_with_discovered_skills(definition.skills.clone())
            .await;
        if !all_skills.is_empty() {
            let base_dir = self.resolve_working_dir();

            // On updates only: clean up old skill files and subagent MCP connections
            // from the previous generation of this agent before writing the new ones.
            if is_update {
                if let Some(old_state) = self.agent_map.get(&agent_id) {
                    let old_def = old_state.definition.read().await;
                    let mut old_skill_ids: Vec<&str> =
                        old_def.skills.iter().map(|s| s.id.as_str()).collect();
                    for subagent_def in &old_def.subagents {
                        for skill in &subagent_def.skills {
                            old_skill_ids.push(skill.id.as_str());
                        }
                        let subagent_mcp_id =
                            format!("{}::subagent::{}", agent_id, subagent_def.name);
                        self.mcp_manager.disconnect_agent(&subagent_mcp_id).await;
                    }
                    tools::skill_files::cleanup_skill_files(&old_skill_ids, &base_dir).await;
                }
            }

            tools::skill_files::write_skill_files(&all_skills, &base_dir).await;
            tool_registry.register(Arc::new(tools::skill_tools::SkillTool::with_base_dir(
                all_skills, base_dir,
            )));
        }

        // Register integration tools and inject their permissions
        let control_plane_url = std::env::var("BRIDGE_CONTROL_PLANE_URL")
            .unwrap_or_else(|_| "http://localhost:3000".to_string());
        let integration_tools = tools::integration::create_integration_tools(
            &definition.integrations,
            &control_plane_url,
        );
        for (tool, permission) in &integration_tools {
            tool_registry.register(tool.clone());
            if *permission != bridge_core::permission::ToolPermission::Allow {
                definition
                    .permissions
                    .insert(tool.name().to_string(), permission.clone());
            }
        }

        // Register the upload_to_workspace tool when the agent declares an
        // artifacts config. The tool streams chunks to the control plane via
        // tus.io and persists in-flight state to sqlite for crash-resume.
        if let Some(artifacts_cfg) = definition.artifacts.clone() {
            let bearer = std::env::var("BRIDGE_CONTROL_PLANE_API_KEY")
                .ok()
                .filter(|s| !s.is_empty());
            let boundary = Some(tools::ProjectBoundary::new(self.resolve_working_dir()));
            tool_registry.register(Arc::new(tools::artifacts::UploadToWorkspaceTool::new(
                artifacts_cfg,
                agent_id.clone(),
                bearer,
                boundary,
                self.storage_backend.clone(),
            )));
        }

        // Remove disabled tools — takes priority over everything else.
        // The LLM will never see these tools.
        for name in &definition.config.disabled_tools {
            tool_registry.remove(name);
        }

        // (Duplicate removal preserved from original code for behavior parity)
        if !is_update {
            for name in &definition.config.disabled_tools {
                tool_registry.remove(name);
            }
        }

        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| tool_registry.get(name))
            .collect();

        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;

        // Build the rig agent
        let rig_agent = build_agent(&definition, dynamic_tools)?;

        // Build subagents from definition.subagents
        let subagent_map = build_subagents(
            &definition,
            &integration_tools,
            &self.mcp_manager,
            &self.resolve_working_dir(),
        )
        .await?;

        // Inject the parent agent as "__self__" for self-delegation
        let (self_fg_timeout, self_bg_timeout) =
            crate::agent_runner::resolve_subagent_timeouts(&definition.config);
        subagent_map.insert(
            tools::self_agent::SELF_AGENT_NAME.to_string(),
            SubAgentEntry {
                name: tools::self_agent::SELF_AGENT_NAME.to_string(),
                description: "Self-delegation agent".to_string(),
                agent: Arc::new(rig_agent.clone()),
                registered_tools: vec![], // uses parent's tool registry
                foreground_timeout: self_fg_timeout,
                background_timeout: self_bg_timeout,
            },
        );

        Ok(BuiltAgentState {
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            mcp_server_tools,
        })
    }
}
