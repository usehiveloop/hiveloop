use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct VerifierAgentConfig {
    #[serde(default)]
    pub enabled: bool,

    pub primary: VerifierModel,

    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub fallback: Option<VerifierModel>,

    #[serde(default = "default_max_reprompts")]
    pub max_reprompts_per_turn: u32,

    #[serde(default = "default_true")]
    pub require_high_confidence: bool,

    #[serde(default = "default_true")]
    pub blocking: bool,

    #[serde(default = "default_max_input_tokens")]
    pub max_input_tokens: u32,

    #[serde(default = "default_timeout_ms")]
    pub timeout_ms: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct VerifierModel {
    pub provider: VerifierProvider,
    pub model: String,
    pub api_key: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_url: Option<String>,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum VerifierProvider {
    OpenAI,
    Gemini,
}

fn default_max_reprompts() -> u32 {
    2
}
fn default_true() -> bool {
    true
}
fn default_max_input_tokens() -> u32 {
    10_000
}
fn default_timeout_ms() -> u32 {
    5_000
}
