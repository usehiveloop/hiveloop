//! Retry-classification helpers for LLM transient errors.

use std::time::Duration;

use rig::completion::{CompletionError, PromptError};

/// Maximum number of retry attempts for transient LLM errors.
pub(crate) const MAX_RETRIES: usize = 3;
/// Initial backoff delay between retries.
pub(crate) const INITIAL_BACKOFF: Duration = Duration::from_millis(500);
/// Maximum backoff delay.
pub(crate) const MAX_BACKOFF: Duration = Duration::from_secs(8);
/// Backoff multiplier.
pub(crate) const BACKOFF_FACTOR: f64 = 2.0;

/// Determine if a `PromptError` is transient and safe to retry.
///
/// Only HTTP-level errors are retryable — these occur before any tool
/// execution, so conversation history has not been modified by rig.
///
/// Non-retryable: auth failures (401/403), bad request (400), tool errors
/// (history already mutated), max turns, cancellation, JSON/URL parsing.
pub(crate) fn is_retryable_error(err: &PromptError) -> bool {
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
