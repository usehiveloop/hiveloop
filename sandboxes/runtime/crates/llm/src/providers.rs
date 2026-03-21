use bridge_core::provider::{ProviderConfig, ProviderType};
use bridge_core::BridgeError;
use rig::agent::Agent;
use rig::completion::{CompletionModel, Prompt, PromptError};
use rig::message::Message;
use rig::prelude::CompletionClient;
use tracing::{error, info};

use crate::tool_hook::ToolCallEmitter;

// ---------------------------------------------------------------------------
// Per-provider completion-model type aliases
// ---------------------------------------------------------------------------
type OpenAIModel = <rig::providers::openai::CompletionsClient as CompletionClient>::CompletionModel;
type AnthropicModel = <rig::providers::anthropic::Client as CompletionClient>::CompletionModel;
type GeminiModel = <rig::providers::gemini::Client as CompletionClient>::CompletionModel;
type CohereModel = <rig::providers::cohere::Client as CompletionClient>::CompletionModel;

// ---------------------------------------------------------------------------
// BridgeAgent — enum over all supported provider agents
// ---------------------------------------------------------------------------

/// Unified agent type supporting multiple LLM providers.
///
/// Each variant wraps a rig-core `Agent<M>` for the corresponding provider's
/// native completion model. The enum delegates prompt calls to the inner
/// agent, producing a unified `Result<PromptResponse, PromptError>`.
#[derive(Clone)]
pub enum BridgeAgent {
    OpenAI(Agent<OpenAIModel>),
    Anthropic(Agent<AnthropicModel>),
    Gemini(Agent<GeminiModel>),
    Cohere(Agent<CohereModel>),
}

/// Response from a prompt with extended details (token usage).
pub struct PromptResponse {
    pub output: String,
    pub total_usage: rig::completion::Usage,
}

/// Helper macro to dispatch a prompt chain across all enum variants.
///
/// Each variant produces the same `Result<PromptResponse, PromptError>` so
/// the call sites never need to know which provider is backing the agent.
macro_rules! dispatch_prompt {
    ($agent_variant:expr, $text:expr, $history:expr, $hook:expr) => {{
        let resp = $agent_variant
            .prompt($text)
            .extended_details()
            .with_history($history)
            .with_hook($hook)
            .await?;
        Ok(PromptResponse {
            output: resp.output,
            total_usage: resp.total_usage,
        })
    }};
}

macro_rules! dispatch_prompt_simple {
    ($agent_variant:expr, $text:expr) => {{
        $agent_variant.prompt($text).await
    }};
}

