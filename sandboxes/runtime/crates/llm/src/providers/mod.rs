//! `BridgeAgent` — unified provider-agnostic LLM agent.
//!
//! Wraps rig-core's per-provider `Agent<M>` in an enum so the rest of
//! Bridge can treat the LLM as opaque. Adds a SHA-256 `prefix_hash`
//! computed at build time so every request can be correlated to the
//! exact cacheable bytes sent to the provider.

use std::pin::Pin;
use std::sync::Arc;

use futures::Stream;
use rig::agent::Agent;
use rig::message::Message;
use rig::prelude::CompletionClient;

pub(crate) mod build;
mod cache_control_middleware;
mod dispatch;
mod prompt_hooked;
mod prompt_plain;
mod retry;
pub mod tool_call_recovery;
mod tool_choice_middleware;

#[cfg(test)]
mod tests;

pub use build::create_agent;

// ---------------------------------------------------------------------------
// Per-provider completion-model type aliases
// ---------------------------------------------------------------------------
// Most rig clients are parameterised over the HTTP transport, so we pin them
// to `build::RetryingHttp` — a newtype over `ClientWithMiddleware` that adds
// `Default` (required by rig's generic builder) and delegates to a process-wide
// `reqwest_middleware::ClientWithMiddleware` carrying the retry middleware.
//
// Gemini is the exception: rig 0.35's `impl<H> Capabilities<H> for GeminiExt`
// hardcodes `Completion = Capable<CompletionModel>` without threading the H
// type parameter (other providers correctly write `Capable<CompletionModel<H>>`).
// As a result, swapping out Gemini's HTTP client doesn't actually swap the
// completion model, and we get a type mismatch when the model's Client type
// resolves back to `Client<GeminiExt, reqwest::Client>`. Until rig fixes this
// upstream, Gemini stays on the default `reqwest::Client` with no retry
// middleware. The harness retry only applies to OpenAI/Anthropic/Cohere paths.
type Http = crate::providers::build::RetryingHttp;
type OpenAIModel =
    <rig::providers::openai::CompletionsClient<Http> as CompletionClient>::CompletionModel;
type AnthropicModel =
    <rig::providers::anthropic::Client<Http> as CompletionClient>::CompletionModel;
type GeminiModel = <rig::providers::gemini::Client as CompletionClient>::CompletionModel;
type CohereModel = <rig::providers::cohere::Client<Http> as CompletionClient>::CompletionModel;

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
    /// Construct from already-built parts. Crate-internal: callers should
    /// go through [`create_agent`].
    pub(crate) fn from_parts(inner: BridgeAgentInner, prefix_hash: Arc<str>) -> Self {
        Self { inner, prefix_hash }
    }

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
    /// One HTTP call inside rig's multi-turn loop completed and returned its
    /// per-call usage. Bridge accumulates these so token counts survive even
    /// when a later HTTP call in the same conversation errors out (otherwise
    /// rig only surfaces aggregated_usage on the terminal `FinalResponse`,
    /// which never fires on error). One per LLM HTTP call inside a multi-turn
    /// streaming response.
    IntermediateUsage(rig::completion::Usage),
    /// The stream finished. Contains final text, aggregated token usage, and
    /// the enriched conversation history (if history was provided).
    StreamFinished {
        response: String,
        usage: rig::completion::Usage,
        history: Option<Vec<Message>>,
    },
    /// The agent loop was cancelled by a `PromptHook` returning
    /// `HookAction::Terminate`. Carries the history at the moment of
    /// cancellation (including all tool calls + results from the most recent
    /// LLM response) and the hook's reason string. Bridge uses this to
    /// trigger an immortal chain handoff mid-rig-loop and resume with the
    /// post-handoff history — the agent's last tool batch is preserved.
    HookCancelled {
        history: Vec<Message>,
        reason: String,
    },
    /// A streaming error occurred.
    StreamError(String),
}

/// A type-erased stream of [`BridgeStreamItem`]s.
pub type BridgeStream = Pin<Box<dyn Stream<Item = BridgeStreamItem> + Send>>;
