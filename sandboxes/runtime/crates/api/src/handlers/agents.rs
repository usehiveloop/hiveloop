use std::collections::HashMap;

use axum::extract::{Path, State};
use axum::Json;
use bridge_core::agent::AgentConfig;
use bridge_core::integration::IntegrationDefinition;
use bridge_core::mcp::McpServerDefinition;
use bridge_core::metrics::MetricsSnapshot;
use bridge_core::permission::ToolPermission;
use bridge_core::provider::ProviderType;
use bridge_core::skill::SkillDefinition;
use bridge_core::tool::ToolDefinition;
use bridge_core::BridgeError;
use serde::Serialize;

use crate::state::AppState;

/// Truncate a string to `max_len` chars, appending "..." if truncated.
fn truncate(s: &str, max_len: usize) -> String {
    if s.len() <= max_len {
        s.to_string()
    } else {
        let truncated: String = s.chars().take(max_len).collect();
        format!("{}...", truncated)
    }
}

/// Provider info with secrets redacted.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ProviderSummary {
    pub provider_type: ProviderType,
    pub model: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub base_url: Option<String>,
}

/// Subagent summary for API responses.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SubAgentSummary {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub system_prompt: String,
    pub provider: ProviderSummary,
    pub config: AgentConfig,
    pub tools: Vec<ToolDefinition>,
}

/// Full agent response — returned by both list and detail endpoints.
/// System prompts may be truncated on the list endpoint.
#[derive(Serialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentResponse {
    pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub system_prompt: String,
    pub provider: ProviderSummary,
    pub tools: Vec<ToolDefinition>,
    pub mcp_servers: Vec<McpServerDefinition>,
    pub skills: Vec<SkillDefinition>,
    pub integrations: Vec<IntegrationDefinition>,
    pub config: AgentConfig,
    pub subagents: Vec<SubAgentSummary>,
    pub permissions: HashMap<String, ToolPermission>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub webhook_url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<String>,
    pub active_conversations: usize,
    pub metrics: MetricsSnapshot,
}

/// Build an `AgentResponse` from an `AgentState`.
/// If `truncate_prompts` is true, system prompts are truncated to 100 chars.
async fn build_agent_response(
    agent: &runtime::AgentState,
    truncate_prompts: bool,
) -> AgentResponse {
    let def = agent.definition.read().await;
    let prompt_len = if truncate_prompts { 100 } else { usize::MAX };

    let provider = ProviderSummary {
        provider_type: def.provider.provider_type.clone(),
        model: def.provider.model.clone(),
        base_url: def.provider.base_url.clone(),
    };

    let subagents = def
        .subagents
        .iter()
        .map(|sa| SubAgentSummary {
            name: sa.name.clone(),
            description: sa.description.clone(),
            system_prompt: truncate(&sa.system_prompt, prompt_len),
            provider: ProviderSummary {
                provider_type: sa.provider.provider_type.clone(),
                model: sa.provider.model.clone(),
                base_url: sa.provider.base_url.clone(),
            },
            config: sa.config.clone(),
            tools: sa.tools.clone(),
        })
        .collect();

    let metrics = agent.metrics.snapshot(&def.id, &def.name);

    AgentResponse {
        id: def.id.clone(),
        name: def.name.clone(),
        description: def.description.clone(),
        system_prompt: truncate(&def.system_prompt, prompt_len),
        provider,
        tools: def.tools.clone(),
        mcp_servers: def.mcp_servers.clone(),
        skills: def.skills.clone(),
        integrations: def.integrations.clone(),
        config: def.config.clone(),
        subagents,
        permissions: def.permissions.clone(),
        webhook_url: def.webhook_url.clone(),
        version: def.version.clone(),
        updated_at: def.updated_at.clone(),
        active_conversations: agent.active_conversation_count(),
        metrics,
    }
}

/// GET /agents — list all loaded agents with full details (truncated system prompts).
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/agents",
    responses(
        (status = 200, description = "List of agents with full details", body = Vec<AgentResponse>)
    )
))]
pub async fn list_agents(State(state): State<AppState>) -> Json<Vec<AgentResponse>> {
    let agent_states = state.supervisor.list_agent_states();
    let mut responses = Vec::with_capacity(agent_states.len());
    for agent in &agent_states {
        responses.push(build_agent_response(agent, true).await);
    }
    Json(responses)
}

/// GET /agents/:agent_id — get full agent details.
#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/agents/{agent_id}",
    params(("agent_id" = String, Path, description = "Agent identifier")),
    responses(
        (status = 200, description = "Full agent details", body = AgentResponse),
        (status = 404, description = "Agent not found")
    )
))]
pub async fn get_agent(
    State(state): State<AppState>,
    Path(agent_id): Path<String>,
) -> Result<Json<AgentResponse>, BridgeError> {
    let agent = state
        .supervisor
        .get_agent(&agent_id)
        .ok_or_else(|| BridgeError::AgentNotFound(agent_id.clone()))?;

    Ok(Json(build_agent_response(&agent, false).await))
}
