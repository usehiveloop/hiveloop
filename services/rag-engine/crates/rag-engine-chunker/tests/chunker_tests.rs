//! Integration tests for `rag-engine-chunker`.
//!
//! These are pure-compute tests — the chunker has no infrastructure
//! dependencies, so we do not mock anything. Tests exercise the real
//! `text-splitter` crate with either the real `cl100k_base` tokenizer
//! (for token-count parity checks) or the `StubTokenizer` (for fast
//! word-counted budgeting tests).
//!
//! Onyx parity: every test pins behaviour we depend on from
//! `backend/onyx/indexing/chunker.py` and
//! `backend/onyx/indexing/chunking/*.py`.

use rag_engine_chunker::{
    clean_text,
    constants::{HIVELOOP_CHUNK_OVERLAP_TOKENS, MINI_CHUNK_SIZE},
    tokenizer::{StubTokenizer, TiktokenTokenizer, Tokenizer},
    Chunker, ChunkerConfig, Document, Section,
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Build a string whose cl100k_base token count is approximately `words`
/// (each "word" in the base list tokenizes to 1 token, so 1 word ≈ 1
/// token). Callers use `TiktokenTokenizer::count` to assert the exact
/// total if they need precision.
fn long_text(words: usize) -> String {
    let base = ["lorem ", "ipsum ", "dolor ", "sit ", "amet "];
    let mut out = String::with_capacity(words * 6);
    for i in 0..words {
        out.push_str(base[i % base.len()]);
    }
    out.trim_end().to_string()
}

fn make_doc(title: Option<&str>, sections: Vec<(&str, Option<&str>)>) -> Document {
    Document {
        doc_id: "doc-under-test".to_string(),
        title: title.map(|s| s.to_string()),
        sections: sections
            .into_iter()
            .map(|(text, link)| Section {
                text: text.to_string(),
                link: link.map(|s| s.to_string()),
                title: None,
            })
            .collect(),
        metadata: Vec::new(),
    }
}

fn tiktoken_chunker(cfg: ChunkerConfig) -> Chunker<TiktokenTokenizer> {
    Chunker::new(TiktokenTokenizer::cl100k_base(), cfg)
}

fn stub_chunker(cfg: ChunkerConfig) -> Chunker<StubTokenizer> {
    Chunker::new(StubTokenizer::new(), cfg)
}

// ---------------------------------------------------------------------------
// 1. test_chunker_produces_expected_count_for_known_input
// Business value: ~1500 tokens / 512 limit / 20% overlap should yield
// roughly 3 chunks. If this ever drops to 1 or explodes to 10, something
// fundamental has changed in the Onyx semantics we depend on.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_produces_expected_count_for_known_input() {
    let text = long_text(1500); // ~1500 tokens in cl100k_base
    let chunker = tiktoken_chunker(ChunkerConfig::default());
    let tok = TiktokenTokenizer::cl100k_base();
    let total = tok.count(&text);
    // Sanity: the test corpus should actually be ~1500 tokens.
    assert!(
        (1300..=1700).contains(&total),
        "test corpus size drifted: {total} tokens"
    );

    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    // With 512 limit + 102 overlap, chunks advance by ~410 tokens each.
    // 1500 / 410 ≈ 3.7 → expect 3–5 chunks.
    assert!(
        (3..=5).contains(&chunks.len()),
        "expected ~3-5 chunks, got {}",
        chunks.len()
    );
}

// ---------------------------------------------------------------------------
// 2. test_chunker_preserves_order
// Business value: downstream consumers rely on chunk_index to
// reconstruct documents; gaps or out-of-order indices break retrieval.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_preserves_order() {
    let text = long_text(1500);
    let chunker = tiktoken_chunker(ChunkerConfig::default());
    let chunks = chunker.chunk(&make_doc(Some("Title"), vec![(&text, None)]));
    for (i, c) in chunks.iter().enumerate() {
        assert_eq!(
            c.chunk_index, i as u32,
            "chunk {i} has index {}",
            c.chunk_index
        );
    }
}

