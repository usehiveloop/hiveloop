//! Chunker using `text-splitter` + `tiktoken-rs`, Onyx-compatible semantics.
//!
//! # Status: Tranche 2E (implementation)
//!
//! This crate ports Onyx's chunker as-is, including
//!   * the full `AccumulatorState` multi-section packer
//!     (`backend/onyx/indexing/chunking/section_chunker.py`,
//!     `text_section_chunker.py`),
//!   * mini-chunks (`MINI_CHUNK_SIZE = 150`,
//!     `backend/onyx/configs/app_configs.py:821`),
//!   * Onyx's title-prefix / metadata-suffix budgeting
//!     (`backend/onyx/indexing/chunker.py:189-275`),
//!   * Onyx's `clean_text` Unicode filter
//!     (`backend/onyx/utils/text_processing.py:252-259`).
//!
//! # Deviation vs Onyx upstream (locked by plans/onyx-port-phase2.md §5.3)
//!
//! - `CHUNK_OVERLAP` is 102 (20% of 512) instead of 0. See
//!   `constants::HIVELOOP_CHUNK_OVERLAP_TOKENS`.
//!
//! Every other semantic is preserved. See `services/rag-engine/DECISIONS.md`
//! for substitution details where a direct Python port wasn't viable.

pub mod accumulator;
pub mod constants;
pub mod mini_chunks;
pub mod splitter;
pub mod tokenizer;

use std::sync::OnceLock;

use regex::Regex;
use unicode_normalization::UnicodeNormalization;

use crate::accumulator::{chunk_text_section, AccumulatorState, ChunkPayload};
use crate::constants::{
    BLURB_SIZE, CHUNK_MIN_CONTENT, DOC_EMBEDDING_CONTEXT_SIZE, HIVELOOP_CHUNK_OVERLAP_TOKENS,
    MAX_METADATA_PERCENTAGE, MINI_CHUNK_SIZE, RETURN_SEPARATOR,
};
use crate::mini_chunks::build_mini_chunks;
use crate::splitter::split_with_tokenizer;
use crate::tokenizer::Tokenizer;

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

/// Input document. Matches the proto `DocumentToIngest`
/// (`proto/rag_engine.proto:83-94`) plus Onyx's
/// `IndexingDocument` shape (`backend/onyx/connectors/models.py`).
#[derive(Debug, Clone)]
pub struct Document {
    pub doc_id: String,
    /// Human-readable title. Mirrors Onyx `semantic_identifier`.
    pub title: Option<String>,
    pub sections: Vec<Section>,
    /// Flat string-string metadata; typed metadata lives in Postgres on
    /// the Go side.
    pub metadata: Vec<(String, String)>,
}

/// One section of a document. Matches the proto `Section`
/// (`proto/rag_engine.proto:96-100`).
#[derive(Debug, Clone)]
pub struct Section {
    pub text: String,
    pub link: Option<String>,
    /// Optional section heading (proto field `title`).
    pub title: Option<String>,
}

/// Final chunk produced by the chunker. Mirrors the relevant subset of
/// Onyx `DocAwareChunk` (`backend/onyx/indexing/models.py`) — only the
/// fields the downstream Lance writer needs.
#[derive(Debug, Clone)]
pub struct Chunk {
    pub chunk_index: u32,
    /// The main chunk body written to the `content` column.
    pub content: String,
    /// What actually gets embedded — `title_prefix + content + metadata_suffix_semantic`.
    pub content_for_embedding: String,
    /// Short preview (first `BLURB_SIZE` tokens of content).
    pub blurb: String,
    /// Source links with byte offsets into `content`.
    pub source_links: Vec<(usize, String)>,
    /// Token count of `content_for_embedding`. u32 matches the proto wire type.
    pub chunk_tok_count: u32,
    /// When mini-chunks are enabled, the per-chunk sub-chunks.
    pub mini_chunks: Option<Vec<String>>,
    /// True when this chunk continues the logical section above.
    pub is_continuation: bool,
}

/// Chunker configuration. Every field maps 1-1 to an Onyx constant
/// (see `constants.rs`); the defaults produce Hiveloop-parity output.
#[derive(Debug, Clone)]
pub struct ChunkerConfig {
    pub chunk_token_limit: usize,
    pub chunk_overlap_tokens: usize,
    pub mini_chunk_size: usize,
    pub enable_mini_chunks: bool,
    pub blurb_size: usize,
    pub include_metadata: bool,
}

impl Default for ChunkerConfig {
    fn default() -> Self {
        Self {
            chunk_token_limit: DOC_EMBEDDING_CONTEXT_SIZE,
            chunk_overlap_tokens: HIVELOOP_CHUNK_OVERLAP_TOKENS,
            mini_chunk_size: MINI_CHUNK_SIZE,
            // Onyx flips this with `enable_multipass`; we default ON since
            // the locked retrieval flow uses mini-chunks.
            enable_mini_chunks: true,
            blurb_size: BLURB_SIZE,
            include_metadata: true,
        }
    }
}