impl BridgeAgent {
    /// Return the provider name for logging.
    fn provider_name(&self) -> &'static str {
        match self {
            BridgeAgent::OpenAI(_) => "openai",
            BridgeAgent::Anthropic(_) => "anthropic",
            BridgeAgent::Gemini(_) => "gemini",
            BridgeAgent::Cohere(_) => "cohere",
        }
    }

    /// Run a prompt with history and a tool-call hook, returning extended
    /// details (output text + token usage).
    ///
    /// This is the primary entry point used by the conversation loop.
    pub async fn prompt_with_hook(
        &self,
        text: &str,
        history: &mut Vec<Message>,
        hook: ToolCallEmitter,
    ) -> Result<PromptResponse, PromptError> {
        let provider = self.provider_name();
        // Borrow for the start log, clone for post-dispatch log (hook is moved into dispatch).
        info!(
            agent_id = %hook.agent_id,
            conversation_id = %hook.conversation_id,
            provider = provider,
            history_len = history.len(),
            "llm_request_start"
        );
        let agent_id = hook.agent_id.clone();
        let conversation_id = hook.conversation_id.clone();

        let start = std::time::Instant::now();
        let result = match self {
            BridgeAgent::OpenAI(a) => dispatch_prompt!(a, text, history, hook),
            BridgeAgent::Anthropic(a) => dispatch_prompt!(a, text, history, hook),
            BridgeAgent::Gemini(a) => dispatch_prompt!(a, text, history, hook),
            BridgeAgent::Cohere(a) => dispatch_prompt!(a, text, history, hook),
        };
        let latency_ms = start.elapsed().as_millis() as u64;

        match &result {
            Ok(resp) => {
                info!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    provider = provider,
                    input_tokens = resp.total_usage.input_tokens,
                    output_tokens = resp.total_usage.output_tokens,
                    latency_ms = latency_ms,
                    "llm_request_complete"
                );
            }
            Err(e) => {
                error!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    provider = provider,
                    error = %e,
                    latency_ms = latency_ms,
                    "llm_request_failed"
                );
            }
        }

        result
    }

    /// Run a prompt with history and a tool-call hook, returning just the text output.
    ///
    /// Used by the subagent runner.
    pub async fn prompt_standard_with_hook(
        &self,
        text: &str,
        history: &mut Vec<Message>,
        hook: ToolCallEmitter,
    ) -> Result<String, PromptError> {
        let provider = self.provider_name();
        info!(
            agent_id = %hook.agent_id,
            conversation_id = %hook.conversation_id,
            provider = provider,
            "llm_request_start"
        );
        let agent_id = hook.agent_id.clone();
        let conversation_id = hook.conversation_id.clone();

        let start = std::time::Instant::now();
        macro_rules! dispatch {
            ($agent:expr) => {{
                $agent
                    .prompt(text)
                    .with_history(history)
                    .with_hook(hook)
                    .await
            }};
        }
        let result = match self {
            BridgeAgent::OpenAI(a) => dispatch!(a),
            BridgeAgent::Anthropic(a) => dispatch!(a),
            BridgeAgent::Gemini(a) => dispatch!(a),
            BridgeAgent::Cohere(a) => dispatch!(a),
        };
        let latency_ms = start.elapsed().as_millis() as u64;

        match &result {
            Ok(_) => {
                info!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    provider = provider,
                    latency_ms = latency_ms,
                    "llm_request_complete"
                );
            }
            Err(e) => {
                error!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    provider = provider,
                    error = %e,
                    latency_ms = latency_ms,
                    "llm_request_failed"
                );
            }
        }

        result
    }

    /// Prompt with history but no hooks. Returns just the text output.
    ///
    /// Used by the no-tools retry agent.
    pub async fn prompt_with_history(
        &self,
        text: &str,
        history: &mut Vec<Message>,
    ) -> Result<String, PromptError> {
        let provider = self.provider_name();
        info!(provider = provider, "llm_request_start");

        let start = std::time::Instant::now();
        macro_rules! dispatch {
            ($agent:expr) => {{
                $agent.prompt(text).with_history(history).await
            }};
        }
        let result = match self {
            BridgeAgent::OpenAI(a) => dispatch!(a),
            BridgeAgent::Anthropic(a) => dispatch!(a),
            BridgeAgent::Gemini(a) => dispatch!(a),
            BridgeAgent::Cohere(a) => dispatch!(a),
        };
        let latency_ms = start.elapsed().as_millis() as u64;

        match &result {
            Ok(_) => info!(
                provider = provider,
                latency_ms = latency_ms,
                "llm_request_complete"
            ),
            Err(e) => {
                error!(provider = provider, error = %e, latency_ms = latency_ms, "llm_request_failed")
            }
        }

        result
    }

    /// Simple prompt without hooks or history. Returns just the text output.
    ///
    /// Used by the compaction summarizer.
    pub async fn prompt_simple(&self, text: &str) -> Result<String, PromptError> {
        let provider = self.provider_name();
        info!(provider = provider, "llm_request_start");

        let start = std::time::Instant::now();
        let result = match self {
            BridgeAgent::OpenAI(a) => dispatch_prompt_simple!(a, text),
            BridgeAgent::Anthropic(a) => dispatch_prompt_simple!(a, text),
            BridgeAgent::Gemini(a) => dispatch_prompt_simple!(a, text),
            BridgeAgent::Cohere(a) => dispatch_prompt_simple!(a, text),
        };
        let latency_ms = start.elapsed().as_millis() as u64;

        match &result {
            Ok(_) => info!(
                provider = provider,
                latency_ms = latency_ms,
                "llm_request_complete"
            ),
            Err(e) => {
                error!(provider = provider, error = %e, latency_ms = latency_ms, "llm_request_failed")
            }
        }

        result
    }
}

// ---------------------------------------------------------------------------
// Provider client construction
// ---------------------------------------------------------------------------

/// Build a `BridgeAgent` for the given provider configuration and tools.
///
/// Dispatches on `provider_type` to instantiate the correct native rig client
/// (OpenAI, Anthropic, Gemini, Cohere) and wraps the resulting agent in the
/// corresponding enum variant. OpenAI-compatible providers (Groq, DeepSeek,
/// Mistral, xAI, Together, Fireworks, Ollama, Custom) all use the OpenAI
/// client with a custom base_url.
pub fn create_agent(
    config: &ProviderConfig,
    tools: Vec<crate::tool_adapter::DynamicTool>,
    preamble: &str,
    definition: &bridge_core::agent::AgentDefinition,
) -> Result<BridgeAgent, BridgeError> {
    match config.provider_type {
        // Native Anthropic client
        ProviderType::Anthropic => {
            let client = build_anthropic_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            Ok(BridgeAgent::Anthropic(agent))
        }
        // Native Gemini client
        ProviderType::Google => {
            let client = build_gemini_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            Ok(BridgeAgent::Gemini(agent))
        }
        // Native Cohere client
        ProviderType::Cohere => {
            let client = build_cohere_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            Ok(BridgeAgent::Cohere(agent))
        }
        // OpenAI + all OpenAI-compatible providers
        ProviderType::OpenAI
        | ProviderType::Groq
        | ProviderType::DeepSeek
        | ProviderType::Mistral
        | ProviderType::XAi
        | ProviderType::Together
        | ProviderType::Fireworks
        | ProviderType::Ollama
        | ProviderType::Custom => {
            let client = build_openai_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            Ok(BridgeAgent::OpenAI(agent))
        }
    }
}