// ---------------------------------------------------------------------------
// 3. test_chunker_respects_chunk_limit
// Business value: if chunks exceed 512 tokens, they can't be embedded by
// models with a 512-token context window. Onyx deviation: Onyx honours
// this too (chunker.py:133).
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_respects_chunk_limit() {
    let text = long_text(1500);
    let chunker = tiktoken_chunker(ChunkerConfig::default());
    let tok = TiktokenTokenizer::cl100k_base();
    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    for c in &chunks {
        let body_tokens = tok.count(&c.content);
        assert!(
            body_tokens <= 512,
            "chunk {} body has {} tokens (>512)",
            c.chunk_index,
            body_tokens
        );
    }
}

// ---------------------------------------------------------------------------
// 4. test_chunker_overlap_is_honored
// Business value: sentences straddling chunk boundaries stay retrievable
// — this is the whole point of the Hiveloop 20% overlap deviation.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_overlap_is_honored() {
    // Build a corpus of ~80 uniquely-numbered tokens so we can detect
    // shared prefix/suffix words between adjacent chunks deterministically.
    let words: Vec<String> = (0..800).map(|i| format!("word{i}")).collect();
    let text = words.join(" ");
    let chunker = tiktoken_chunker(ChunkerConfig::default());
    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    assert!(chunks.len() >= 2, "need ≥2 chunks to test overlap");

    // For at least one adjacent pair, the tail of chunk N and the head of
    // chunk N+1 must share at least one unique-token word. Setting the
    // bar at "some overlap detected" rather than an exact count — the
    // locked overlap is 102 tokens but BPE splits `word123` into
    // multi-token pieces, so we'd need a much more elaborate probe for
    // an exact value. The key business claim is overlap > 0, which
    // proves the config flows through.
    let mut any_overlap = false;
    for pair in chunks.windows(2) {
        let prev_tail: std::collections::HashSet<&str> =
            pair[0].content.split_whitespace().rev().take(100).collect();
        let next_head: std::collections::HashSet<&str> =
            pair[1].content.split_whitespace().take(100).collect();
        if prev_tail.intersection(&next_head).next().is_some() {
            any_overlap = true;
            break;
        }
    }
    assert!(
        any_overlap,
        "no overlap detected across any adjacent pair — overlap config not flowing through"
    );
    // Defensive: make sure the compiled-in value is still the locked 102.
    assert_eq!(HIVELOOP_CHUNK_OVERLAP_TOKENS, 102);
}

// ---------------------------------------------------------------------------
// 5. test_chunker_small_sections_merged_via_accumulator
// Business value: AccumulatorState is the headline retrieval-quality
// win. 10 tiny sections must pack into 1-2 chunks, not 10. If this ever
// produces 10 chunks, `chunk_text_section`'s "fits — extend" branch has
// broken.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_small_sections_merged_via_accumulator() {
    // 10 sections of 50 stub-tokens each (~38 words) → total 500 stub
    // tokens. With a 512 limit that packs into a single chunk.
    let section_text = "foo ".repeat(38); // 38 words → ~50 stub tokens
    let sections: Vec<(&str, Option<&str>)> =
        (0..10).map(|_| (section_text.as_str(), None)).collect();
    let chunker = stub_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let chunks = chunker.chunk(&make_doc(None, sections));
    assert!(
        chunks.len() <= 2,
        "accumulator should pack 10 small sections into 1-2 chunks, got {}",
        chunks.len()
    );
    assert!(!chunks.is_empty(), "must produce at least one chunk");
}

// ---------------------------------------------------------------------------
// 6. test_chunker_large_section_split
// Business value: the oversized-section fallback (Onyx
// text_section_chunker.py:79-117) must kick in; otherwise one giant
// section becomes one giant unembeddable chunk.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_large_section_split() {
    let text = long_text(2000); // ~2000 tokens
    let chunker = tiktoken_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    assert!(
        chunks.len() >= 3,
        "2000-token section must split to ≥3 chunks, got {}",
        chunks.len()
    );
}

