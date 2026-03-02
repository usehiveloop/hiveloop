use bridge_core::provider::{ProviderConfig, ProviderType};
use bridge_core::BridgeError;
use rig::prelude::CompletionClient;

/// Type alias for the OpenAI-compatible completion model used across the bridge.
pub type BridgeCompletionModel =
    <rig::providers::openai::CompletionsClient as CompletionClient>::CompletionModel;

/// Type alias for the agent type used across the bridge.
pub type BridgeAgent = rig::agent::Agent<BridgeCompletionModel>;

/// Type alias for the agent builder with no tools configured.
pub type BridgeAgentBuilder = rig::agent::AgentBuilder<BridgeCompletionModel>;

/// Type alias for the agent builder with tools configured.
pub type BridgeAgentBuilderWithTools =
    rig::agent::AgentBuilder<BridgeCompletionModel, (), rig::agent::WithBuilderTools>;

/// Create a rig-core agent builder for the given provider configuration.
///
/// All providers use the OpenAI-compatible completions API with appropriate
/// base URLs, since most LLM providers support this format.
pub fn create_agent_builder(config: &ProviderConfig) -> Result<BridgeAgentBuilder, BridgeError> {
    let base_url = resolve_base_url(config)?;

    let client = rig::providers::openai::CompletionsClient::builder()
        .api_key(&config.api_key)
        .base_url(&base_url)
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create client: {}", e)))?;

    Ok(client.agent(&config.model))
}

/// Resolve the base URL for a provider, using the custom base_url if provided
/// or falling back to the provider's default endpoint.
fn resolve_base_url(config: &ProviderConfig) -> Result<String, BridgeError> {
    if let Some(ref url) = config.base_url {
        return Ok(url.clone());
    }

    match config.provider_type {
        ProviderType::OpenAI => Ok("https://api.openai.com/v1".to_string()),
        ProviderType::Anthropic => Ok("https://api.anthropic.com/v1".to_string()),
        ProviderType::Google => Ok("https://generativelanguage.googleapis.com/v1beta".to_string()),
        ProviderType::Groq => Ok("https://api.groq.com/openai/v1".to_string()),
        ProviderType::DeepSeek => Ok("https://api.deepseek.com/v1".to_string()),
        ProviderType::Mistral => Ok("https://api.mistral.ai/v1".to_string()),
        ProviderType::Cohere => Ok("https://api.cohere.ai/v1".to_string()),
        ProviderType::XAi => Ok("https://api.x.ai/v1".to_string()),
        ProviderType::Together => Ok("https://api.together.xyz/v1".to_string()),
        ProviderType::Fireworks => Ok("https://api.fireworks.ai/inference/v1".to_string()),
        ProviderType::Ollama => Ok("http://localhost:11434/v1".to_string()),
        ProviderType::Custom => Err(BridgeError::ConfigError(
            "custom provider requires base_url".to_string(),
        )),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_create_agent_builder_openai() {
        let config = ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
        };
        let result = create_agent_builder(&config);
        assert!(result.is_ok());
    }

    #[test]
    fn test_create_agent_builder_custom_no_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::Custom,
            model: "custom-model".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
        };
        let result = create_agent_builder(&config);
        assert!(result.is_err());
    }

    #[test]
    fn test_create_agent_builder_custom_with_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::Custom,
            model: "custom-model".to_string(),
            api_key: "test-key".to_string(),
            base_url: Some("http://localhost:8000/v1".to_string()),
        };
        let result = create_agent_builder(&config);
        assert!(result.is_ok());
    }
}
