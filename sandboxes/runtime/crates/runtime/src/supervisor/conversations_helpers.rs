use bridge_core::{AgentDefinition, BridgeError};
use dashmap::DashMap;
use llm::{adapt_tools, build_agent};
use std::collections::HashMap;
use std::sync::Arc;

use super::AgentSupervisor;
use crate::agent_runner::SubAgentEntry;
use crate::agent_state::AgentState;

/// Register the journal read/write tools onto the per-conversation tool maps
/// when immortal mode is active for this conversation.
pub(super) fn register_journal_tools(
    journal_state: &Arc<tools::journal::JournalState>,
    tool_names: &mut std::collections::HashSet<String>,
    tool_executors: &mut std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
) {
    use tools::registry::ToolExecutor;

    let write_tool = Arc::new(tools::journal::JournalWriteTool::new(journal_state.clone()));
    tool_names.insert(write_tool.name().to_string());
    tool_executors.insert(
        write_tool.name().to_string(),
        write_tool.clone() as Arc<dyn tools::ToolExecutor>,
    );

    let read_tool = Arc::new(tools::journal::JournalReadTool::new(journal_state.clone()));
    tool_names.insert(read_tool.name().to_string());
    tool_executors.insert(
        read_tool.name().to_string(),
        read_tool.clone() as Arc<dyn tools::ToolExecutor>,
    );
}

/// Build the stable system reminder for a conversation. When todo tools are
/// enabled, includes todo state and current date. Otherwise just skills +
/// subagents.
pub(super) async fn build_system_reminder(
    state: &Arc<AgentState>,
    tool_executors: &std::collections::HashMap<String, Arc<dyn tools::ToolExecutor>>,
    has_todo_tools: bool,
) -> String {
    // Get skills from the registered SkillTool (includes local discoveries)
    let skills = state
        .tool_registry
        .get("skill")
        .and_then(|t| {
            t.as_any()
                .downcast_ref::<tools::skill_tools::SkillTool>()
                .map(|st| st.skills().clone())
        })
        .unwrap_or_default();

    // Extract subagent names and descriptions, filtering out __self__
    let subagent_list: Vec<(String, String)> = state
        .subagents
        .iter()
        .filter(|entry| entry.key() != tools::self_agent::SELF_AGENT_NAME)
        .map(|entry| {
            (
                entry.value().name.clone(),
                entry.value().description.clone(),
            )
        })
        .collect();

    if has_todo_tools {
        // Try to get todo state from the tool registry
        let todos = super::helpers::get_todos_from_registry(tool_executors).await;
        crate::system_reminder::create_reminder_with_skills_todos_and_date(
            &skills,
            &subagent_list,
            todos.as_deref(),
            chrono::Utc::now(),
        )
    } else {
        crate::system_reminder::create_reminder_with_skills(&skills, &subagent_list)
    }
}

/// Acquire the global conversation permit (if configured) and validate the
/// per-agent concurrent-conversation cap.
pub(super) async fn acquire_conversation_permit(
    supervisor: &AgentSupervisor,
    state: &Arc<AgentState>,
    agent_id: &str,
) -> Result<Option<tokio::sync::OwnedSemaphorePermit>, BridgeError> {
    let conversation_permit = match &supervisor.conversation_semaphore {
        Some(sem) => match sem.clone().try_acquire_owned() {
            Ok(permit) => Some(permit),
            Err(_) => {
                return Err(BridgeError::CapacityExhausted(
                    "global max concurrent conversations reached".to_string(),
                ));
            }
        },
        None => None,
    };

    // --- Admission control: per-agent conversation limit ---
    {
        let def = state.definition.read().await;
        if let Some(max) = def.config.max_concurrent_conversations {
            if state.conversations.len() >= max as usize {
                return Err(BridgeError::CapacityExhausted(format!(
                    "agent {} at max concurrent conversations ({})",
                    agent_id, max
                )));
            }
        }
    }

    Ok(conversation_permit)
}