// ---------------------------------------------------------------------------
// 7. test_chunker_empty_document_produces_zero_chunks
// Business value: empty documents with no title and no content should
// not produce a phantom chunk that pollutes the index. Onyx
// document_chunker.py:88-93 silences empty sections; with no title there
// is nothing to emit.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_empty_document_produces_zero_chunks() {
    let chunker = stub_chunker(ChunkerConfig::default());
    let chunks = chunker.chunk(&make_doc(None, vec![]));
    assert!(
        chunks.is_empty(),
        "empty doc produced {} chunks",
        chunks.len()
    );
}

// ---------------------------------------------------------------------------
// 8. test_chunker_empty_section_skipped
// Business value: an empty middle section must not produce a chunk of
// its own; matches Onyx document_chunker.py:88-93 exactly.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_empty_section_skipped() {
    let chunker = stub_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let chunks = chunker.chunk(&make_doc(
        Some("Title"),
        vec![
            ("first real body", None),
            ("", None),
            ("second real body", None),
        ],
    ));
    // The two real sections should merge into one accumulator flush.
    assert_eq!(chunks.len(), 1);
    let c = &chunks[0];
    assert!(c.content.contains("first real body"));
    assert!(c.content.contains("second real body"));
}

// ---------------------------------------------------------------------------
// 9. test_chunker_title_prefix_applied
// Business value: the embedded text must include the title so retrieval
// on title-bearing queries hits. Mirrors Onyx chunker.py:201.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_title_prefix_applied() {
    let chunker = stub_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let chunks = chunker.chunk(&make_doc(
        Some("Important Document"),
        vec![("a body sentence", None)],
    ));
    assert_eq!(chunks.len(), 1);
    assert!(
        chunks[0]
            .content_for_embedding
            .starts_with("Important Document"),
        "embedding text missing title prefix: {:?}",
        chunks[0].content_for_embedding
    );
    // The raw content column must NOT have the prefix — it mirrors Onyx
    // which stores prefix separately.
    assert!(!chunks[0].content.starts_with("Important Document"));
}

// ---------------------------------------------------------------------------
// 10. test_chunker_blurb_generated
// Business value: blurb is the UI preview; must be non-empty for a
// non-empty chunk and bounded by BLURB_SIZE tokens. Mirrors Onyx
// section_chunker.py:15-19.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_blurb_generated() {
    let text = long_text(300);
    let chunker = tiktoken_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let tok = TiktokenTokenizer::cl100k_base();
    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    assert!(!chunks[0].blurb.is_empty());
    let blurb_tokens = tok.count(&chunks[0].blurb);
    // BLURB_SIZE = 128; splitter can produce chunks a few tokens over due
    // to sentence-boundary preference. Guardrail: within 1.5x.
    assert!(
        blurb_tokens <= 200,
        "blurb exceeded 128-token budget noticeably: {blurb_tokens}"
    );
}

// ---------------------------------------------------------------------------
// 11. test_mini_chunks_generated_when_enabled
// Business value: mini-chunks provide high-precision retrieval. For a
// 500-token chunk with MINI_CHUNK_SIZE=150 we expect roughly
// ceil(500/150) ≈ 4 mini-chunks per chunk.
// ---------------------------------------------------------------------------
#[test]
fn test_mini_chunks_generated_when_enabled() {
    let text = long_text(200); // ~600 tokens
    let chunker = tiktoken_chunker(ChunkerConfig::default()); // mini on
    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    assert!(!chunks.is_empty());
    let mini = chunks[0]
        .mini_chunks
        .as_ref()
        .expect("mini_chunks should be Some when enabled");
    // Roughly N = ceil(chunk_body_tokens / 150). Body is ≤ 512 so N ≤ ~4.
    assert!(!mini.is_empty());
    assert!(
        mini.len() <= 6,
        "unexpected mini-chunk count: {}",
        mini.len()
    );
}

