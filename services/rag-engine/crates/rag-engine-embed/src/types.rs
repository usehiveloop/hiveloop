//! Public type surface of the embed crate.
//!
//! Wire-level types for the OpenAI `/v1/embeddings` surface live inside
//! the `async-openai` crate now (see `openai_compat.rs`). This module only
//! carries the caller-facing `EmbedKind` enum.

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
