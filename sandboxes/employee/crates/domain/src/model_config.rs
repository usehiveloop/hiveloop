use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ReasoningEffort {
    Low,
    Medium,
    High,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
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
        fallback: Option<Box<ModelConfig>>,
    },
}
