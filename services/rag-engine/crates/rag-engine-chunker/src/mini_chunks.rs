//! Mini-chunk generation.
//!
//! Onyx source:
//! - `backend/onyx/indexing/chunking/section_chunker.py:22-28`
//!   (`get_mini_chunk_texts`)
//! - `backend/onyx/indexing/chunker.py:171-180`
//!   (`mini_chunk_splitter` — constructed only when `enable_multipass=True`)
//!
//! # What are mini-chunks?
//!
//! For every normal 512-token chunk, Onyx optionally splits its body
//! further into ~150-token mini-chunks (`MINI_CHUNK_SIZE = 150`,
//! `backend/onyx/configs/app_configs.py:821`). These mini-chunks live
//! alongside the normal chunk and provide higher-precision retrieval —
//! short queries match small windows better than large ones.
//!
//! Mini-chunks are stored on the parent chunk's record, not as
//! independent rows. That storage shape is preserved here: the Rust
//! `Chunk` type carries an `Option<Vec<String>>` that's `Some` only when
//! mini-chunks are enabled and the body is non-empty.

use crate::splitter::split_with_tokenizer;
use crate::tokenizer::Tokenizer;

/// Produce mini-chunks for a chunk body. Returns `None` when mini-chunks
/// are not applicable (empty/whitespace body) — matches the `chunk_text.strip()`
/// guard in Onyx `section_chunker.py:26`.
pub fn build_mini_chunks<T: Tokenizer + Clone + 'static>(
    chunk_text: &str,
    mini_chunk_size: usize,
    tokenizer: &T,
) -> Option<Vec<String>> {
    if chunk_text.trim().is_empty() {
        return None;
    }
    // Onyx `SentenceChunker(chunk_size=mini_chunk_size, chunk_overlap=0,
    // return_type="texts")`. The mini-chunk splitter never overlaps —
    // that's intentional in Onyx (smaller windows benefit less from
    // overlap, and doubling storage cost is not worth it).
    let out = split_with_tokenizer(chunk_text, tokenizer, mini_chunk_size, 0);
    if out.is_empty() {
        None
    } else {
        Some(out)
    }
}
