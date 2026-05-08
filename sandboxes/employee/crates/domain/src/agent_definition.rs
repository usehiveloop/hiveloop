use serde::{Deserialize, Serialize};

use crate::{
    mcp_specs::McpSpec, model_config::ModelConfig, outbound::OutboundChannelSpec,
    skill_specs::SkillSpec, skill_specs::SubagentSpec, slack_settings::SlackConfig,
    tool_specs::ToolSpec,
};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentDefinition {
    pub agent: AgentMeta,
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
pub struct AgentMeta {
    pub name: String,
    #[serde(default)]
    pub description: String,
    pub system_prompt: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Limits {
    pub max_turns_per_session: u32,
    pub max_tool_calls_per_turn: u32,
    pub max_session_duration_seconds: u32,
    pub input_token_budget: u32,
    pub output_token_budget: u32,
    pub tool_call_timeout_seconds: u32,
    pub subagent_max_depth: u32,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            max_turns_per_session: 50,
            max_tool_calls_per_turn: 20,
            max_session_duration_seconds: 3600,
            input_token_budget: 180_000,
            output_token_budget: 8_000,
            tool_call_timeout_seconds: 60,
            subagent_max_depth: 2,
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ContextConfig {
    #[serde(default)]
    pub max_history_events: Option<u32>,
    #[serde(default)]
    pub compaction: Option<CompactionConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CompactionConfig {
    pub enabled: bool,
    pub trigger_token_threshold: u32,
    pub summarizer_model: String,
    pub keep_last_n_turns: u32,
}

