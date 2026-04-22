//! Reranker trait + SiliconFlow implementation + deterministic FakeReranker.
//!
//! # Status: Stub (Tranche 2A)
//!
//! This crate is empty in Phase 2 Tranche 2A. Tranche 2D will add:
//!   * `Reranker` async trait with `rerank` / `model_id`
//!   * SiliconFlow HTTP-backed implementation against the `/rerank`
//!     endpoint (Qwen3-Reranker-0.6B)
//!   * `FakeReranker` — deterministic length-monotonic scorer for tests
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2D for the full spec.