// ---------------------------------------------------------------------------
// Chunker
// ---------------------------------------------------------------------------

/// The chunker. Stateless apart from the tokenizer and config; one
/// instance can be shared across the ingest handler's concurrency via
/// `Arc<Chunker<T>>`.
#[derive(Clone)]
pub struct Chunker<T: Tokenizer + Clone + 'static> {
    tokenizer: T,
    cfg: ChunkerConfig,
}

impl<T: Tokenizer + Clone + 'static> Chunker<T> {
    pub fn new(tokenizer: T, cfg: ChunkerConfig) -> Self {
        Self { tokenizer, cfg }
    }

    pub fn config(&self) -> &ChunkerConfig {
        &self.cfg
    }

    /// Chunk one document. Direct port of
    /// `Chunker._handle_single_document`
    /// (`backend/onyx/indexing/chunker.py:189-285`).
    pub fn chunk(&self, doc: &Document) -> Vec<Chunk> {
        // ---------- Title prep ---------- Onyx chunker.py:196-202
        let raw_title = doc.title.clone().unwrap_or_default();
        let title = extract_blurb(&raw_title, self.cfg.blurb_size, &self.tokenizer);
        let title_prefix = if title.is_empty() {
            String::new()
        } else {
            format!("{title}{RETURN_SEPARATOR}")
        };
        let title_tokens = self.tokenizer.count(&title_prefix);

        // ---------- Metadata prep ---------- Onyx chunker.py:205-220
        let mut metadata_suffix_semantic = String::new();
        let mut metadata_tokens = 0usize;
        if self.cfg.include_metadata {
            let (semantic, _keyword) = metadata_suffix_for_document_index(&doc.metadata, true);
            metadata_suffix_semantic = semantic;
            metadata_tokens = self.tokenizer.count(&metadata_suffix_semantic);

            // Onyx chunker.py:218-220 — if metadata is > 25% of chunk budget, drop it.
            let cap = (self.cfg.chunk_token_limit as f32 * MAX_METADATA_PERCENTAGE) as usize;
            if metadata_tokens >= cap {
                metadata_suffix_semantic.clear();
                metadata_tokens = 0;
            }
        }

        // ---------- Content budget ---------- Onyx chunker.py:244-263
        let mut content_token_limit = self
            .cfg
            .chunk_token_limit
            .saturating_sub(title_tokens)
            .saturating_sub(metadata_tokens);

        let mut effective_title_prefix = title_prefix.clone();
        let mut effective_metadata_suffix = metadata_suffix_semantic.clone();

        if content_token_limit <= CHUNK_MIN_CONTENT {
            // Not enough room — drop the prefix/suffix and use the full
            // chunk for body. Onyx chunker.py:259-263.
            content_token_limit = self.cfg.chunk_token_limit;
            effective_title_prefix.clear();
            effective_metadata_suffix.clear();
        }

        // ---------- Collect section payloads ---------- Onyx document_chunker.py:76-107
        let mut accumulator = AccumulatorState::new();
        let mut payloads: Vec<ChunkPayload> = Vec::new();

        for (section_idx, section) in doc.sections.iter().enumerate() {
            let section_text = clean_text(&section.text);

            // Onyx document_chunker.py:88-93 — skip empty sections unless
            // they're the first one and we have a title to carry.
            if section_text.is_empty() && (doc.title.is_none() || section_idx > 0) {
                continue;
            }

            let link = section.link.clone().unwrap_or_default();
            let out = chunk_text_section(
                &section_text,
                &link,
                accumulator,
                content_token_limit,
                self.cfg.chunk_overlap_tokens,
                &self.tokenizer,
            );
            payloads.extend(out.payloads);
            accumulator = out.accumulator;
        }

        // Final flush. Onyx document_chunker.py:104-105.
        payloads.extend(accumulator.flush_to_list());

        // Onyx document_chunker.py:60-62 — "title-only" docs still get one
        // empty payload so they remain searchable via title_prefix.
        //
        // DEVIATION: Onyx unconditionally appends an empty payload when
        // `payloads` is empty. We gate this on "has a non-empty title
        // prefix", because an empty doc with no title yields a useless
        // all-empty record that pollutes the index without adding
        // retrieval signal. This is the Hiveloop refinement documented
        // in DECISIONS.md.
        if payloads.is_empty() && !effective_title_prefix.is_empty() {
            payloads.push(ChunkPayload {
                text: String::new(),
                links: vec![(0, String::new())],
                is_continuation: false,
            });
        }

        // ---------- Materialise chunks ----------
        payloads
            .into_iter()
            .enumerate()
            .map(|(idx, payload)| {
                let blurb = extract_blurb(&payload.text, self.cfg.blurb_size, &self.tokenizer);
                let content_for_embedding = format!(
                    "{effective_title_prefix}{body}{effective_metadata_suffix}",
                    body = payload.text,
                );
                let chunk_tok_count = self.tokenizer.count(&content_for_embedding) as u32;

                let mini = if self.cfg.enable_mini_chunks {
                    build_mini_chunks(&payload.text, self.cfg.mini_chunk_size, &self.tokenizer)
                } else {
                    None
                };

                Chunk {
                    chunk_index: idx as u32,
                    content: payload.text,
                    content_for_embedding,
                    blurb,
                    source_links: payload.links,
                    chunk_tok_count,
                    mini_chunks: mini,
                    is_continuation: payload.is_continuation,
                }
            })
            .collect()
    }
}

