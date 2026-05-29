use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::{
    mcp_specs::McpSpec, model_config::ModelConfig, model_config::SafetyConfig,
    outbound::OutboundChannelSpec, skill_specs::SkillSpec, tool_specs::ToolSpec,
};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentDefinition {
    pub agent: AgentMeta,
    #[serde(default)]
    pub mode: RuntimeMode,
    #[serde(default)]
    pub specialist_profile: Option<SpecialistProfile>,
    #[serde(default)]
    pub system_prompt: SystemPromptConfig,
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
    pub outbound_channels: Vec<OutboundChannelSpec>,
    #[serde(default)]
    pub sub_agents: HashMap<String, AgentDefinition>,
    #[serde(default)]
    pub safety: SafetyConfig,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum RuntimeMode {
    #[default]
    Employee,
    Specialist,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SpecialistProfile {
    pub name: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct AgentMeta {
    pub name: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct SystemPromptConfig {
    #[serde(default)]
    pub cacheable_segments: Vec<SystemPromptSegment>,
    #[serde(default)]
    pub dynamic_segments: Vec<SystemPromptSegment>,
}

impl SystemPromptConfig {
    pub fn validate(&self) -> Result<(), String> {
        const MAX_SEGMENTS: usize = 64;
        const MAX_TEXT_CHARS: usize = 64 * 1024;
        if self.cacheable_segments.len() > MAX_SEGMENTS {
            return Err("too many cacheable prompt segments".to_string());
        }
        if self.dynamic_segments.len() > MAX_SEGMENTS {
            return Err("too many dynamic prompt segments".to_string());
        }
        for segment in self
            .cacheable_segments
            .iter()
            .chain(self.dynamic_segments.iter())
        {
            segment.validate(MAX_TEXT_CHARS)?;
        }
        for segment in &self.cacheable_segments {
            if !matches!(segment, SystemPromptSegment::StaticText(_)) {
                return Err("cacheable prompt segments must be static_text".to_string());
            }
        }
        Ok(())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", content = "config", rename_all = "snake_case")]
pub enum SystemPromptSegment {
    StaticText(StaticPromptSegment),
    DynamicContext(DynamicContextPromptSegment),
    MemoryContext(MemoryPromptSegment),
    SkillCatalog(ListPromptSegment),
    McpTools(ListPromptSegment),
}

impl SystemPromptSegment {
    fn validate(&self, max_text_chars: usize) -> Result<(), String> {
        match self {
            SystemPromptSegment::StaticText(segment) => {
                validate_prompt_text("static_text.title", &segment.title, max_text_chars)?;
                validate_prompt_text("static_text.content", &segment.content, max_text_chars)?;
            }
            SystemPromptSegment::DynamicContext(segment) => {
                validate_prompt_text("dynamic_context.title", &segment.title, max_text_chars)?;
                validate_prompt_text(
                    "dynamic_context.preamble",
                    &segment.preamble,
                    max_text_chars,
                )?;
                validate_prompt_text(
                    "dynamic_context.item_template",
                    &segment.item_template,
                    max_text_chars,
                )?;
            }
            SystemPromptSegment::MemoryContext(segment) => {
                validate_prompt_text("memory_context.title", &segment.title, max_text_chars)?;
                validate_prompt_text("memory_context.preamble", &segment.preamble, max_text_chars)?;
                validate_prompt_text(
                    "memory_context.open_wrapper",
                    &segment.open_wrapper,
                    max_text_chars,
                )?;
                validate_prompt_text(
                    "memory_context.close_wrapper",
                    &segment.close_wrapper,
                    max_text_chars,
                )?;
                validate_prompt_text(
                    "memory_context.item_template",
                    &segment.item_template,
                    max_text_chars,
                )?;
            }
            SystemPromptSegment::SkillCatalog(segment) | SystemPromptSegment::McpTools(segment) => {
                validate_prompt_text("list.title", &segment.title, max_text_chars)?;
                validate_prompt_text("list.preamble", &segment.preamble, max_text_chars)?;
                validate_prompt_text("list.item_template", &segment.item_template, max_text_chars)?;
            }
        }
        Ok(())
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct StaticPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub content: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct DynamicContextPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub preamble: String,
    #[serde(default = "default_context_item_template")]
    pub item_template: String,
}

impl Default for DynamicContextPromptSegment {
    fn default() -> Self {
        Self {
            title: String::new(),
            preamble: String::new(),
            item_template: default_context_item_template(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct MemoryPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub preamble: String,
    #[serde(default)]
    pub open_wrapper: String,
    #[serde(default)]
    pub close_wrapper: String,
    #[serde(default = "default_memory_item_template")]
    pub item_template: String,
}

impl Default for MemoryPromptSegment {
    fn default() -> Self {
        Self {
            title: String::new(),
            preamble: String::new(),
            open_wrapper: String::new(),
            close_wrapper: String::new(),
            item_template: default_memory_item_template(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ListPromptSegment {
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub preamble: String,
    #[serde(default = "default_list_item_template")]
    pub item_template: String,
}

impl Default for ListPromptSegment {
    fn default() -> Self {
        Self {
            title: String::new(),
            preamble: String::new(),
            item_template: default_list_item_template(),
        }
    }
}

fn validate_prompt_text(label: &str, value: &str, max_text_chars: usize) -> Result<(), String> {
    if value.len() > max_text_chars {
        return Err(format!("{label} is too large"));
    }
    Ok(())
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct Limits {
    pub max_turns_per_session: u32,
    pub input_token_budget: u32,
    pub output_token_budget: u32,
    pub tool_call_timeout_seconds: u32,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            max_turns_per_session: 5_000,
            input_token_budget: 180_000,
            output_token_budget: 8_000,
            tool_call_timeout_seconds: 60,
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
    #[serde(default)]
    pub token_threshold: Option<u32>,
    #[serde(default)]
    pub token_threshold_percentage: Option<f64>,
    #[serde(default)]
    pub turn_threshold: Option<u32>,
    #[serde(default)]
    pub message_threshold: Option<u32>,
    #[serde(default = "default_eviction")]
    pub eviction_window: f64,
    #[serde(default)]
    pub retention_window: u32,
    #[serde(default = "default_overlap")]
    pub overlap_event_count: u32,
    #[serde(default = "default_chars_per_token")]
    pub chars_per_token: u32,
    #[serde(default)]
    pub on_turn_end: Option<bool>,
}

fn default_eviction() -> f64 {
    0.2
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
fn default_context_item_template() -> String {
    "{content}".to_string()
}
fn default_memory_item_template() -> String {
    "- {line}".to_string()
}
fn default_list_item_template() -> String {
    "- {name}: {description}".to_string()
}
