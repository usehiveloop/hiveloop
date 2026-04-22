//! Chunker constants. Every value is pinned to an Onyx source location so
//! downstream agents can verify parity in one grep.
//!
//! Upstream Onyx references:
//! - `backend/onyx/indexing/chunker.py`
//! - `backend/onyx/configs/app_configs.py`
//! - `backend/shared_configs/configs.py`
//! - `backend/onyx/configs/constants.py`

// Onyx backend/shared_configs/configs.py:42 — `DOC_EMBEDDING_CONTEXT_SIZE = 512`
// The hard token limit for a single embedded chunk. Every embedder in the
// locked architecture targets a 512-token context window.
pub const DOC_EMBEDDING_CONTEXT_SIZE: usize = 512;

// Onyx backend/onyx/indexing/chunker.py:28 — `CHUNK_OVERLAP = 0`
//
// HIVELOOP DEVIATION (locked in plans/onyx-port-phase2.md §5.3): upstream
// Onyx ships 0-token overlap. Hiveloop uses 20% overlap (102 tokens out of
// 512) because empirical retrieval quality on our corpus improves
// noticeably when sentences straddle chunk boundaries. See DECISIONS.md.
pub const HIVELOOP_CHUNK_OVERLAP_TOKENS: usize = 102;

// Onyx backend/onyx/indexing/chunker.py:31 — `MAX_METADATA_PERCENTAGE = 0.25`
pub const MAX_METADATA_PERCENTAGE: f32 = 0.25;

// Onyx backend/onyx/indexing/chunker.py:32 — `CHUNK_MIN_CONTENT = 256`
// If content budget after reserving title+metadata falls under this, drop
// the prefix/suffix and revert to full-chunk-body mode.
pub const CHUNK_MIN_CONTENT: usize = 256;

// Onyx backend/onyx/configs/app_configs.py:46 — `BLURB_SIZE = 128`
// Tokens of content included in the blurb preview (first-sentence-ish
// summary shown in the UI).
pub const BLURB_SIZE: usize = 128;

// Onyx backend/onyx/configs/app_configs.py:821 — `MINI_CHUNK_SIZE = 150`
// Sub-chunks produced within each chunk for higher-precision retrieval.
pub const MINI_CHUNK_SIZE: usize = 150;

// Onyx backend/onyx/configs/app_configs.py:824 — `LARGE_CHUNK_RATIO = 4`
// Number of normal chunks combined into one "large" chunk for multipass
// retrieval. Included here for completeness; large-chunk generation is
// explicitly out of scope for 2E (multipass is not in the locked
// retrieval flow).
pub const LARGE_CHUNK_RATIO: usize = 4;

// Onyx backend/onyx/configs/constants.py:51 — `RETURN_SEPARATOR = "\n\r\n"`
pub const RETURN_SEPARATOR: &str = "\n\r\n";

// Onyx backend/onyx/configs/constants.py:52 — `SECTION_SEPARATOR = "\n\n"`
pub const SECTION_SEPARATOR: &str = "\n\n";
