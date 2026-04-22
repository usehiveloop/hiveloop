//! Wire-level types for the OpenAI-compatible `/v1/embeddings` surface.
//!
//! SiliconFlow is drop-in compatible with OpenAI's embedding API, so the
//! request/response shape is shared. Kept `pub(crate)` because nothing
//! outside this crate should speak the wire format — they go through the
//! `Embedder` trait.

use serde::{Deserialize, Serialize};

/// Distinguishes whether the embedding is for storage (documents being
/// indexed) or for querying (a user's search string). Qwen3 models expect
/// different prefixes per kind; the choice is made at the call site and
/// carried down to the wire.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EmbedKind {
    /// Document being indexed. Qwen prefix: `"passage: "`.
    Passage,
    /// User query. Qwen prefix: `"query: "`.
    Query,
}

/// OpenAI-compatible embeddings request body.
#[derive(Debug, Serialize)]
pub(crate) struct EmbeddingRequest<'a> {
    pub model: &'a str,
    pub input: Vec<String>,
    pub encoding_format: &'static str,
}

/// OpenAI-compatible embeddings response body.
#[derive(Debug, Deserialize)]
pub(crate) struct EmbeddingResponse {
    pub data: Vec<EmbeddingRow>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct EmbeddingRow {
    pub embedding: Vec<f32>,
    #[allow(dead_code)]
    #[serde(default)]
    pub index: usize,
}
