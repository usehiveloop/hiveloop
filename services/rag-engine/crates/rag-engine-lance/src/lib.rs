//! LanceDB storage layer for the rag-engine service.
//!
//! # Status: Tranche 2B (implementation)
//!
//! This crate wraps the upstream `lancedb` Rust crate and exposes the
//! primitives the Phase 0 Go-binding spike failed to deliver. Most
//! importantly, the metadata-only `list<string>` update — the op that
//! killed the Go spike — MUST work here. Without it, perm-sync at the
//! Phase 3 scale is not viable.
//!
//! Modules:
//! * [`chunk`]   — `ChunkRow` struct matching the proto `SearchHit` / `DocumentToIngest` surface
//! * [`schema`]  — Arrow schema builder keyed by vector dimension
//! * [`store`]   — S3 / MinIO connection wiring
//! * [`dataset`] — create / open / exists / drop
//! * [`ingest`]  — batch upsert (`merge_insert`), reindex wipe
//! * [`update`]  — **metadata-only ACL update** (the Op 6 primitive)
//! * [`search`]  — hybrid vector + FTS with org + ACL filter
//! * [`delete`]  — by doc_id, by org
//! * [`indexes`] — FTS + ANN index builders
//! * [`error`]   — typed errors for this layer
//! * [`filter`]  — injection-safe SQL filter builder
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2B for the full spec.

pub mod chunk;
pub mod dataset;
pub mod delete;
pub mod error;
pub mod filter;
pub mod indexes;
pub mod ingest;
pub mod schema;
pub mod search;
pub mod store;
pub mod update;

pub use chunk::ChunkRow;
pub use dataset::DatasetHandle;
pub use error::{LanceStoreError, Result};
pub use ingest::{IngestStats, IngestionMode};
pub use search::{SearchHit, SearchMode, SearchParams};
pub use store::{LanceStore, StoreConfig};
pub use update::UpdateAclEntry;
