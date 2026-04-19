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

/// Prompt-cache TTL hint.
///
/// Anthropic supports two TTLs today: 5-minute ephemeral (default,
/// 1.25× cache-write multiplier) and 1-hour extended (2.0× write, needs
/// `extended-cache-ttl-2025-04-11` beta header). Other providers ignore
/// this field — their caches are automatic and provider-managed.
///
/// **Note (rig 0.31)**: `with_prompt_caching()` only emits the
/// `{"type": "ephemeral"}` breakpoint, which corresponds to the 5-minute
/// TTL. Setting `OneHour` here sends the beta header but does not yet
/// add `"ttl": "1h"` to the cache_control block — upgrade rig to get
/// true 1-hour writes. The field is wired now so the choice survives
/// the upgrade.
#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, PartialEq, Eq)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "snake_case")]
pub enum CacheTtl {
    /// Default 5-minute ephemeral cache.
    #[default]
    FiveMinutes,
    /// Extended 1-hour cache — sends beta header when supported.
    OneHour,
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
    /// Whether to enable explicit prompt-cache breakpoints for providers
    /// that require them (Anthropic today; MiniMax's Anthropic-shape
    /// endpoint tomorrow). Implicit-cache providers (OpenAI, GLM, Gemini
    /// 2.5 implicit, Kimi K2) ignore this — they cache unconditionally.
    #[serde(default = "default_true")]
    pub prompt_caching_enabled: bool,
    /// TTL for explicit cache breakpoints. See [`CacheTtl`].
    #[serde(default)]
    pub cache_ttl: CacheTtl,
}

fn default_true() -> bool {
    true
}
