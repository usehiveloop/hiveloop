//! Reranker trait + SiliconFlow Qwen3-Reranker implementation +
//! deterministic `FakeReranker`.
//!
//! Phase 2 Tranche 2D.
//!
//! The trait surface is minimal by design: callers pass an ordered list of
//! candidate strings (chunk content) alongside a query, and receive scores
//! aligned 1:1 with the input order. Score semantics are a relevance score
//! in `[0.0, 1.0]`, larger-is-better. Ordering of candidates is preserved —
//! top-K selection is the caller's responsibility.
//!
//! Mocking is permitted for this crate per `internal/rag/doc/TESTING.md`
//! rule 1: the SiliconFlow rerank endpoint is paid and external.

#![deny(unsafe_code)]
#![warn(missing_debug_implementations)]

pub mod batching;
pub mod config;
pub mod fake;
pub mod siliconflow;

use async_trait::async_trait;
use thiserror::Error;

/// Errors produced by any `Reranker` implementation.
#[derive(Debug, Error)]
pub enum RerankError {
    /// Upstream HTTP call failed after retries.
    #[error("http error: {0}")]
    Http(String),

    /// Upstream returned a non-2xx status that was not retried (4xx).
    #[error("upstream status {status}: {body}")]
    UpstreamStatus { status: u16, body: String },

    /// Upstream returned a malformed / unexpected body.
    #[error("decode error: {0}")]
    Decode(String),

    /// Request deadline exceeded.
    #[error("timeout")]
    Timeout,

    /// Invalid input (e.g. empty candidate list, candidate too large).
    #[error("invalid input: {0}")]
    Invalid(String),

    /// Configuration problem (missing api key, bad url, etc).
    #[error("config error: {0}")]
    Config(String),
}

/// Reranker contract.
///
/// * `rerank` MUST return exactly `candidates.len()` scores.
/// * Scores MUST be in `[0.0, 1.0]` (best-effort; upstream may exceed —
///   implementations clamp).
/// * Output order MUST be aligned with input order. Callers sort / take-K.
/// * Implementations MUST be safe to share across tasks (`Send + Sync`).
#[async_trait]
pub trait Reranker: Send + Sync {
    /// Stable identifier for this reranker (for logging / metrics).
    fn id(&self) -> &str;

    /// Score every candidate against `query`.
    async fn rerank(&self, query: &str, candidates: Vec<String>) -> Result<Vec<f32>, RerankError>;
}

pub use config::{build, RerankerConfig, RerankerKind};
pub use fake::FakeReranker;
pub use siliconflow::{SiliconFlowReranker, SiliconFlowRerankerConfig};
