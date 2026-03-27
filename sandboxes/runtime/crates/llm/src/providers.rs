use std::time::Duration;

use bridge_core::provider::{ProviderConfig, ProviderType};
use bridge_core::BridgeError;
use rig::agent::Agent;
use rig::completion::{CompletionError, CompletionModel, Prompt, PromptError};
use rig::message::Message;
use rig::prelude::CompletionClient;
use tracing::{error, info, warn};

use crate::tool_hook::ToolCallEmitter;

// ---------------------------------------------------------------------------
// Retry configuration
// ---------------------------------------------------------------------------

/// Maximum number of retry attempts for transient LLM errors.
const MAX_RETRIES: usize = 3;
/// Initial backoff delay between retries.
const INITIAL_BACKOFF: Duration = Duration::from_millis(500);
/// Maximum backoff delay.
const MAX_BACKOFF: Duration = Duration::from_secs(8);
/// Backoff multiplier.
const BACKOFF_FACTOR: f64 = 2.0;

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
        $agent_variant
            .prompt($text)
            .extended_details()
            .with_history($history)
            .with_hook($hook)
            .await
            .map(|resp| PromptResponse {
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
    /// Automatically retries on transient HTTP errors with exponential backoff.
    pub async fn prompt_with_hook(
        &self,
        text: &str,
        history: &mut Vec<Message>,
        hook: ToolCallEmitter,
    ) -> Result<PromptResponse, PromptError> {
        let provider = self.provider_name();
        let agent_id = hook.agent_id.clone();
        let conversation_id = hook.conversation_id.clone();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                warn!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    provider = provider,
                    attempt = attempt,
                    max_retries = MAX_RETRIES,
                    backoff_ms = backoff.as_millis() as u64,
                    error = %last_error.as_ref().unwrap(),
                    "llm_request_retry"
                );
                tokio::time::sleep(backoff).await;
                backoff = Duration::from_secs_f64(
                    (backoff.as_secs_f64() * BACKOFF_FACTOR).min(MAX_BACKOFF.as_secs_f64()),
                );
            }

            let hook_clone = hook.clone();
            info!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                provider = provider,
                history_len = history.len(),
                attempt = attempt,
                "llm_request_start"
            );

            let start = std::time::Instant::now();
            let result = match self {
                BridgeAgent::OpenAI(a) => dispatch_prompt!(a, text, history, hook_clone),
                BridgeAgent::Anthropic(a) => dispatch_prompt!(a, text, history, hook_clone),
                BridgeAgent::Gemini(a) => dispatch_prompt!(a, text, history, hook_clone),
                BridgeAgent::Cohere(a) => dispatch_prompt!(a, text, history, hook_clone),
            };
            let latency_ms = start.elapsed().as_millis() as u64;

            match result {
                Ok(resp) => {
                    info!(
                        agent_id = %agent_id,
                        conversation_id = %conversation_id,
                        provider = provider,
                        input_tokens = resp.total_usage.input_tokens,
                        output_tokens = resp.total_usage.output_tokens,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_complete"
                    );
                    return Ok(resp);
                }
                Err(e) => {
                    if is_retryable_error(&e) && attempt < MAX_RETRIES {
                        error!(
                            agent_id = %agent_id,
                            conversation_id = %conversation_id,
                            provider = provider,
                            error = %e,
                            latency_ms = latency_ms,
                            attempt = attempt,
                            "llm_request_failed_will_retry"
                        );
                        last_error = Some(e);
                        continue;
                    }
                    error!(
                        agent_id = %agent_id,
                        conversation_id = %conversation_id,
                        provider = provider,
                        error = %e,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_failed"
                    );
                    return Err(e);
                }
            }
        }

        Err(last_error.unwrap())
    }

    /// Run a prompt with history and a tool-call hook, returning just the text output.
    ///
    /// Used by the subagent runner.
    /// Automatically retries on transient HTTP errors with exponential backoff.
    pub async fn prompt_standard_with_hook(
        &self,
        text: &str,
        history: &mut Vec<Message>,
        hook: ToolCallEmitter,
    ) -> Result<String, PromptError> {
        let provider = self.provider_name();
        let agent_id = hook.agent_id.clone();
        let conversation_id = hook.conversation_id.clone();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                warn!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    provider = provider,
                    attempt = attempt,
                    max_retries = MAX_RETRIES,
                    backoff_ms = backoff.as_millis() as u64,
                    error = %last_error.as_ref().unwrap(),
                    "llm_request_retry"
                );
                tokio::time::sleep(backoff).await;
                backoff = Duration::from_secs_f64(
                    (backoff.as_secs_f64() * BACKOFF_FACTOR).min(MAX_BACKOFF.as_secs_f64()),
                );
            }

            let hook_clone = hook.clone();
            info!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                provider = provider,
                attempt = attempt,
                "llm_request_start"
            );

            let start = std::time::Instant::now();
            macro_rules! dispatch {
                ($agent:expr) => {{
                    $agent
                        .prompt(text)
                        .with_history(history)
                        .with_hook(hook_clone)
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

            match result {
                Ok(output) => {
                    info!(
                        agent_id = %agent_id,
                        conversation_id = %conversation_id,
                        provider = provider,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_complete"
                    );
                    return Ok(output);
                }
                Err(e) => {
                    if is_retryable_error(&e) && attempt < MAX_RETRIES {
                        error!(
                            agent_id = %agent_id,
                            conversation_id = %conversation_id,
                            provider = provider,
                            error = %e,
                            latency_ms = latency_ms,
                            attempt = attempt,
                            "llm_request_failed_will_retry"
                        );
                        last_error = Some(e);
                        continue;
                    }
                    error!(
                        agent_id = %agent_id,
                        conversation_id = %conversation_id,
                        provider = provider,
                        error = %e,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_failed"
                    );
                    return Err(e);
                }
            }
        }

        Err(last_error.unwrap())
    }

    /// Prompt with history but no hooks. Returns just the text output.
    ///
    /// Used by the no-tools retry agent.
    /// Automatically retries on transient HTTP errors with exponential backoff.
    pub async fn prompt_with_history(
        &self,
        text: &str,
        history: &mut Vec<Message>,
    ) -> Result<String, PromptError> {
        let provider = self.provider_name();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                warn!(
                    provider = provider,
                    attempt = attempt,
                    max_retries = MAX_RETRIES,
                    backoff_ms = backoff.as_millis() as u64,
                    error = %last_error.as_ref().unwrap(),
                    "llm_request_retry"
                );
                tokio::time::sleep(backoff).await;
                backoff = Duration::from_secs_f64(
                    (backoff.as_secs_f64() * BACKOFF_FACTOR).min(MAX_BACKOFF.as_secs_f64()),
                );
            }

            info!(provider = provider, attempt = attempt, "llm_request_start");

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

            match result {
                Ok(output) => {
                    info!(
                        provider = provider,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_complete"
                    );
                    return Ok(output);
                }
                Err(e) => {
                    if is_retryable_error(&e) && attempt < MAX_RETRIES {
                        error!(
                            provider = provider,
                            error = %e,
                            latency_ms = latency_ms,
                            attempt = attempt,
                            "llm_request_failed_will_retry"
                        );
                        last_error = Some(e);
                        continue;
                    }
                    error!(
                        provider = provider,
                        error = %e,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_failed"
                    );
                    return Err(e);
                }
            }
        }

        Err(last_error.unwrap())
    }

    /// Simple prompt without hooks or history. Returns just the text output.
    ///
    /// Used by the compaction summarizer.
    /// Automatically retries on transient HTTP errors with exponential backoff.
    pub async fn prompt_simple(&self, text: &str) -> Result<String, PromptError> {
        let provider = self.provider_name();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                warn!(
                    provider = provider,
                    attempt = attempt,
                    max_retries = MAX_RETRIES,
                    backoff_ms = backoff.as_millis() as u64,
                    error = %last_error.as_ref().unwrap(),
                    "llm_request_retry"
                );
                tokio::time::sleep(backoff).await;
                backoff = Duration::from_secs_f64(
                    (backoff.as_secs_f64() * BACKOFF_FACTOR).min(MAX_BACKOFF.as_secs_f64()),
                );
            }

            info!(provider = provider, attempt = attempt, "llm_request_start");

            let start = std::time::Instant::now();
            let result = match self {
                BridgeAgent::OpenAI(a) => dispatch_prompt_simple!(a, text),
                BridgeAgent::Anthropic(a) => dispatch_prompt_simple!(a, text),
                BridgeAgent::Gemini(a) => dispatch_prompt_simple!(a, text),
                BridgeAgent::Cohere(a) => dispatch_prompt_simple!(a, text),
            };
            let latency_ms = start.elapsed().as_millis() as u64;

            match result {
                Ok(output) => {
                    info!(
                        provider = provider,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_complete"
                    );
                    return Ok(output);
                }
                Err(e) => {
                    if is_retryable_error(&e) && attempt < MAX_RETRIES {
                        error!(
                            provider = provider,
                            error = %e,
                            latency_ms = latency_ms,
                            attempt = attempt,
                            "llm_request_failed_will_retry"
                        );
                        last_error = Some(e);
                        continue;
                    }
                    error!(
                        provider = provider,
                        error = %e,
                        latency_ms = latency_ms,
                        attempt = attempt,
                        "llm_request_failed"
                    );
                    return Err(e);
                }
            }
        }

        Err(last_error.unwrap())
    }
}

