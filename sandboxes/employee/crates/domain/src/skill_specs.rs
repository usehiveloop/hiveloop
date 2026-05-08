use serde::{Deserialize, Serialize};

use crate::{model_config::ModelConfig, tool_specs::ToolSpec, agent_definition::Limits};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SkillSpec {
    pub name: String,
    pub description: String,
    pub trigger: SkillTrigger,
    pub instructions: String,
    #[serde(default)]
    pub files: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum SkillTrigger {
    Always,
    Keyword { patterns: Vec<String> },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SubagentSpec {
    pub name: String,
    pub description: String,
    #[serde(default = "default_expose_as_tool")]
    pub expose_as_tool: bool,
    pub tool_name: String,
    pub tool_description: String,
    pub definition: SubagentDefinition,
}

fn default_expose_as_tool() -> bool {
    true
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SubagentDefinition {
    pub system_prompt: String,
    pub model: ModelConfig,
    #[serde(default)]
    pub tools: Vec<ToolSpec>,
    #[serde(default)]
    pub mcp_inherit: Vec<String>,
    #[serde(default)]
    pub limits: Limits,
}
