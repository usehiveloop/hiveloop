use pretty_assertions::assert_eq;

use crate::provider::{ProviderConfig, ProviderType};

#[test]
fn provider_type_all_variants_serialize_to_snake_case() {
    let cases = vec![
        (ProviderType::OpenAI, "\"open_ai\""),
        (ProviderType::Anthropic, "\"anthropic\""),
        (ProviderType::Google, "\"google\""),
        (ProviderType::Groq, "\"groq\""),
        (ProviderType::DeepSeek, "\"deep_seek\""),
        (ProviderType::Mistral, "\"mistral\""),
        (ProviderType::Cohere, "\"cohere\""),
        (ProviderType::XAi, "\"x_ai\""),
        (ProviderType::Together, "\"together\""),
        (ProviderType::Fireworks, "\"fireworks\""),
        (ProviderType::Ollama, "\"ollama\""),
        (ProviderType::Custom, "\"custom\""),
    ];

    for (variant, expected_json) in cases {
        let json = serde_json::to_string(&variant).expect("serialize ProviderType");
        assert_eq!(
            json, expected_json,
            "ProviderType::{:?} serialization",
            variant
        );
    }
}

#[test]
fn provider_type_all_variants_roundtrip() {
    let variants = vec![
        ProviderType::OpenAI,
        ProviderType::Anthropic,
        ProviderType::Google,
        ProviderType::Groq,
        ProviderType::DeepSeek,
        ProviderType::Mistral,
        ProviderType::Cohere,
        ProviderType::XAi,
        ProviderType::Together,
        ProviderType::Fireworks,
        ProviderType::Ollama,
        ProviderType::Custom,
    ];

    for variant in variants {
        let json = serde_json::to_string(&variant).expect("serialize");
        let deserialized: ProviderType = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(
            variant, deserialized,
            "ProviderType roundtrip for {:?}",
            variant
        );
    }
}

#[test]
fn provider_type_display_matches_serde() {
    let variants = vec![
        ProviderType::OpenAI,
        ProviderType::Anthropic,
        ProviderType::Google,
        ProviderType::Groq,
        ProviderType::DeepSeek,
        ProviderType::Mistral,
        ProviderType::Cohere,
        ProviderType::XAi,
        ProviderType::Together,
        ProviderType::Fireworks,
        ProviderType::Ollama,
        ProviderType::Custom,
    ];

    for variant in variants {
        let display = format!("{}", variant);
        // Display output should be the same as the JSON value without quotes
        let json = serde_json::to_string(&variant).expect("serialize");
        let json_unquoted = json.trim_matches('"');
        assert_eq!(
            display, json_unquoted,
            "Display and serde should match for {:?}",
            variant
        );
    }
}

#[test]
fn provider_type_from_str_accepts_aliases() {
    use std::str::FromStr;

    assert_eq!(
        ProviderType::from_str("openai").unwrap(),
        ProviderType::OpenAI
    );
    assert_eq!(
        ProviderType::from_str("open_ai").unwrap(),
        ProviderType::OpenAI
    );
    assert_eq!(
        ProviderType::from_str("deepseek").unwrap(),
        ProviderType::DeepSeek
    );
    assert_eq!(
        ProviderType::from_str("deep_seek").unwrap(),
        ProviderType::DeepSeek
    );
    assert_eq!(ProviderType::from_str("xai").unwrap(), ProviderType::XAi);
    assert_eq!(ProviderType::from_str("x_ai").unwrap(), ProviderType::XAi);
}

#[test]
fn provider_type_from_str_rejects_unknown() {
    use std::str::FromStr;
    assert!(ProviderType::from_str("nonexistent").is_err());
}

// ──────────────────────────────────────────────
// ProviderConfig
// ──────────────────────────────────────────────

#[test]
fn provider_config_roundtrip_with_base_url() {
    let config = ProviderConfig {
        provider_type: ProviderType::Custom,
        model: "custom-model".to_string(),
        api_key: "custom-key".to_string(),
        base_url: Some("https://custom.api.com/v1".to_string()),
    };

    let json = serde_json::to_string_pretty(&config).expect("serialize");
    let deserialized: ProviderConfig = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(config, deserialized);
}

#[test]
fn provider_config_roundtrip_without_base_url() {
    let config = ProviderConfig {
        provider_type: ProviderType::Anthropic,
        model: "claude-sonnet-4-20250514".to_string(),
        api_key: "<anthropic-api-key>".to_string(),
        base_url: None,
    };

    let json = serde_json::to_string_pretty(&config).expect("serialize");
    assert!(!json.contains("base_url"));
    let deserialized: ProviderConfig = serde_json::from_str(&json).expect("deserialize");
    assert_eq!(config, deserialized);
}

// ──────────────────────────────────────────────
// McpTransport
// ──────────────────────────────────────────────
