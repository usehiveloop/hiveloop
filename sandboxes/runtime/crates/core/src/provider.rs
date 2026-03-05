use serde::{Deserialize, Serialize};

/// Supported LLM provider types.
#[derive(
    Debug,
    Clone,
    Serialize,
    Deserialize,
    PartialEq,
    Eq,
    strum_macros::Display,
    strum_macros::EnumString,
)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum ProviderType {
    /// OpenAI (GPT-4o, etc.)
    #[serde(rename = "open_ai")]
    #[strum(serialize = "open_ai", serialize = "openai")]
    OpenAI,
    /// Anthropic (Claude, etc.)
    #[strum(serialize = "anthropic")]
    Anthropic,
    /// Google (Gemini, etc.)
    #[strum(serialize = "google")]
    Google,
    /// Groq
    #[strum(serialize = "groq")]
    Groq,
    /// DeepSeek
    #[strum(serialize = "deep_seek", serialize = "deepseek")]
    DeepSeek,
    /// Mistral
    #[strum(serialize = "mistral")]
    Mistral,
    /// Cohere
    #[strum(serialize = "cohere")]
    Cohere,
    /// xAI (Grok, etc.)
    #[strum(serialize = "x_ai", serialize = "xai")]
    XAi,
    /// Together AI
    #[strum(serialize = "together")]
    Together,
    /// Fireworks AI
    #[strum(serialize = "fireworks")]
    Fireworks,
    /// Ollama (local models)
    #[strum(serialize = "ollama")]
    Ollama,
    /// Custom provider with custom base URL
    #[strum(serialize = "custom")]
    Custom,
}

/// Configuration for an LLM provider.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct ProviderConfig {
    /// The provider type (openai, anthropic, etc.)
    pub provider_type: ProviderType,
    /// Model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514")
    pub model: String,
    /// API key for authentication
    pub api_key: String,
    /// Optional custom endpoint URL
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_url: Option<String>,
}
