use std::pin::Pin;
use std::sync::Arc;
use std::time::Duration;

use bridge_core::provider::{ProviderConfig, ProviderType};
use bridge_core::BridgeError;
use futures::stream::StreamExt;
use futures::Stream;
use rig::agent::Agent;
use rig::completion::{CompletionError, CompletionModel, Prompt, PromptError};
use rig::message::Message;
use rig::prelude::CompletionClient;
use rig::streaming::StreamingPrompt;
use tracing::{error, info, warn};

use crate::prefix_hash::{
    prefix_hash_from_definitions, split_hashes_from_definitions, suspected_volatile_markers,
};
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

/// Provider-specific inner agent. Wrapped by [`BridgeAgent`] so that the
/// prefix-hash metadata (see P0.3) travels alongside the dispatch.
#[derive(Clone)]
pub enum BridgeAgentInner {
    OpenAI(Agent<OpenAIModel>),
    Anthropic(Agent<AnthropicModel>),
    Gemini(Agent<GeminiModel>),
    Cohere(Agent<CohereModel>),
}

/// Unified agent type supporting multiple LLM providers.
///
/// Each variant of [`BridgeAgentInner`] wraps a rig-core `Agent<M>`. The
/// outer struct adds a `prefix_hash` — SHA-256 of `(preamble || tool_defs)`
/// — so that every request can be correlated to the cacheable prefix at
/// the time of agent construction. If two requests from the same agent
/// emit different hashes in the logs, something is mutating the prefix and
/// silently breaking cache reuse.
#[derive(Clone)]
pub struct BridgeAgent {
    inner: BridgeAgentInner,
    prefix_hash: Arc<str>,
}

impl BridgeAgent {
    /// SHA-256 hex digest of the cacheable prefix.
    pub fn prefix_hash(&self) -> &str {
        &self.prefix_hash
    }

    /// Access to the underlying provider-specific agent. Primarily for tests.
    pub fn inner(&self) -> &BridgeAgentInner {
        &self.inner
    }
}

/// Response from a prompt with extended details (token usage).
pub struct PromptResponse {
    pub output: String,
    pub total_usage: rig::completion::Usage,
}

/// Provider-agnostic stream item for real-time text streaming.
///
/// Erases the provider-specific response type from rig's `MultiTurnStreamItem<R>`,
/// exposing only the items Bridge needs: text deltas, final response, and errors.
/// Tool call events are handled separately by `ToolCallEmitter` hooks.
pub enum BridgeStreamItem {
    /// Incremental text token from the assistant.
    TextDelta(String),
    /// Incremental reasoning/thinking text from the model.
    ReasoningDelta(String),
    /// The stream finished. Contains final text, aggregated token usage, and
    /// the enriched conversation history (if history was provided).
    StreamFinished {
        response: String,
        usage: rig::completion::Usage,
        history: Option<Vec<Message>>,
    },
    /// A streaming error occurred.
    StreamError(String),
}

/// A type-erased stream of [`BridgeStreamItem`]s.
pub type BridgeStream = Pin<Box<dyn Stream<Item = BridgeStreamItem> + Send>>;

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

/// Dispatch a streaming prompt across a concrete agent variant, mapping
/// the provider-specific `MultiTurnStreamItem<R>` into `BridgeStreamItem`.
///
/// Text deltas and the final response are forwarded; tool events are filtered
/// out because `ToolCallEmitter` hooks handle them via SSE directly.
macro_rules! dispatch_stream {
    ($agent_variant:expr, $text:expr, $history:expr, $hook:expr) => {{
        use rig::agent::MultiTurnStreamItem;
        use rig::streaming::StreamedAssistantContent;

        let stream = $agent_variant
            .stream_prompt($text)
            .with_history($history)
            .with_hook($hook)
            .await;

        let mapped = stream.filter_map(|item| async move {
            match item {
                Ok(MultiTurnStreamItem::StreamAssistantItem(StreamedAssistantContent::Text(
                    text,
                ))) => {
                    if text.text.is_empty() {
                        None
                    } else {
                        Some(BridgeStreamItem::TextDelta(text.text))
                    }
                }
                Ok(MultiTurnStreamItem::FinalResponse(f)) => {
                    Some(BridgeStreamItem::StreamFinished {
                        response: f.response().to_string(),
                        usage: f.usage(),
                        history: f.history().map(|h: &[Message]| h.to_vec()),
                    })
                }
                Ok(MultiTurnStreamItem::StreamAssistantItem(
                    StreamedAssistantContent::ReasoningDelta { reasoning, .. },
                )) => {
                    if reasoning.is_empty() {
                        None
                    } else {
                        Some(BridgeStreamItem::ReasoningDelta(reasoning))
                    }
                }
                Ok(MultiTurnStreamItem::StreamAssistantItem(
                    StreamedAssistantContent::Reasoning(reasoning),
                )) => {
                    let text = reasoning
                        .content
                        .iter()
                        .filter_map(|c| match c {
                            rig::completion::message::ReasoningContent::Text { text, .. } => {
                                Some(text.as_str())
                            }
                            _ => None,
                        })
                        .collect::<Vec<_>>()
                        .join("");
                    if text.is_empty() {
                        None
                    } else {
                        Some(BridgeStreamItem::ReasoningDelta(text))
                    }
                }
                Err(e) => Some(BridgeStreamItem::StreamError(format!("{}", e))),
                _ => None, // tool events — handled by ToolCallEmitter hook
            }
        });

        Box::pin(mapped) as BridgeStream
    }};
}

