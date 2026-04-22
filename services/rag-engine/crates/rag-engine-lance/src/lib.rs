//! LanceDB storage layer for the rag-engine service.
//!
//! # Status: Stub (Tranche 2A)
//!
//! This crate is deliberately empty in Phase 2 Tranche 2A — it is a
//! placeholder the Cargo workspace can refer to so that downstream
//! tranches (2B) can land without restructuring the workspace.
//!
//! Tranche 2B will add:
//!   * `connection::LanceConnection` — S3 / MinIO wiring
//!   * `schema` — Arrow schema builder keyed by vector dimension
//!   * `dataset::DatasetHandle` — create / drop / exists
//!   * `ingest` — batch upsert, reindex wipe
//!   * `update` — metadata-only column updates (the Op 6 primitive)
//!   * `search` — vector + FTS + filtered
//!   * `delete` — by doc_id, by org, prune
//!   * `filters` — typed, injection-safe SQL filter builder
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2B for the full spec.
