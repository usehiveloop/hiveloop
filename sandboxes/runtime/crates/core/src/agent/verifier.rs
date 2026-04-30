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
    // Wire format matches `core::ProviderType::OpenAI` ("open_ai") rather
    // than the serde-default `"open_a_i"`. Without this override snake_case
    // splits on every capital letter (O p e n _A _I). Keep aligned with
    // `crates/core/src/provider.rs` so callers spell OpenAI the same way
    // everywhere in bridge configs.
    #[serde(rename = "open_ai")]
    OpenAI,
    Gemini,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn verifier_provider_openai_serializes_as_open_ai() {
        assert_eq!(
            serde_json::to_string(&VerifierProvider::OpenAI).unwrap(),
            "\"open_ai\""
        );
    }

    #[test]
    fn verifier_provider_openai_deserializes_from_open_ai() {
        let p: VerifierProvider = serde_json::from_str("\"open_ai\"").unwrap();
        assert_eq!(p, VerifierProvider::OpenAI);
    }

    #[test]
    fn verifier_provider_openai_rejects_legacy_open_a_i() {
        let r: Result<VerifierProvider, _> = serde_json::from_str("\"open_a_i\"");
        assert!(r.is_err(), "expected legacy spelling to fail");
    }

    #[test]
    fn verifier_provider_gemini_unchanged() {
        assert_eq!(
            serde_json::to_string(&VerifierProvider::Gemini).unwrap(),
            "\"gemini\""
        );
    }
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
