//! Token counting abstraction.
//!
//! Onyx analog: `backend/onyx/natural_language_processing/utils.py`
//! (`BaseTokenizer`, `count_tokens`, `split_text_by_tokens`).
//!
//! Two implementations:
//!
//! 1. [`TiktokenTokenizer`] — real `cl100k_base` BPE. This is what Onyx
//!    uses in production for every OpenAI-family embedder. The BPE table
//!    is ~9 MB and is loaded once per process.
//! 2. [`StubTokenizer`] — deterministic word-count * 1.3 approximation for
//!    tests that don't want the 500 ms of BPE init on every run. Exists
//!    solely to keep the unit test suite fast; **do not use in
//!    production**.

use std::sync::Arc;

use text_splitter::ChunkSizer;
use tiktoken_rs::CoreBPE;

/// Common token-counting interface used by the chunker.
///
/// Mirrors `BaseTokenizer` in Onyx
/// (`backend/onyx/natural_language_processing/utils.py`).
///
/// Implementations must also be `ChunkSizer` so the text-splitter crate
/// can size chunks using the same token function (avoiding drift between
/// the accumulator's count and the splitter's cut points).
pub trait Tokenizer: ChunkSizer + Send + Sync {
    fn count(&self, text: &str) -> usize;
}

/// Real tiktoken tokenizer with `cl100k_base` BPE. This is the shared
/// default across every OpenAI-family embedder in Onyx.
#[derive(Clone)]
pub struct TiktokenTokenizer {
    bpe: Arc<CoreBPE>,
}

impl TiktokenTokenizer {
    /// Load the `cl100k_base` encoding. Panics only if the embedded
    /// tiktoken asset is missing — that would be a packaging bug, not a
    /// runtime condition.
    pub fn cl100k_base() -> Self {
        let bpe = tiktoken_rs::cl100k_base()
            .expect("cl100k_base BPE table ships with tiktoken-rs — packaging bug if this fails");
        Self { bpe: Arc::new(bpe) }
    }
}

impl Tokenizer for TiktokenTokenizer {
    fn count(&self, text: &str) -> usize {
        self.bpe.encode_with_special_tokens(text).len()
    }
}

// `text_splitter` wants a `ChunkSizer` impl; we forward to the real count.
impl ChunkSizer for TiktokenTokenizer {
    fn size(&self, chunk: &str) -> usize {
        self.count(chunk)
    }
}

/// Tests-only approximation. word-count * 1.3 is the historical rule of
/// thumb for English + GPT BPE (average subword:word ratio ≈ 1.3).
///
/// This is never instantiated by production code; it exists to keep the
/// chunker's unit tests free of the 100 ms+ cl100k_base init.
#[derive(Clone, Copy, Default)]
pub struct StubTokenizer;

impl StubTokenizer {
    pub fn new() -> Self {
        Self
    }
}

impl Tokenizer for StubTokenizer {
    fn count(&self, text: &str) -> usize {
        if text.is_empty() {
            return 0;
        }
        let words = text.split_whitespace().count();
        // Ceiling, not floor, so "hello" (1 word) → 2 tokens, matching the
        // practical observation that even single words tokenise to ≥ 1
        // chunk token + often a space variant.
        ((words as f32) * 1.3).ceil() as usize
    }
}

impl ChunkSizer for StubTokenizer {
    fn size(&self, chunk: &str) -> usize {
        self.count(chunk)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Business value: the stub must monotonically track word count so
    // AccumulatorState behaviour (which buckets by token total) is
    // deterministic in tests.
    #[test]
    fn stub_tokenizer_is_monotonic_in_words() {
        let t = StubTokenizer::new();
        let one = t.count("hello");
        let ten = t.count("hello ".repeat(10).trim_end());
        let hundred = t.count("hello ".repeat(100).trim_end());
        assert!(ten > one);
        assert!(hundred > ten);
    }

    // Business value: empty text is 0 tokens, matching Onyx's
    // `len(tokenizer.encode(""))` behaviour.
    #[test]
    fn stub_tokenizer_empty_string_is_zero() {
        let t = StubTokenizer::new();
        assert_eq!(t.count(""), 0);
    }
}
