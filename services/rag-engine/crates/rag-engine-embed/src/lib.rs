//! Embedder trait + SiliconFlow implementation + deterministic FakeEmbedder.
//!
//! # Status: Stub (Tranche 2A)
//!
//! This crate is empty in Phase 2 Tranche 2A. Tranche 2C will add:
//!   * `Embedder` async trait with `embed_passages` / `embed_query` /
//!     `dimension` / `model_id`
//!   * SiliconFlow HTTP-backed implementation against the OpenAI-compatible
//!     `/v1/embeddings` surface
//!   * `FakeEmbedder` — deterministic SHA-256 + ChaCha20 unit-vector
//!     generator, used by every non-2C test in the workspace
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2C for the full spec.