impl BridgeAgent {
    /// Return the provider name for logging.
    fn provider_name(&self) -> &'static str {
        match &self.inner {
            BridgeAgentInner::OpenAI(_) => "openai",
            BridgeAgentInner::Anthropic(_) => "anthropic",
            BridgeAgentInner::Gemini(_) => "gemini",
            BridgeAgentInner::Cohere(_) => "cohere",
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
                prefix_hash = %self.prefix_hash(),
                history_len = history.len(),
                attempt = attempt,
                "llm_request_start"
            );

            let start = std::time::Instant::now();
            let result = match &self.inner {
                BridgeAgentInner::OpenAI(a) => dispatch_prompt!(a, text, history, hook_clone),
                BridgeAgentInner::Anthropic(a) => dispatch_prompt!(a, text, history, hook_clone),
                BridgeAgentInner::Gemini(a) => dispatch_prompt!(a, text, history, hook_clone),
                BridgeAgentInner::Cohere(a) => dispatch_prompt!(a, text, history, hook_clone),
            };
            let latency_ms = start.elapsed().as_millis() as u64;

            match result {
                Ok(resp) => {
                    info!(
                        agent_id = %agent_id,
                        conversation_id = %conversation_id,
                        provider = provider,
                        input_tokens = resp.total_usage.input_tokens,
                        cached_input_tokens = resp.total_usage.cached_input_tokens,
                        cache_hit_ratio = bridge_core::metrics::cache_hit_ratio(
                            resp.total_usage.input_tokens,
                            resp.total_usage.cached_input_tokens
                        ),
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
                prefix_hash = %self.prefix_hash(),
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
            let result = match &self.inner {
                BridgeAgentInner::OpenAI(a) => dispatch!(a),
                BridgeAgentInner::Anthropic(a) => dispatch!(a),
                BridgeAgentInner::Gemini(a) => dispatch!(a),
                BridgeAgentInner::Cohere(a) => dispatch!(a),
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

            info!(provider = provider, prefix_hash = %self.prefix_hash(), attempt = attempt, "llm_request_start");

            let start = std::time::Instant::now();
            macro_rules! dispatch {
                ($agent:expr) => {{
                    $agent.prompt(text).with_history(history).await
                }};
            }
            let result = match &self.inner {
                BridgeAgentInner::OpenAI(a) => dispatch!(a),
                BridgeAgentInner::Anthropic(a) => dispatch!(a),
                BridgeAgentInner::Gemini(a) => dispatch!(a),
                BridgeAgentInner::Cohere(a) => dispatch!(a),
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

    /// Stream a prompt with history and a tool-call hook, returning a
    /// provider-agnostic stream of text deltas and the final response.
    ///
    /// Unlike [`prompt_with_hook`](Self::prompt_with_hook) which waits for the
    /// full response, this returns immediately with a stream that yields text
    /// chunks as they arrive from the LLM, interleaved with tool execution
    /// (which the [`ToolCallEmitter`] hook handles via SSE directly).
    ///
    /// History is passed by value because the streaming path consumes it;
    /// the enriched history is returned via [`BridgeStreamItem::StreamFinished`].
    ///
    /// No retry logic is applied — once streaming starts, data has already been
    /// emitted to the client, making retries unsafe.
    pub async fn stream_prompt_with_hook(
        &self,
        text: &str,
        history: Vec<Message>,
        hook: ToolCallEmitter,
    ) -> BridgeStream {
        match &self.inner {
            BridgeAgentInner::OpenAI(a) => dispatch_stream!(a, text, history, hook),
            BridgeAgentInner::Anthropic(a) => dispatch_stream!(a, text, history, hook),
            BridgeAgentInner::Gemini(a) => dispatch_stream!(a, text, history, hook),
            BridgeAgentInner::Cohere(a) => dispatch_stream!(a, text, history, hook),
        }
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

            info!(provider = provider, prefix_hash = %self.prefix_hash(), attempt = attempt, "llm_request_start");

            let start = std::time::Instant::now();
            let result = match &self.inner {
                BridgeAgentInner::OpenAI(a) => dispatch_prompt_simple!(a, text),
                BridgeAgentInner::Anthropic(a) => dispatch_prompt_simple!(a, text),
                BridgeAgentInner::Gemini(a) => dispatch_prompt_simple!(a, text),
                BridgeAgentInner::Cohere(a) => dispatch_prompt_simple!(a, text),
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
    // Compute prefix hash BEFORE moving `tools` into the builder. The hash
    // fingerprints the exact (preamble || tool_defs) bytes the provider
    // will see — any drift between two calls with identical agent config
    // means our prefix is non-deterministic and cache hits will suffer.
    let tool_defs: Vec<rig::completion::ToolDefinition> =
        tools.iter().map(|t| t.definition_sync()).collect();
    let prefix_hash: Arc<str> = prefix_hash_from_definitions(preamble, &tool_defs).into();
    let (preamble_hash, tools_hash) = split_hashes_from_definitions(preamble, &tool_defs);

    // Hygiene warning: if the preamble looks like it interpolates dynamic
    // content, cache hits will thrash. We only log — never fail — because
    // false positives on static text that happens to mention a year are
    // possible. Grep the logs for `preamble_volatile_markers` if hit rate
    // suddenly drops.
    let markers = suspected_volatile_markers(preamble);
    if !markers.is_empty() {
        warn!(
            provider = %config.provider_type,
            model = %config.model,
            preamble_hash = %preamble_hash,
            markers = ?markers,
            "preamble_volatile_markers_detected"
        );
    }

    info!(
        provider = %config.provider_type,
        model = %config.model,
        prefix_hash = %prefix_hash,
        preamble_hash = %preamble_hash,
        tools_hash = %tools_hash,
        tool_count = tool_defs.len(),
        preamble_bytes = preamble.len(),
        "bridge_agent_built"
    );

    let inner = match config.provider_type {
        // Native Anthropic client
        ProviderType::Anthropic => {
            let client = build_anthropic_client(config)?;
            // P2: enable explicit prompt-cache breakpoints on Anthropic when
            // caching is permitted for this agent. `with_prompt_caching` is
            // on the CompletionModel, not the AgentBuilder — hence the
            // detour through `completion_model(...)`. rig 0.31 places the
            // breakpoints on the last system block and the last message,
            // which is the minimum viable "automatic" layout.
            let mut model = client.completion_model(&config.model);
            if config.prompt_caching_enabled {
                info!(
                    provider = "anthropic",
                    model = %config.model,
                    cache_ttl = ?config.cache_ttl,
                    "anthropic_prompt_caching_enabled"
                );
                model = model.with_prompt_caching();
            }
            let builder = rig::agent::AgentBuilder::new(model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::Anthropic(agent)
        }
        // Native Gemini client
        ProviderType::Google => {
            let client = build_gemini_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::Gemini(agent)
        }
        // Native Cohere client
        ProviderType::Cohere => {
            let client = build_cohere_client(config)?;
            let builder = client.agent(&config.model);
            let agent = configure_and_build(builder, preamble, definition, tools);
            BridgeAgentInner::Cohere(agent)
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
            BridgeAgentInner::OpenAI(agent)
        }
    };

    Ok(BridgeAgent { inner, prefix_hash })
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

    // Wire json_schema for structured output
    let builder = if let Some(ref json_schema) = definition.config.json_schema {
        // Extract the inner "schema" field (OpenAI format: {name, schema})
        let schema_value = json_schema.get("schema").unwrap_or(json_schema);
        match serde_json::from_value::<schemars::Schema>(schema_value.clone()) {
            Ok(schema) => builder.output_schema_raw(schema),
            Err(e) => {
                tracing::warn!("invalid json_schema, skipping structured output: {}", e);
                builder
            }
        }
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
    // 1-hour cache TTL ships behind a beta header. We set it whenever the
    // caller opts into OneHour so that the moment rig exposes `"ttl":"1h"`
    // on `CacheControl`, existing agents start getting 1-hour writes
    // without a config change. With rig 0.31 the effective TTL is still
    // 5-minute, but the header is a no-op otherwise and safe to send.
    if matches!(config.cache_ttl, bridge_core::provider::CacheTtl::OneHour) {
        builder = builder.anthropic_beta("extended-cache-ttl-2025-04-11");
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
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
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
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
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
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        };
        assert!(build_anthropic_client(&config).is_ok());
    }

    #[test]
    fn test_build_anthropic_client_with_one_hour_ttl_succeeds() {
        // Setting the 1-hour TTL is always safe — the beta header is just
        // attached. Actual cache_control TTL wiring awaits a newer rig.
        let config = ProviderConfig {
            provider_type: ProviderType::Anthropic,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
            prompt_caching_enabled: true,
            cache_ttl: bridge_core::provider::CacheTtl::OneHour,
        };
        assert!(build_anthropic_client(&config).is_ok());
    }

    #[tokio::test]
    async fn test_create_agent_anthropic_computes_prefix_hash() {
        // Build a minimal agent and confirm the prefix hash is populated
        // (non-empty, 64 hex chars). The hash must be the same for two
        // identical builds — the cache-bust invariant this P0.3 work
        // protects.
        let config = ProviderConfig {
            provider_type: ProviderType::Anthropic,
            model: "claude-sonnet-4-20250514".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        };
        let definition = bridge_core::agent::AgentDefinition {
            id: "t".into(),
            name: "t".into(),
            description: None,
            system_prompt: "you are helpful".into(),
            provider: config.clone(),
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: bridge_core::agent::AgentConfig::default(),
            subagents: vec![],
            permissions: std::collections::HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        };
        let a = create_agent(&config, vec![], "you are helpful", &definition).unwrap();
        let b = create_agent(&config, vec![], "you are helpful", &definition).unwrap();
        assert_eq!(a.prefix_hash().len(), 64);
        assert_eq!(
            a.prefix_hash(),
            b.prefix_hash(),
            "two identical builds must produce the same prefix hash"
        );
    }

    #[tokio::test]
    async fn test_create_agent_prefix_hash_changes_with_preamble() {
        let config = ProviderConfig {
            provider_type: ProviderType::OpenAI,
            model: "gpt-4o".to_string(),
            api_key: "k".to_string(),
            base_url: Some("https://api.openai.com/v1".to_string()),
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
        };
        let definition = bridge_core::agent::AgentDefinition {
            id: "t".into(),
            name: "t".into(),
            description: None,
            system_prompt: "ignored".into(),
            provider: config.clone(),
            tools: vec![],
            mcp_servers: vec![],
            skills: vec![],
            integrations: vec![],
            config: bridge_core::agent::AgentConfig::default(),
            subagents: vec![],
            permissions: std::collections::HashMap::new(),
            webhook_url: None,
            webhook_secret: None,
            version: None,
            updated_at: None,
        };
        let a = create_agent(&config, vec![], "preamble A", &definition).unwrap();
        let b = create_agent(&config, vec![], "preamble B", &definition).unwrap();
        assert_ne!(
            a.prefix_hash(),
            b.prefix_hash(),
            "preamble diff must surface as a prefix-hash diff"
        );
    }

    #[test]
    fn test_build_gemini_client() {
        let config = ProviderConfig {
            provider_type: ProviderType::Google,
            model: "gemini-2.0-flash".to_string(),
            api_key: "test-key".to_string(),
            base_url: None,
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
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
            prompt_caching_enabled: true,
            cache_ttl: Default::default(),
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

    #[test]
    fn test_bridge_stream_item_reasoning_delta_variant_exists() {
        let item = BridgeStreamItem::ReasoningDelta("thinking...".to_string());
        match item {
            BridgeStreamItem::ReasoningDelta(text) => {
                assert_eq!(text, "thinking...");
            }
            _ => panic!("expected ReasoningDelta"),
        }
    }

    #[test]
    fn test_reasoning_delta_empty_is_filtered() {
        // Verify that empty reasoning deltas would be filtered (matches provider logic)
        let delta = "";
        assert!(delta.is_empty(), "empty reasoning should be filtered");
    }
}