// ---------------------------------------------------------------------------
// 12. test_mini_chunks_respect_size
// Business value: oversized mini-chunks break the downstream retrieval
// buckets they were sized for.
// ---------------------------------------------------------------------------
#[test]
fn test_mini_chunks_respect_size() {
    let text = long_text(400);
    let chunker = tiktoken_chunker(ChunkerConfig::default());
    let tok = TiktokenTokenizer::cl100k_base();
    let chunks = chunker.chunk(&make_doc(None, vec![(&text, None)]));
    for c in &chunks {
        if let Some(mini) = &c.mini_chunks {
            for m in mini {
                let t = tok.count(m);
                assert!(
                    t <= MINI_CHUNK_SIZE,
                    "mini-chunk {:?} has {t} tokens (> {MINI_CHUNK_SIZE})",
                    m
                );
            }
        }
    }
}

// ---------------------------------------------------------------------------
// 13. test_mini_chunks_disabled_by_config
// Business value: we need a kill switch for storage-constrained
// deployments.
// ---------------------------------------------------------------------------
#[test]
fn test_mini_chunks_disabled_by_config() {
    let chunker = stub_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let chunks = chunker.chunk(&make_doc(None, vec![(&long_text(300), None)]));
    for c in &chunks {
        assert!(
            c.mini_chunks.is_none(),
            "mini-chunks produced despite config being off"
        );
    }
}

// ---------------------------------------------------------------------------
// 14. test_source_links_propagate
// Business value: source_links feed the citation UI. If they get lost
// during accumulator packing, we can't tell the user where a quote came
// from.
// ---------------------------------------------------------------------------
#[test]
fn test_source_links_propagate() {
    let chunker = stub_chunker(ChunkerConfig {
        enable_mini_chunks: false,
        ..ChunkerConfig::default()
    });
    let chunks = chunker.chunk(&make_doc(
        None,
        vec![
            ("body one", Some("https://example.com/a")),
            ("body two", Some("https://example.com/b")),
        ],
    ));
    // Either the sections pack into one chunk (two links at distinct
    // offsets) or into two chunks (one link each). Both are legal — the
    // invariant is that every link is preserved somewhere.
    let mut seen = std::collections::HashSet::new();
    for c in &chunks {
        for (_, link) in &c.source_links {
            seen.insert(link.clone());
        }
    }
    assert!(seen.contains("https://example.com/a"));
    assert!(seen.contains("https://example.com/b"));
}

// ---------------------------------------------------------------------------
// 15. test_chunker_deterministic
// Business value: same input must produce byte-identical chunks on
// repeated runs. If this fails, retries would write divergent records
// and break idempotency keys in the Lance writer.
// ---------------------------------------------------------------------------
#[test]
fn test_chunker_deterministic() {
    let text = long_text(400);
    let chunker = tiktoken_chunker(ChunkerConfig::default());
    let doc = make_doc(Some("T"), vec![(&text, Some("link"))]);
    let run1 = chunker.chunk(&doc);
    let run2 = chunker.chunk(&doc);
    assert_eq!(run1.len(), run2.len());
    for (a, b) in run1.iter().zip(run2.iter()) {
        assert_eq!(a.content, b.content);
        assert_eq!(a.content_for_embedding, b.content_for_embedding);
        assert_eq!(a.blurb, b.blurb);
        assert_eq!(a.source_links, b.source_links);
        assert_eq!(a.mini_chunks, b.mini_chunks);
        assert_eq!(a.chunk_index, b.chunk_index);
    }
}

// ---------------------------------------------------------------------------
// clean_text parity tests
// Business value: corpus must round-trip Onyx's preprocessing so eval
// numbers comparable to upstream.
// ---------------------------------------------------------------------------
#[test]
fn test_clean_text_strips_control_chars() {
    assert_eq!(clean_text("\x00hello\x07"), "hello");
    assert_eq!(clean_text("\x01\x02world"), "world");
}

#[test]
fn test_clean_text_preserves_newlines_and_tabs() {
    assert_eq!(clean_text("a\nb\tc"), "a\nb\tc");
}

#[test]
fn test_clean_text_strips_general_punctuation_block() {
    // U+2014 EM DASH is in the General Punctuation block (2000-206f) and
    // Onyx strips it.
    assert_eq!(clean_text("a\u{2014}b"), "ab");
}