pub(super) fn validate_api_key_overrides(
    state: &Arc<AgentState>,
    api_key_override: Option<&str>,
    subagent_api_key_overrides: Option<&HashMap<String, String>>,
) -> Result<(), BridgeError> {
    if let Some(key) = api_key_override {
        if key.trim().is_empty() {
            return Err(BridgeError::InvalidRequest(
                "api_key cannot be empty".to_string(),
            ));
        }
    }
    if let Some(overrides) = subagent_api_key_overrides {
        for (name, key) in overrides {
            if key.trim().is_empty() {
                return Err(BridgeError::InvalidRequest(format!(
                    "subagent_api_keys: key for '{}' cannot be empty",
                    name
                )));
            }
            if !state.subagents.contains_key(name) {
                return Err(BridgeError::InvalidRequest(format!(
                    "subagent_api_keys: unknown subagent '{}'",
                    name
                )));
            }
        }
    }
    Ok(())
}

pub(super) fn build_conversation_subagents(
    state: &Arc<AgentState>,
    def: &AgentDefinition,
    subagent_api_key_overrides: Option<&HashMap<String, String>>,
) -> Result<Arc<DashMap<String, SubAgentEntry>>, BridgeError> {
    let Some(overrides) = subagent_api_key_overrides else {
        return Ok(state.subagents.clone());
    };
    let scoped_map = Arc::new(DashMap::new());
    let control_plane_url = std::env::var("BRIDGE_CONTROL_PLANE_URL")
        .unwrap_or_else(|_| "http://localhost:3000".to_string());
    let integration_tools =
        tools::integration::create_integration_tools(&def.integrations, &control_plane_url);

    for entry in state.subagents.iter() {
        let name = entry.key().clone();
        let original = entry.value();
        if let Some(override_key) = overrides.get(&name) {
            // Find the subagent definition and rebuild with overridden key
            if let Some(sub_def) = def.subagents.iter().find(|s| s.name == name) {
                let mut overridden_def = sub_def.clone();
                overridden_def.provider.api_key = override_key.clone();

                let mut sub_registry = tools::ToolRegistry::new();
                tools::builtin::register_builtin_tools_for_subagent(&mut sub_registry);
                for (tool, _) in &integration_tools {
                    sub_registry.register(tool.clone());
                }
                let sub_executors: Vec<Arc<dyn tools::ToolExecutor>> = sub_registry
                    .list()
                    .iter()
                    .filter_map(|(n, _)| sub_registry.get(n))
                    .collect();
                let sub_dynamic = adapt_tools(sub_executors)?;
                let sub_agent = build_agent(&overridden_def, sub_dynamic)?;

                scoped_map.insert(
                    name,
                    SubAgentEntry {
                        name: original.name.clone(),
                        description: original.description.clone(),
                        agent: Arc::new(sub_agent),
                        registered_tools: original.registered_tools.clone(),
                        foreground_timeout: original.foreground_timeout,
                        background_timeout: original.background_timeout,
                    },
                );
            } else {
                // Subagent name exists in runtime but not in definition (e.g. __self__)
                scoped_map.insert(
                    name,
                    SubAgentEntry {
                        name: original.name.clone(),
                        description: original.description.clone(),
                        agent: original.agent.clone(),
                        registered_tools: original.registered_tools.clone(),
                        foreground_timeout: original.foreground_timeout,
                        background_timeout: original.background_timeout,
                    },
                );
            }
        } else {
            // No override — share original entry
            scoped_map.insert(
                name,
                SubAgentEntry {
                    name: original.name.clone(),
                    description: original.description.clone(),
                    agent: original.agent.clone(),
                    registered_tools: original.registered_tools.clone(),
                    foreground_timeout: original.foreground_timeout,
                    background_timeout: original.background_timeout,
                },
            );
        }
    }
    Ok(scoped_map)
}
