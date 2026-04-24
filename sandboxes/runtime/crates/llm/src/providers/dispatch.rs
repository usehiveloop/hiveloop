//! Shared dispatch macros, retry-delay helper, and streaming entrypoint.
//!
//! Each macro covers one call shape across all `BridgeAgentInner` variants
//! so the per-method retry wrappers in sibling modules can stay free of
//! provider-specific code.

use std::time::Duration;

use futures::stream::StreamExt;
use rig::completion::PromptError;
use rig::message::Message;
use rig::streaming::StreamingPrompt;
use tracing::warn;

use crate::tool_hook::ToolCallEmitter;

use super::retry::{BACKOFF_FACTOR, MAX_BACKOFF, MAX_RETRIES};
use super::{BridgeAgent, BridgeAgentInner, BridgeStream, BridgeStreamItem};

/// Helper macro to dispatch a prompt chain across all enum variants.
macro_rules! dispatch_prompt {
    ($agent_variant:expr, $text:expr, $history:expr, $hook:expr) => {{
        use rig::completion::Prompt;
        $agent_variant
            .prompt($text)
            .extended_details()
            .with_history($history.to_vec())
            .with_hook($hook)
            .await
            .map(|resp| crate::providers::PromptResponse {
                output: resp.output,
                total_usage: resp.usage,
            })
    }};
}

macro_rules! dispatch_prompt_simple {
    ($agent_variant:expr, $text:expr) => {{
        use rig::completion::Prompt;
        $agent_variant.prompt($text).await
    }};
}

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
                // Per-HTTP-call usage event inside rig's multi-turn loop.
                // Capture so partial usage survives a later turn's failure.
                Ok(MultiTurnStreamItem::StreamAssistantItem(StreamedAssistantContent::Final(
                    final_resp,
                ))) => {
                    use rig::completion::GetTokenUsage;
                    final_resp
                        .token_usage()
                        .map(BridgeStreamItem::IntermediateUsage)
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

pub(super) use dispatch_prompt;
pub(super) use dispatch_prompt_simple;

/// Apply the retry backoff before the next attempt. Preserves the
/// original warn-log shape used across all `prompt_*` methods.
#[inline]
pub(super) async fn pre_retry_delay(
    provider: &str,
    agent_id: Option<&str>,
    conversation_id: Option<&str>,
    attempt: usize,
    backoff: &mut Duration,
    last_error: &PromptError,
) {
    match (agent_id, conversation_id) {
        (Some(aid), Some(cid)) => warn!(
            agent_id = %aid,
            conversation_id = %cid,
            provider = provider,
            attempt = attempt,
            max_retries = MAX_RETRIES,
            backoff_ms = backoff.as_millis() as u64,
            error = %last_error,
            "llm_request_retry"
        ),
        _ => warn!(
            provider = provider,
            attempt = attempt,
            max_retries = MAX_RETRIES,
            backoff_ms = backoff.as_millis() as u64,
            error = %last_error,
            "llm_request_retry"
        ),
    }
    tokio::time::sleep(*backoff).await;
    *backoff = Duration::from_secs_f64(
        (backoff.as_secs_f64() * BACKOFF_FACTOR).min(MAX_BACKOFF.as_secs_f64()),
    );
}

impl BridgeAgent {
    /// Return the provider name for logging.
    pub(super) fn provider_name(&self) -> &'static str {
        match self.inner() {
            BridgeAgentInner::OpenAI(_) => "openai",
            BridgeAgentInner::Anthropic(_) => "anthropic",
            BridgeAgentInner::Gemini(_) => "gemini",
            BridgeAgentInner::Cohere(_) => "cohere",
        }
    }

    /// Stream a prompt with history and a tool-call hook, returning a
    /// provider-agnostic stream of text deltas and the final response.
    ///
    /// Unlike `prompt_with_hook` which waits for the full response, this
    /// returns immediately with a stream that yields text chunks as they
    /// arrive from the LLM, interleaved with tool execution (which the
    /// [`ToolCallEmitter`] hook handles via SSE directly).
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
        match self.inner() {
            BridgeAgentInner::OpenAI(a) => dispatch_stream!(a, text, history, hook),
            BridgeAgentInner::Anthropic(a) => dispatch_stream!(a, text, history, hook),
            BridgeAgentInner::Gemini(a) => dispatch_stream!(a, text, history, hook),
            BridgeAgentInner::Cohere(a) => dispatch_stream!(a, text, history, hook),
        }
    }
}