// ---------------------------------------------------------------------------
// Helpers (direct ports from Onyx)
// ---------------------------------------------------------------------------

/// Port of Onyx `extract_blurb`
/// (`backend/onyx/indexing/chunking/section_chunker.py:15-19`).
/// Returns the first `blurb_size`-token chunk of the input, or empty.
fn extract_blurb<T: Tokenizer + Clone + 'static>(
    text: &str,
    blurb_size: usize,
    tokenizer: &T,
) -> String {
    if text.is_empty() {
        return String::new();
    }
    let pieces = split_with_tokenizer(text, tokenizer, blurb_size, 0);
    pieces.into_iter().next().unwrap_or_default()
}

/// Port of Onyx `_get_metadata_suffix_for_document_index`
/// (`backend/onyx/indexing/chunker.py:37-67`). Produces a
/// `(semantic, keyword)` pair. Note: `metadata` here is `Vec<(K,V)>`
/// rather than a dict, so iteration order is deterministic and caller-
/// controlled — matches Python 3.7+ dict-insertion-order semantics.
fn metadata_suffix_for_document_index(
    metadata: &[(String, String)],
    include_separator: bool,
) -> (String, String) {
    if metadata.is_empty() {
        return (String::new(), String::new());
    }

    let mut metadata_str = String::from("Metadata:\n");
    let mut values: Vec<String> = Vec::new();
    for (key, value) in metadata {
        metadata_str.push('\t');
        metadata_str.push_str(key);
        metadata_str.push_str(" - ");
        metadata_str.push_str(value);
        metadata_str.push('\n');
        values.push(value.clone());
    }
    let metadata_semantic = metadata_str.trim_end().to_string();
    let metadata_keyword = values.join(" ");

    if include_separator {
        (
            format!("{RETURN_SEPARATOR}{metadata_semantic}"),
            format!("{RETURN_SEPARATOR}{metadata_keyword}"),
        )
    } else {
        (metadata_semantic, metadata_keyword)
    }
}

/// Port of Onyx `clean_text`
/// (`backend/onyx/utils/text_processing.py:252-259`).
/// 1. NFKC-normalise (DEVIATION — see DECISIONS.md).
/// 2. Strip the `_INITIAL_FILTER` Unicode ranges (Specials, Emoticons,
///    General Punctuation, Arrows, Dingbats).
/// 3. Strip control chars except `\n` and `\t`.
pub fn clean_text(text: &str) -> String {
    let normalized: String = text.nfkc().collect();
    let regex = initial_filter();
    let stripped = regex.replace_all(&normalized, "");
    stripped
        .chars()
        .filter(|&c| c >= ' ' || c == '\n' || c == '\t')
        .collect()
}

fn initial_filter() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        // Mirrors `_INITIAL_FILTER` (backend/onyx/utils/text_processing.py:54-63):
        //   \U0000fff0-\U0000ffff  Specials
        //   \U0001f000-\U0001f9ff  Emoticons
        //   \U00002000-\U0000206f  General Punctuation
        //   \U00002190-\U000021ff  Arrows
        //   \U00002700-\U000027bf  Dingbats
        Regex::new(
            "[\u{fff0}-\u{ffff}\u{1f000}-\u{1f9ff}\u{2000}-\u{206f}\u{2190}-\u{21ff}\u{2700}-\u{27bf}]+",
        )
        .expect("static regex compiles")
    })
}

// Compile-time assertion that `Chunker` is Send + Sync + 'static when `T`
// is. Business value: one chunker instance shared across the ingest
// handler's concurrency.
const _: fn() = || {
    fn assert_send_sync<X: Send + Sync>() {}
    assert_send_sync::<Chunker<crate::tokenizer::StubTokenizer>>();
    assert_send_sync::<Chunker<crate::tokenizer::TiktokenTokenizer>>();
};
