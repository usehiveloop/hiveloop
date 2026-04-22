//! Port of Onyx's `AccumulatorState` multi-section packer.
//!
//! Onyx source:
//! - `backend/onyx/indexing/chunking/section_chunker.py:73-85`
//!   (`AccumulatorState`, `ChunkPayload`)
//! - `backend/onyx/indexing/chunking/text_section_chunker.py:33-117`
//!   (`TextChunker.chunk_section` + `_handle_oversized_section`)
//! - `backend/onyx/indexing/chunking/document_chunker.py:76-107`
//!   (`_collect_section_payloads` driver loop)
//!
//! # Why this exists
//!
//! Naive per-section chunking produces a blizzard of tiny chunks for
//! documents that have many short sections (Confluence bullet lists,
//! Slack threads, e-mail quotes). `AccumulatorState` packs consecutive
//! small sections into a single chunk until the token budget is full,
//! then flushes. Retrieval quality is materially better because chunks
//! sit near the embedding model's sweet spot instead of at 20 tokens each.
//!
//! The port is intentionally line-for-line. Deviations are documented
//! inline with `// DEVIATION:` comments.

use crate::constants::SECTION_SEPARATOR;
use crate::splitter::split_with_tokenizer;
use crate::tokenizer::Tokenizer;

/// A section-local chunk payload without document-scoped fields.
///
/// Mirrors Onyx `ChunkPayload`
/// (`backend/onyx/indexing/chunking/section_chunker.py:31-70`).
///
/// `links` maps UTF-8 byte offsets inside `text` to the upstream section
/// link. When the accumulator concatenates several sections, each section
/// adds one entry at its concatenation offset.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ChunkPayload {
    pub text: String,
    /// offset-in-text → source_link. Matches Onyx `ChunkPayload.links`.
    pub links: Vec<(usize, String)>,
    pub is_continuation: bool,
}

/// Cross-section text buffer threaded through the chunker.
///
/// Direct port of
/// `backend/onyx/indexing/chunking/section_chunker.py:73-85`.
#[derive(Debug, Clone, Default)]
pub struct AccumulatorState {
    pub text: String,
    pub link_offsets: Vec<(usize, String)>,
}

impl AccumulatorState {
    pub fn new() -> Self {
        Self::default()
    }

    /// Onyx `AccumulatorState.is_empty` — trims then checks.
    pub fn is_empty(&self) -> bool {
        self.text.trim().is_empty()
    }

    /// Onyx `AccumulatorState.flush_to_list` — returns 0 or 1 payload.
    pub fn flush_to_list(&self) -> Vec<ChunkPayload> {
        if self.is_empty() {
            return Vec::new();
        }
        vec![ChunkPayload {
            text: self.text.clone(),
            links: self.link_offsets.clone(),
            is_continuation: false,
        }]
    }
}

/// Result of processing one section. Mirrors Onyx `SectionChunkerOutput`.
#[derive(Debug, Clone, Default)]
pub struct SectionChunkerOutput {
    pub payloads: Vec<ChunkPayload>,
    pub accumulator: AccumulatorState,
}

/// Process one text section against an `AccumulatorState`.
///
/// Direct port of `TextChunker.chunk_section`
/// (`backend/onyx/indexing/chunking/text_section_chunker.py:33-77`)
/// with the oversized-section fallback
/// (`_handle_oversized_section`, lines 79-117).
pub fn chunk_text_section<T: Tokenizer + Clone + 'static>(
    section_text: &str,
    section_link: &str,
    accumulator: AccumulatorState,
    content_token_limit: usize,
    overlap_tokens: usize,
    tokenizer: &T,
) -> SectionChunkerOutput {
    let section_token_count = tokenizer.count(section_text);

    // Oversized — flush buffer and split the section.
    // Onyx `text_section_chunker.py:44`.
    if section_token_count > content_token_limit {
        return handle_oversized_section(
            section_text,
            section_link,
            accumulator,
            content_token_limit,
            overlap_tokens,
            tokenizer,
        );
    }

    let current_token_count = tokenizer.count(&accumulator.text);
    // Onyx counts the separator tokens every time via `section_separator_token_count`.
    // We compute it here; `SECTION_SEPARATOR = "\n\n"` is 1 token in cl100k_base.
    let separator_tokens = if accumulator.text.is_empty() {
        0
    } else {
        tokenizer.count(SECTION_SEPARATOR)
    };
    let next_section_tokens = separator_tokens + section_token_count;

    // Fits — extend the accumulator.
    // Onyx `text_section_chunker.py:56-68`.
    if next_section_tokens + current_token_count <= content_token_limit {
        // DEVIATION: Onyx uses `shared_precompare_cleanup(accumulator.text)`
        // which lowercases + strips punctuation to compute the offset. The
        // reason Onyx does that is to match LLM-rewritten quotes during
        // citation. Here we store raw byte offsets in the original
        // accumulator text; the Rust downstream consumer uses byte indices,
        // not citation-match indices. See DECISIONS.md §AccumulatorState
        // offset semantics.
        let offset = accumulator.text.len();
        let mut new_text = accumulator.text;
        if !new_text.is_empty() {
            new_text.push_str(SECTION_SEPARATOR);
        }
        new_text.push_str(section_text);

        let mut link_offsets = accumulator.link_offsets;
        link_offsets.push((offset, section_link.to_string()));

        return SectionChunkerOutput {
            payloads: Vec::new(),
            accumulator: AccumulatorState {
                text: new_text,
                link_offsets,
            },
        };
    }

    // Doesn't fit — flush buffer and restart with this section.
    // Onyx `text_section_chunker.py:71-77`.
    SectionChunkerOutput {
        payloads: accumulator.flush_to_list(),
        accumulator: AccumulatorState {
            text: section_text.to_string(),
            link_offsets: vec![(0, section_link.to_string())],
        },
    }
}

/// Direct port of
/// `TextChunker._handle_oversized_section`
/// (`backend/onyx/indexing/chunking/text_section_chunker.py:79-117`).
///
/// DEVIATION: we do not port the `STRICT_CHUNK_TOKEN_LIMIT` +
/// `split_text_by_tokens` fallback (lines 90-104). `text-splitter`
/// already honours the token capacity as a hard ceiling, so the second
/// "smaller_chunks" pass Onyx runs against chonkie output is not needed
/// for our splitter. Documented in DECISIONS.md.
fn handle_oversized_section<T: Tokenizer + Clone + 'static>(
    section_text: &str,
    section_link: &str,
    accumulator: AccumulatorState,
    content_token_limit: usize,
    overlap_tokens: usize,
    tokenizer: &T,
) -> SectionChunkerOutput {
    let mut payloads = accumulator.flush_to_list();

    let split_texts =
        split_with_tokenizer(section_text, tokenizer, content_token_limit, overlap_tokens);

    for (i, split_text) in split_texts.iter().enumerate() {
        payloads.push(ChunkPayload {
            text: split_text.clone(),
            links: vec![(0, section_link.to_string())],
            is_continuation: i != 0,
        });
    }

    SectionChunkerOutput {
        payloads,
        accumulator: AccumulatorState::new(),
    }
}
