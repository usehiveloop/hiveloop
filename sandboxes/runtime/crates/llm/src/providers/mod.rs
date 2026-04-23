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

mod build;
mod dispatch;
mod prompt_hooked;
mod prompt_plain;
mod retry;

#[cfg(test)]
mod tests;

pub use build::create_agent;

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
