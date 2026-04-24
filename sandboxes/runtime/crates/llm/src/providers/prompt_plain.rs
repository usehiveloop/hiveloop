//! `prompt_with_history` and `prompt_simple` — retry wrappers that skip
//! the tool-call hook.

use rig::completion::PromptError;
use rig::message::Message;
use tracing::{error, info};

use super::dispatch::{dispatch_prompt_simple, pre_retry_delay};
use super::retry::{is_retryable_error, INITIAL_BACKOFF, MAX_RETRIES};
use super::{BridgeAgent, BridgeAgentInner};

impl BridgeAgent {
    /// Prompt with history but no hooks. Returns just the text output.
    ///
    /// Used by the no-tools retry agent.
    /// Automatically retries on transient HTTP errors with exponential backoff.
    pub async fn prompt_with_history(
        &self,
        text: &str,
        history: &mut [Message],
    ) -> Result<String, PromptError> {
        let provider = self.provider_name();

        let mut last_error: Option<PromptError> = None;
        let mut backoff = INITIAL_BACKOFF;

        for attempt in 0..=MAX_RETRIES {
            if attempt > 0 {
                pre_retry_delay(
                    provider,
                    None,
                    None,
                    attempt,
                    &mut backoff,
                    last_error.as_ref().unwrap(),
                )
                .await;
            }

            info!(provider = provider, prefix_hash = %self.prefix_hash(), attempt = attempt, "llm_request_start");

            let start = std::time::Instant::now();
            macro_rules! dispatch {
                ($agent:expr) => {{
                    use rig::completion::Prompt;
                    $agent.prompt(text).with_history(history.to_vec()).await
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
                pre_retry_delay(
                    provider,
                    None,
                    None,
                    attempt,
                    &mut backoff,
                    last_error.as_ref().unwrap(),
                )
                .await;
            }

            info!(provider = provider, prefix_hash = %self.prefix_hash(), attempt = attempt, "llm_request_start");

            let start = std::time::Instant::now();
            let result = match self.inner() {
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
