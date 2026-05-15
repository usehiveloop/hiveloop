use serde::{Deserialize, Serialize};

use crate::{
    mcp_specs::McpSpec, model_config::ModelConfig, outbound::OutboundChannelSpec,
    skill_specs::SkillSpec, skill_specs::SubagentSpec, slack_settings::SlackConfig,
    tool_specs::ToolSpec,
};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentDefinition {
    pub agent: AgentMeta,
    #[serde(default)]
    pub prompt_fragments: PromptFragments,
    pub model: ModelConfig,
    #[serde(default)]
    pub multimodal_model: Option<ModelConfig>,
    #[serde(default)]
    pub limits: Limits,
    #[serde(default)]
    pub context: ContextConfig,
    #[serde(default)]
    pub tools: Vec<ToolSpec>,
    #[serde(default)]
    pub mcp_servers: Vec<McpSpec>,
    #[serde(default)]
    pub skills: Vec<SkillSpec>,
    #[serde(default)]
    pub subagents: Vec<SubagentSpec>,
    #[serde(default)]
    pub slack: SlackConfig,
    #[serde(default)]
    pub outbound_channels: Vec<OutboundChannelSpec>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentMeta {
    pub name: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub system_prompt: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PromptFragments {
    #[serde(default)]
    pub identity: PromptFragment,
    #[serde(default)]
    pub company: PromptFragment,
    #[serde(default)]
    pub team: PromptFragment,
    #[serde(default)]
    pub operating_principles: PromptFragment,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct PromptFragment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub content: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct Limits {
    pub max_turns_per_session: u32,
    pub input_token_budget: u32,
    pub output_token_budget: u32,
    pub tool_call_timeout_seconds: u32,
    pub subagent_max_depth: u32,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            max_turns_per_session: 50,
            input_token_budget: 180_000,
            output_token_budget: 8_000,
            tool_call_timeout_seconds: 60,
            subagent_max_depth: 2,
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ContextConfig {
    #[serde(default)]
    pub max_history_events: Option<u32>,
    #[serde(default)]
    pub compaction: Option<CompactionConfig>,
    #[serde(default)]
    pub memory: MemoryContextConfig,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct MemoryContextConfig {
    #[serde(default)]
    pub entries: Vec<MemoryContextEntry>,
    #[serde(default = "default_memory_token_budget")]
    pub token_budget: u32,
}

impl Default for MemoryContextConfig {
    fn default() -> Self {
        Self {
            entries: Vec::new(),
            token_budget: default_memory_token_budget(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct MemoryContextEntry {
    pub content: String,
    #[serde(default)]
    pub memory_type: String,
    #[serde(default)]
    pub source: String,
    #[serde(default)]
    pub confidence: Option<f32>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CompactionConfig {
    pub enabled: bool,
    pub token_threshold: u32,
    #[serde(default = "default_overlap")]
    pub overlap_event_count: u32,
    #[serde(default = "default_chars_per_token")]
    pub chars_per_token: u32,
    pub summarizer_model: ModelConfig,
}

fn default_overlap() -> u32 {
    10
}
fn default_chars_per_token() -> u32 {
    4
}
fn default_memory_token_budget() -> u32 {
    1000
}
