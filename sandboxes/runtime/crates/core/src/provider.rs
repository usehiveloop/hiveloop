use serde::{Deserialize, Serialize};
use std::fmt;
use std::str::FromStr;

/// Supported LLM provider types.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ProviderType {
    /// OpenAI (GPT-4o, etc.)
    #[serde(rename = "open_ai")]
    OpenAI,
    /// Anthropic (Claude, etc.)
    Anthropic,
    /// Google (Gemini, etc.)
    Google,
    /// Groq
    Groq,
    /// DeepSeek
    DeepSeek,
    /// Mistral
    Mistral,
    /// Cohere
    Cohere,
    /// xAI (Grok, etc.)
    XAi,
    /// Together AI
    Together,
    /// Fireworks AI
    Fireworks,
    /// Ollama (local models)
    Ollama,
    /// Custom provider with custom base URL
    Custom,
}

impl fmt::Display for ProviderType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let s = match self {
            Self::OpenAI => "open_ai",
            Self::Anthropic => "anthropic",
            Self::Google => "google",
            Self::Groq => "groq",
            Self::DeepSeek => "deep_seek",
            Self::Mistral => "mistral",
            Self::Cohere => "cohere",
            Self::XAi => "x_ai",
            Self::Together => "together",
            Self::Fireworks => "fireworks",
            Self::Ollama => "ollama",
            Self::Custom => "custom",
        };
        write!(f, "{}", s)
    }
}

impl FromStr for ProviderType {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s {
            "open_ai" | "openai" => Ok(Self::OpenAI),
            "anthropic" => Ok(Self::Anthropic),
            "google" => Ok(Self::Google),
            "groq" => Ok(Self::Groq),
            "deep_seek" | "deepseek" => Ok(Self::DeepSeek),
            "mistral" => Ok(Self::Mistral),
            "cohere" => Ok(Self::Cohere),
            "x_ai" | "xai" => Ok(Self::XAi),
            "together" => Ok(Self::Together),
            "fireworks" => Ok(Self::Fireworks),
            "ollama" => Ok(Self::Ollama),
            "custom" => Ok(Self::Custom),
            _ => Err(format!("unknown provider type: {}", s)),
        }
    }
}

/// Configuration for an LLM provider.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
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
