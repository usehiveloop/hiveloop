//! Chunker using `text-splitter` + `tiktoken-rs`, Onyx-compatible semantics.
//!
//! # Status: Stub (Tranche 2A)
//!
//! This crate is empty in Phase 2 Tranche 2A. Tranche 2E will add:
//!   * `Chunker` struct + `ChunkerConfig` (512-token content limit, 102-
//!     token overlap — the locked Hiveloop deviation from Onyx upstream,
//!     per `plans/onyx-port-phase2.md` §DEVIATION chunk parameters)
//!   * `tiktoken-rs` wrapper keyed by embedder model family
//!   * Port of Onyx's `AccumulatorState` multi-section packing
//!   * Port of Onyx's mini-chunks (`MINI_CHUNK_SIZE = 150`)
//!   * Port of `clean_text` from `backend/onyx/utils/text_processing.py`
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2E for the full spec.