// ---------------------------------------------------------------------------
// Retry classification
// ---------------------------------------------------------------------------

/// Determine if a `PromptError` is transient and safe to retry.
///
/// Only HTTP-level errors are retryable — these occur before any tool
/// execution, so conversation history has not been modified by rig.
///
/// Non-retryable: auth failures (401/403), bad request (400), tool errors
/// (history already mutated), max turns, cancellation, JSON/URL parsing.
fn is_retryable_error(err: &PromptError) -> bool {
    match err {
        PromptError::CompletionError(completion_err) => match completion_err {
            CompletionError::HttpError(http_err) => {
                use rig::http_client::Error;
                match http_err {
                    Error::InvalidStatusCode(status)
                    | Error::InvalidStatusCodeWithMessage(status, _) => {
                        status.is_server_error() || status.as_u16() == 429
                    }
                    // Network-level errors (timeout, connection refused, DNS failure)
                    Error::Instance(_) => true,
                    // Connection dropped mid-stream
                    Error::StreamEnded => true,
                    // Structural/protocol errors — not transient
                    Error::Protocol(_)
                    | Error::InvalidHeaderValue(_)
                    | Error::NoHeaders
                    | Error::InvalidContentType(_) => false,
                }
            }
            CompletionError::ProviderError(msg) => {
                // Some providers wrap HTTP errors in string messages
                msg.contains("502")
                    || msg.contains("503")
                    || msg.contains("504")
                    || msg.contains("429")
                    || msg.contains("upstream")
                    || msg.contains("overloaded")
                    || msg.contains("timeout")
                    || msg.contains("connection")
            }
            CompletionError::RequestError(_) => true,
            // JsonError can be transient with OpenAI-compatible providers
            // (e.g. OpenRouter) that intermittently return non-standard formats.
            CompletionError::JsonError(_) => true,
            CompletionError::UrlError(_) | CompletionError::ResponseError(_) => false,
        },
        // Tool errors mean tools already executed — NOT safe to retry
        PromptError::ToolError(_)
        | PromptError::ToolServerError(_)
        | PromptError::MaxTurnsError { .. }
        | PromptError::PromptCancelled { .. } => false,
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

    // -----------------------------------------------------------------------
    // is_retryable_error tests
    // -----------------------------------------------------------------------

    fn http_error(err: rig::http_client::Error) -> PromptError {
        PromptError::CompletionError(CompletionError::HttpError(err))
    }

    #[test]
    fn test_retryable_502_bad_gateway() {
        let err = http_error(rig::http_client::Error::InvalidStatusCodeWithMessage(
            http::StatusCode::BAD_GATEWAY,
            "upstream unreachable".into(),
        ));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_503_service_unavailable() {
        let err = http_error(rig::http_client::Error::InvalidStatusCode(
            http::StatusCode::SERVICE_UNAVAILABLE,
        ));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_500_internal_server_error() {
        let err = http_error(rig::http_client::Error::InvalidStatusCode(
            http::StatusCode::INTERNAL_SERVER_ERROR,
        ));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_429_rate_limit() {
        let err = http_error(rig::http_client::Error::InvalidStatusCode(
            http::StatusCode::TOO_MANY_REQUESTS,
        ));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_stream_ended() {
        let err = http_error(rig::http_client::Error::StreamEnded);
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_network_error() {
        let network_err: Box<dyn std::error::Error + Send + Sync> =
            "connection reset".to_string().into();
        let err = http_error(rig::http_client::Error::Instance(network_err));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_provider_error_upstream() {
        let err = PromptError::CompletionError(CompletionError::ProviderError(
            "upstream unreachable".into(),
        ));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_provider_error_overloaded() {
        let err = PromptError::CompletionError(CompletionError::ProviderError(
            "model is overloaded".into(),
        ));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_401_unauthorized() {
        let err = http_error(rig::http_client::Error::InvalidStatusCodeWithMessage(
            http::StatusCode::UNAUTHORIZED,
            "invalid api key".into(),
        ));
        assert!(!is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_400_bad_request() {
        let err = http_error(rig::http_client::Error::InvalidStatusCode(
            http::StatusCode::BAD_REQUEST,
        ));
        assert!(!is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_403_forbidden() {
        let err = http_error(rig::http_client::Error::InvalidStatusCode(
            http::StatusCode::FORBIDDEN,
        ));
        assert!(!is_retryable_error(&err));
    }

    #[test]
    fn test_retryable_json_error() {
        let json_err = serde_json::from_str::<String>("not json").unwrap_err();
        let err = PromptError::CompletionError(CompletionError::JsonError(json_err));
        assert!(is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_tool_error() {
        let err = PromptError::ToolError(rig::tool::ToolSetError::ToolNotFoundError("x".into()));
        assert!(!is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_max_turns() {
        let err = PromptError::MaxTurnsError {
            max_turns: 10,
            chat_history: Box::new(vec![]),
            prompt: Box::new(Message::from("test")),
        };
        assert!(!is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_prompt_cancelled() {
        let err = PromptError::PromptCancelled {
            chat_history: Box::new(vec![]),
            reason: "user cancelled".into(),
        };
        assert!(!is_retryable_error(&err));
    }

    #[test]
    fn test_not_retryable_provider_error_generic() {
        let err = PromptError::CompletionError(CompletionError::ProviderError(
            "invalid model name".into(),
        ));
        assert!(!is_retryable_error(&err));
    }
}