// ---------------------------------------------------------------------------
// Generic builder configuration (works for any CompletionModel)
// ---------------------------------------------------------------------------

/// Apply preamble, temperature, max_tokens, max_turns, and tools to an agent
/// builder of any provider type.
fn configure_and_build<M: CompletionModel>(
    builder: rig::agent::AgentBuilder<M>,
    preamble: &str,
    definition: &bridge_core::agent::AgentDefinition,
    tools: Vec<crate::tool_adapter::DynamicTool>,
) -> Agent<M> {
    let builder = builder.preamble(preamble);

    let builder = if let Some(temp) = definition.config.temperature {
        builder.temperature(temp)
    } else {
        builder
    };

    let builder = if let Some(max_tokens) = definition.config.max_tokens {
        builder.max_tokens(max_tokens as u64)
    } else {
        builder
    };

    let builder = if let Some(max_turns) = definition.config.max_turns {
        builder.default_max_turns(max_turns as usize)
    } else {
        builder
    };

    if tools.is_empty() {
        builder.build()
    } else {
        let mut iter = tools.into_iter();
        let first = iter.next().expect("checked non-empty above");
        let mut builder = builder.tool(first);
        for tool in iter {
            builder = builder.tool(tool);
        }
        builder.build()
    }
}

// ---------------------------------------------------------------------------
// Per-provider client builders
// ---------------------------------------------------------------------------

fn require_base_url(config: &ProviderConfig) -> Result<&str, BridgeError> {
    config.base_url.as_deref().ok_or_else(|| {
        BridgeError::ConfigError(format!(
            "provider '{}' requires base_url to be set in the agent definition",
            config.provider_type
        ))
    })
}

fn build_openai_client(
    config: &ProviderConfig,
) -> Result<rig::providers::openai::CompletionsClient, BridgeError> {
    let base_url = require_base_url(config)?;
    rig::providers::openai::CompletionsClient::builder()
        .api_key(&config.api_key)
        .base_url(base_url)
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create OpenAI client: {}", e)))
}

fn build_anthropic_client(
    config: &ProviderConfig,
) -> Result<rig::providers::anthropic::Client, BridgeError> {
    let mut builder = rig::providers::anthropic::Client::builder().api_key(&config.api_key);
    if let Some(ref base_url) = config.base_url {
        builder = builder.base_url(base_url);
    }
    builder.build().map_err(|e| {
        BridgeError::ProviderError(format!("failed to create Anthropic client: {}", e))
    })
}

fn build_gemini_client(
    config: &ProviderConfig,
) -> Result<rig::providers::gemini::Client, BridgeError> {
    let mut builder = rig::providers::gemini::Client::builder().api_key(&config.api_key);
    if let Some(ref base_url) = config.base_url {
        builder = builder.base_url(base_url);
    }
    builder
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create Gemini client: {}", e)))
}

fn build_cohere_client(
    config: &ProviderConfig,
) -> Result<rig::providers::cohere::Client, BridgeError> {
    let mut builder = rig::providers::cohere::Client::builder().api_key(&config.api_key);
    if let Some(ref base_url) = config.base_url {
        builder = builder.base_url(base_url);
    }
    builder
        .build()
        .map_err(|e| BridgeError::ProviderError(format!("failed to create Cohere client: {}", e)))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_openai_client_requires_base_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
        };
        assert!(build_openai_client(&config).is_err());
    }

    #[test]
    fn test_build_openai_client_with_base_url() {
        let config = ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "test-key".to_string(),
            base_url: Some("https://api.openai.com/v1".to_string()),
        };
        assert!(build_openai_client(&config).is_ok());
    }

    #[test]
    fn test_build_anthropic_client() {
        let config = ProviderConfig {
            provider_type: ProviderType::Anthropic,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
        };
        assert!(build_anthropic_client(&config).is_ok());
    }

    #[test]
    fn test_build_gemini_client() {
        let config = ProviderConfig {
            provider_type: ProviderType::Google,
            model: "gemini-2.0-flash".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
        };
        assert!(build_gemini_client(&config).is_ok());
    }

    #[test]
    fn test_build_cohere_client() {
        let config = ProviderConfig {
            provider_type: ProviderType::Cohere,
            model: "command-r-plus".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
        };
        assert!(build_cohere_client(&config).is_ok());
    }
}
