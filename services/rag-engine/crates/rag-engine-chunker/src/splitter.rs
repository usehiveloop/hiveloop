//! Text splitter wrapper around the `text-splitter` crate.
//!
//! Onyx upstream uses `chonkie.SentenceChunker` (see
//! `backend/onyx/indexing/chunker.py:157-178`). `chonkie` is Python-only;
//! the Rust-native equivalent closest in semantics is `text-splitter`,
//! which also splits on sentence boundaries and supports a
//! tokenizer-driven chunk size. See `services/rag-engine/DECISIONS.md`
//! for the full substitution rationale.

use text_splitter::{ChunkConfig, TextSplitter};

use crate::tokenizer::Tokenizer;

/// Split `text` into pieces of ≤ `max_tokens` tokens each, with
/// `overlap_tokens` of overlap at boundaries. Mirrors the role of
/// `SentenceChunker.chunk()` in Onyx.
pub fn split_with_tokenizer<T: Tokenizer + Clone + 'static>(
    text: &str,
    tokenizer: &T,
    max_tokens: usize,
    overlap_tokens: usize,
) -> Vec<String> {
    if text.is_empty() {
        return Vec::new();
    }

    // `text-splitter` rejects overlap >= capacity. Clamp as a safety net;
    // constants in this crate already guarantee overlap < capacity, but
    // callers can pass custom `ChunkerConfig` values.
    let safe_overlap = overlap_tokens.min(max_tokens.saturating_sub(1));

    let cfg = ChunkConfig::new(max_tokens)
        .with_sizer(tokenizer.clone())
        .with_overlap(safe_overlap)
        .expect("overlap already clamped below capacity");

    let splitter = TextSplitter::new(cfg);
    splitter.chunks(text).map(|s| s.to_string()).collect()
}
