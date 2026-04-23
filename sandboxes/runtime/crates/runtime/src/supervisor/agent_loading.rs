use bridge_core::{AgentDefinition, BridgeError};
use llm::{adapt_tools, build_agent, DynamicTool};
use std::sync::Arc;
use tracing::{error, info};

use super::helpers::{definitions_equivalent, restore_agent_sessions};
use super::AgentSupervisor;
use crate::agent_state::AgentState;
use crate::drain::drain_and_replace;

impl AgentSupervisor {
    /// Build and load a single agent.
    pub(super) async fn load_single_agent(
        &self,
        definition: AgentDefinition,
    ) -> Result<(), BridgeError> {
        let built = self.build_agent_state_common(definition, false).await?;

        let super::agent_build::BuiltAgentState {
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            mcp_server_tools,
        } = built;

        let agent_id = definition.id.clone();
        let persisted_definition = definition.clone();

        let state = Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            self.storage.clone(),
            mcp_server_tools,
        ));

        if let Some(storage_backend) = &self.storage_backend {
            restore_agent_sessions(storage_backend.as_ref(), &agent_id, &state.session_store).await;
        }

        self.agent_map.insert(agent_id.clone(), state);

        if let Some(storage) = &self.storage {
            storage.save_agent(persisted_definition);
        }

        info!(agent_id = agent_id, "agent loaded");
        Ok(())
    }

    /// Build agent state without inserting into the map (used for drain_and_replace).
    pub(super) async fn load_single_agent_state(
        &self,
        definition: AgentDefinition,
    ) -> Result<Arc<AgentState>, BridgeError> {
        let built = self.build_agent_state_common(definition, true).await?;

        let super::agent_build::BuiltAgentState {
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            mcp_server_tools,
        } = built;

        let agent_id = definition.id.clone();

        let state = Arc::new(AgentState::new(
            definition,
            rig_agent,
            tool_registry,
            subagent_map,
            self.storage.clone(),
            mcp_server_tools,
        ));

        if let Some(storage_backend) = &self.storage_backend {
            restore_agent_sessions(storage_backend.as_ref(), &agent_id, &state.session_store).await;
        }

        Ok(state)
    }

    /// Load a set of agent definitions, building their runtime state.
    pub async fn load_agents(&self, definitions: Vec<AgentDefinition>) -> Result<(), BridgeError> {
        for def in definitions {
            let agent_id = def.id.clone();

            if let Some(existing) = self.agent_map.get(&agent_id) {
                let existing_def = existing.definition.read().await.clone();
                if definitions_equivalent(&existing_def, &def) {
                    continue;
                }

                match self.load_single_agent_state(def).await {
                    Ok(new_state) => {
                        let persisted_def = new_state.definition.read().await.clone();
                        let timeout = std::time::Duration::from_secs(60);
                        if let Err(e) =
                            drain_and_replace(&self.agent_map, &agent_id, new_state, timeout).await
                        {
                            error!(agent_id = agent_id, error = %e, "failed to replace existing agent");
                        } else if let Some(storage) = &self.storage {
                            storage.save_agent(persisted_def);
                        }
                    }
                    Err(e) => {
                        error!(agent_id = agent_id, error = %e, "failed to rebuild existing agent");
                    }
                }
            } else if let Err(e) = self.load_single_agent(def).await {
                error!(error = %e, "failed to load agent");
            }
        }
        Ok(())
    }

    /// Apply a diff of agent changes (add/update/remove).
    pub async fn apply_diff(
        &self,
        added: Vec<AgentDefinition>,
        updated: Vec<AgentDefinition>,
        removed: Vec<String>,
    ) -> Result<(), BridgeError> {
        // Add new agents
        for def in added {
            if let Err(e) = self.load_single_agent(def).await {
                error!(error = %e, "failed to add agent");
            }
        }

        // Update existing agents with drain
        for def in updated {
            let agent_id = def.id.clone();
            match self.load_single_agent_state(def).await {
                Ok(new_state) => {
                    let persisted_def = new_state.definition.read().await.clone();
                    let timeout = std::time::Duration::from_secs(60);
                    if let Err(e) =
                        drain_and_replace(&self.agent_map, &agent_id, new_state, timeout).await
                    {
                        error!(agent_id = agent_id, error = %e, "failed to drain and replace agent");
                    } else if let Some(storage) = &self.storage {
                        storage.save_agent(persisted_def);
                    }
                }
                Err(e) => {
                    error!(agent_id = agent_id, error = %e, "failed to build updated agent");
                }
            }
        }

        // Remove agents
        for agent_id in removed {
            if let Some(state) = self.agent_map.remove(&agent_id) {
                // Clean up materialized skill files and subagent MCP connections.
                {
                    let base_dir = self.resolve_working_dir();
                    let def = state.definition.read().await;

                    // Collect skill IDs from parent and all subagents for cleanup.
                    let mut skill_ids: Vec<&str> =
                        def.skills.iter().map(|s| s.id.as_str()).collect();
                    for subagent_def in &def.subagents {
                        for skill in &subagent_def.skills {
                            skill_ids.push(skill.id.as_str());
                        }
                        let subagent_mcp_id =
                            format!("{}::subagent::{}", agent_id, subagent_def.name);
                        self.mcp_manager.disconnect_agent(&subagent_mcp_id).await;
                    }
                    tools::skill_files::cleanup_skill_files(&skill_ids, &base_dir).await;
                }

                state.cancel.cancel();
                state.tracker.close();
                state.tracker.wait().await;
                self.mcp_manager.disconnect_agent(&agent_id).await;
                if let Some(storage) = &self.storage {
                    storage.delete_agent(agent_id.clone());
                }
                info!(agent_id = agent_id, "agent removed");
            }
        }

        Ok(())
    }

    /// Update the API key for an agent at runtime.
    ///
    /// Rebuilds the `BridgeAgent` with the new key and swaps it in-place so
    /// both existing and new conversations pick up the rotated key on their
    /// next LLM turn. No drain, no cancellation.
    pub async fn update_agent_api_key(
        &self,
        agent_id: &str,
        api_key: String,
    ) -> Result<(), BridgeError> {
        let state = self
            .agent_map
            .get(agent_id)
            .ok_or_else(|| BridgeError::AgentNotFound(agent_id.to_string()))?;

        // Clone current definition and update the API key
        let mut updated_def = state.definition.read().await.clone();
        updated_def.provider.api_key = api_key;

        // Rebuild the agent with existing tools
        let all_executors: Vec<Arc<dyn tools::ToolExecutor>> = state
            .tool_registry
            .list()
            .iter()
            .filter_map(|(name, _)| state.tool_registry.get(name))
            .collect();
        let dynamic_tools: Vec<DynamicTool> = adapt_tools(all_executors)?;
        let new_agent = build_agent(&updated_def, dynamic_tools)?;

        // Swap in the new agent — all conversations sharing this Arc see the new agent on next turn
        *state.rig_agent.write().await = new_agent;
        *state.definition.write().await = updated_def.clone();

        if let Some(storage) = &self.storage {
            storage.save_agent(updated_def.clone());
        }

        info!(agent_id = agent_id, "agent API key updated");
        Ok(())
    }
}
