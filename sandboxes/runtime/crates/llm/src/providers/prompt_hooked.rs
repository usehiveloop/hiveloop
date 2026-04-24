//! `prompt_with_hook` and `prompt_standard_with_hook` — retry wrappers over
//! rig's `.prompt(...).with_hook(...)` chain.

use rig::completion::PromptError;
use rig::message::Message;
use tracing::{error, info};

use crate::tool_hook::ToolCallEmitter;

use super::dispatch::{dispatch_prompt, pre_retry_delay};
use super::retry::{is_retryable_error, INITIAL_BACKOFF, MAX_RETRIES};
use super::{BridgeAgent, BridgeAgentInner, PromptResponse};

impl BridgeAgent {
    /// Run a prompt with history and a tool-call hook, returning extended
    /// details (output text + token usage).
    ///
    /// This is the primary entry point used by the conversation loop.
    /// Automatically retries on transient HTTP errors with exponential backoff.
    pub async fn prompt_with_hook(
        &self,
        text: &str,
        history: &mut [Message],
        hook: ToolCallEmitter,
    ) -> Result<PromptResponse, PromptError> {
        let provider = self.provider_name();
        let agent_id = hook.agent_id.clone();
        let conversation_id = hook.conversation_id.clone();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                pre_retry_delay(
                    provider,
                    Some(&agent_id),
                    Some(&conversation_id),
                    attempt,
                    &mut backoff,
                    last_error.as_ref().unwrap(),
                )
                .await;
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
            let result = match self.inner() {
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
        history: &mut [Message],
        hook: ToolCallEmitter,
    ) -> Result<String, PromptError> {
        let provider = self.provider_name();
        let agent_id = hook.agent_id.clone();
        let conversation_id = hook.conversation_id.clone();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                pre_retry_delay(
                    provider,
                    Some(&agent_id),
                    Some(&conversation_id),
                    attempt,
                    &mut backoff,
                    last_error.as_ref().unwrap(),
                )
                .await;
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
                    use rig::completion::Prompt;
                    $agent
                        .prompt(text)
                        .with_history(history.to_vec())
                        .with_hook(hook_clone)
                        .await
                }};
            }
            let result = match self.inner() {
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
}
