//! Embedder trait + OpenAI-compatible implementation + deterministic FakeEmbedder.
//!
//! # Tranche 2C (refactored)
//!
//! This crate exposes a narrow async trait, an `async-openai`-backed
//! implementation that targets any provider speaking the OpenAI
//! `/v1/embeddings` surface (SiliconFlow, OpenRouter, Groq, OpenAI,
//! Together, etc. — selected purely by `LLM_API_URL` / `LLM_API_KEY` /
//! `LLM_MODEL`), a deterministic `FakeEmbedder` used by tests, a bounded-
//! concurrency batching helper, and a figment+env-driven factory.
//!
//! Per `TESTING.md`, `FakeEmbedder` is the ONLY mock-like construct
//! permitted in production code. Test-time upstream mocks use `wiremock`,
//! which is a real HTTP server that happens to be controlled by us.

mod batching;
mod config;
mod errors;
mod fake;
mod openai_compat;
mod types;

pub use batching::embed_batched;
pub use config::{
    build, load_from_env_and_file, EmbedderConfig, FakeConfig, OpenAICompatConfig, Provider,
};
pub use errors::EmbedError;
pub use fake::FakeEmbedder;
pub use openai_compat::{OpenAICompatEmbedder, OpenAICompatOptions};
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
    /// Stable identifier (e.g. `"openai-compat:Qwen/Qwen3-Embedding-4B"`).
    /// Used for logging, metrics tagging, and dataset-name derivation in
    /// the storage layer.
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
