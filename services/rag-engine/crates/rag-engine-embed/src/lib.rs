//! Embedder trait + SiliconFlow implementation + deterministic FakeEmbedder.
//!
//! # Tranche 2C
//!
//! This crate exposes a narrow async trait, an OpenAI-compatible HTTP-backed
//! implementation aimed at SiliconFlow (but usable against any compatible
//! endpoint), a deterministic `FakeEmbedder` used by tests, a bounded-
//! concurrency batching helper, and a figment-driven factory.
//!
//! Per `TESTING.md`, `FakeEmbedder` is the ONLY mock-like construct
//! permitted in the codebase. Every other test talks to real services.

mod batching;
mod config;
mod errors;
mod fake;
mod siliconflow;
mod types;

pub use batching::embed_batched;
pub use config::{build, EmbedderConfig, FakeConfig, Provider, SiliconFlowConfig};
pub use errors::EmbedError;
pub use fake::FakeEmbedder;
pub use siliconflow::{SiliconFlowEmbedder, SiliconFlowOptions};
pub use types::EmbedKind;

use async_trait::async_trait;

/// Core embedder abstraction. All embedder implementations — real and
/// fake — implement this trait.
///
/// Designed around a single `embed` entry point that accepts a batch and
/// a kind. Splitting query vs. passage into an enum (rather than separate
/// methods) keeps the caller code identical for both paths; the impl
/// decides whether to apply a prefix. Matches Onyx's
/// `IndexingEmbedder`/`EmbeddingModel` split but collapses it into one
/// method for caller simplicity.
#[async_trait]
pub trait Embedder: Send + Sync {
    /// Stable identifier (e.g. `"siliconflow:qwen3-embedding-4b"`). Used
    /// for logging, metrics tagging, and dataset-name derivation in the
    /// storage layer.
    fn id(&self) -> &str;

    /// Output dimension. Checked by callers against the configured
    /// dataset dim; a mismatch is a terminal error (`EmbedError::
    /// InvalidResponse` from the impl).
    fn dimension(&self) -> u32;

    /// Maximum number of tokens (per input string) the embedder accepts
    /// before returning an error. Chunking upstream uses this to bound
    /// chunk size.
    fn max_input_tokens(&self) -> u32;

    /// Embed a batch of texts. Order of outputs matches order of inputs.
    async fn embed(&self, texts: Vec<String>, kind: EmbedKind)
        -> Result<Vec<Vec<f32>>, EmbedError>;
}
