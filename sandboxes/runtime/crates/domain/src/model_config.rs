use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SafetyConfig {
    #[serde(default = "default_true")]
    pub xml_tool_repair: bool,
    #[serde(default = "default_true")]
    pub thinking_strip: bool,
    #[serde(default)]
    pub overthinking: OverthinkingConfig,
    #[serde(default)]
    pub repeat_detection: RepeatDetectionConfig,
    #[serde(default = "default_true")]
    pub json_repair: bool,
}

impl Default for SafetyConfig {
    fn default() -> Self {
        Self {
            xml_tool_repair: true,
            thinking_strip: true,
            overthinking: OverthinkingConfig::default(),
            repeat_detection: RepeatDetectionConfig::default(),
            json_repair: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OverthinkingConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_120")]
    pub max_duration_secs: u64,
    #[serde(default = "default_8192")]
    pub max_tokens: u64,
    #[serde(default = "default_500")]
    pub stall_threshold: u64,
}

impl Default for OverthinkingConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            max_duration_secs: 120,
            max_tokens: 8192,
            stall_threshold: 500,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RepeatDetectionConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_3")]
    pub max_consecutive: usize,
    #[serde(default = "default_5")]
    pub max_total: usize,
}

impl Default for RepeatDetectionConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            max_consecutive: 3,
            max_total: 5,
        }
    }
}

fn default_true() -> bool {
    true
}

fn default_120() -> u64 {
    120
}

fn default_8192() -> u64 {
    8192
}

fn default_500() -> u64 {
    500
}

fn default_3() -> usize {
    3
}

fn default_5() -> usize {
    5
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "lowercase")]
pub enum ReasoningEffort {
    Low,
    Medium,
    High,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "provider", rename_all = "snake_case")]
pub enum ModelConfig {
    OpenaiCompatible {
        base_url: String,
        model_id: String,
        api_key_env: String,
        #[serde(default)]
        temperature: Option<f32>,
        #[serde(default)]
        max_output_tokens: Option<u32>,
        #[serde(default)]
        reasoning_effort: Option<ReasoningEffort>,
        #[serde(default)]
        extra_headers: HashMap<String, String>,
        #[serde(default)]
        #[cfg_attr(feature = "openapi", schema(no_recursion))]
        fallback: Option<Box<ModelConfig>>,
    },
}
