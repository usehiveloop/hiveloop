//! Error variants exposed by `Embedder` implementations.
//!
//! Callers upstream distinguish between transient (retryable) and terminal
//! conditions. `RateLimited` and `Upstream5xx` after max retries are
//! surfaced so the orchestration layer (Go asynq) can choose whether to
//! retry the batch or fail the ingest attempt.

use thiserror::Error;

/// Unified error type for embedding operations.
#[derive(Debug, Error)]
pub enum EmbedError {
    /// Upstream returned 429 after we exhausted the retry budget.
    #[error("embedder rate-limited after max retries")]
    RateLimited,

    /// Upstream returned a 5xx after we exhausted the retry budget, or
    /// a 4xx other than 429 (e.g. 400, 401, 403).
    #[error("embedder upstream error: status={status}, body={body}")]
    Upstream { status: u16, body: String },

    /// Network error (timeout, connection refused, TLS failure, etc.) —
    /// already retried by the middleware layer.
    #[error("embedder transport error: {0}")]
    Transport(String),

    /// Response body parsed but was not the expected shape / was empty /
    /// the embedding dim didn't match the configured dim.
    #[error("embedder response invalid: {0}")]
    InvalidResponse(String),

    /// Configuration is malformed (e.g. missing API key at construction
    /// time, base URL is not an http(s) URL).
    #[error("embedder misconfigured: {0}")]
    Config(String),
}
