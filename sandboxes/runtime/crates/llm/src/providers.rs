use bridge_core::provider::ProviderConfig;
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
/// All providers use the OpenAI-compatible completions API.
/// The `base_url` must be explicitly set in the agent definition.
pub fn create_agent_builder(config: &ProviderConfig) -> Result<BridgeAgentBuilder, BridgeError> {
    let base_url = config.base_url.as_deref().ok_or_else(|| {
        BridgeError::ConfigError(format!(
            "provider '{}' requires base_url to be set in the agent definition",
            config.provider_type
        ))
    })?;

    let client = rig::providers::openai::CompletionsClient::builder()
        .api_key(&config.api_key)
        .base_url(base_url)
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create client: {}", e)))?;

    Ok(client.agent(&config.model))
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::provider::ProviderType;

    #[test]
    fn test_create_agent_builder_with_base_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "test-key".to_string(),
            base_url: Some("https://api.openai.com/v1".to_string()),
        };
        let result = create_agent_builder(&config);
        assert!(result.is_ok());
    }

    #[test]
    fn test_create_agent_builder_missing_base_url_errors() {
        let config = ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
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
