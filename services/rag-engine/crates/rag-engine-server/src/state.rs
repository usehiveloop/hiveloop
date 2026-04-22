//! Application state — the dependency graph all RPC handlers share.
//!
//! Every handler receives an `Arc<AppState>` (via `RagEngineService`),
//! which owns:
//!   * `LanceStore` — LanceDB connection (cheap to clone; internally
//!     `Arc<Connection>`).
//!   * `Embedder` — trait object picked at boot from env (openai_compat
//!     or fake). `Arc<dyn Embedder>` keeps it cloneable across tasks.
//!   * `Reranker` — trait object; `Arc<dyn Reranker>`.
//!   * `Chunker<TiktokenTokenizer>` — one process-wide instance. The
//!     tokenizer loads a ~9 MB BPE table on construction; never rebuild
//!     it per-request.
//!   * Idempotency LRU caches — separate per RPC type so a cached
//!     `IngestBatch` result never collides with a cached `UpdateACL`.
//!   * `&'static Metrics` — from tranche 2G's global registry.
//!   * Limits — `max_docs_per_batch`, `max_chunks_per_batch` — read at
//!     boot, enforced per-RPC.
//!
//! `AppState` is `Send + Sync + 'static`; every field is either trivially
//! thread-safe or wrapped in an `Arc`/`Mutex`.

use std::sync::Arc;

use rag_engine_chunker::tokenizer::TiktokenTokenizer;
use rag_engine_chunker::Chunker;
use rag_engine_embed::Embedder;
use rag_engine_lance::LanceStore;
use rag_engine_proto::{
    DeleteByDocIdResponse, DeleteByOrgResponse, IngestBatchResponse, PruneResponse,
    UpdateAclResponse,
};
use rag_engine_rerank::Reranker;

use crate::idempotency::IdempotencyCache;
use crate::metrics::Metrics;

/// Tune-ables the server enforces at the RPC boundary. Read from env at
/// boot; see `AppStateBuilder::from_env`.
#[derive(Debug, Clone)]
pub struct StateLimits {
    /// Hard upper bound on `IngestBatchRequest.documents`.
    /// Larger batches → `INVALID_ARGUMENT`.
    pub max_docs_per_batch: usize,
    /// Hard upper bound on the total number of chunks one batch may
    /// produce after chunking. Enforced by tranche 2B's ingest layer too
    /// (2000), but the server rejects earlier so the caller gets a
    /// crisp error code.
    pub max_chunks_per_batch: usize,
    /// LRU capacity + TTL applied to every idempotency cache.
    pub idempotency_capacity: usize,
    pub idempotency_ttl_secs: u64,
}

impl StateLimits {
    pub fn defaults() -> Self {
        Self {
            max_docs_per_batch: 1_000,
            // Matches the per-call ceiling inside `rag-engine-lance::
            // ingest::MAX_CHUNKS_PER_CALL`. 2F rejects at the boundary
            // with a clearer error message.
            max_chunks_per_batch: 2_000,
            idempotency_capacity: 1_000,
            idempotency_ttl_secs: 3_600,
        }
    }

    /// Merge env overrides on top of defaults. Unset or unparseable env
    /// vars fall back to the default — we never fail boot over a
    /// bad-value env on a knob (config knobs, unlike the shared secret,
    /// have safe defaults).
    pub fn from_env() -> Self {
        let mut s = Self::defaults();
        if let Ok(v) = std::env::var("MAX_DOCS_PER_BATCH") {
            if let Ok(n) = v.parse::<usize>() {
                if n > 0 {
                    s.max_docs_per_batch = n;
                }
            }
        }
        if let Ok(v) = std::env::var("MAX_CHUNKS_PER_BATCH") {
            if let Ok(n) = v.parse::<usize>() {
                if n > 0 {
                    s.max_chunks_per_batch = n;
                }
            }
        }
        if let Ok(v) = std::env::var("IDEMPOTENCY_CACHE_CAPACITY") {
            if let Ok(n) = v.parse::<usize>() {
                if n > 0 {
                    s.idempotency_capacity = n;
                }
            }
        }
        if let Ok(v) = std::env::var("IDEMPOTENCY_CACHE_TTL_SECS") {
            if let Ok(n) = v.parse::<u64>() {
                if n > 0 {
                    s.idempotency_ttl_secs = n;
                }
            }
        }
        s
    }
}

/// Every RPC's idempotency cache. Separate types per RPC so the LRU
/// stores the exact response we'll play back.
pub struct IdempotencyCaches {
    pub ingest: IdempotencyCache<IngestBatchResponse>,
    pub update_acl: IdempotencyCache<UpdateAclResponse>,
    pub delete_by_doc: IdempotencyCache<DeleteByDocIdResponse>,
    pub delete_by_org: IdempotencyCache<DeleteByOrgResponse>,
    pub prune: IdempotencyCache<PruneResponse>,
}

impl IdempotencyCaches {
    pub fn new(capacity: usize, ttl: std::time::Duration) -> Self {
        Self {
            ingest: IdempotencyCache::new(capacity, ttl),
            update_acl: IdempotencyCache::new(capacity, ttl),
            delete_by_doc: IdempotencyCache::new(capacity, ttl),
            delete_by_org: IdempotencyCache::new(capacity, ttl),
            prune: IdempotencyCache::new(capacity, ttl),
        }
    }
}

impl std::fmt::Debug for IdempotencyCaches {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("IdempotencyCaches")
            .field("ingest_len", &self.ingest.len())
            .field("update_acl_len", &self.update_acl.len())
            .field("delete_by_doc_len", &self.delete_by_doc.len())
            .field("delete_by_org_len", &self.delete_by_org.len())
            .field("prune_len", &self.prune.len())
            .finish()
    }
}

/// Process-wide state handed to every RPC.
pub struct AppState {
    pub store: LanceStore,
    pub embedder: Arc<dyn Embedder>,
    pub reranker: Arc<dyn Reranker>,
    pub chunker: Arc<Chunker<TiktokenTokenizer>>,
    pub idempotency: IdempotencyCaches,
    pub metrics: &'static Metrics,
    pub limits: StateLimits,
}

impl AppState {
    /// Assemble state from already-built pieces. Used by tests to inject
    /// test doubles and by `main.rs` to wire real components.
    pub fn new(
        store: LanceStore,
        embedder: Arc<dyn Embedder>,
        reranker: Arc<dyn Reranker>,
        chunker: Arc<Chunker<TiktokenTokenizer>>,
        limits: StateLimits,
    ) -> Self {
        let ttl = std::time::Duration::from_secs(limits.idempotency_ttl_secs);
        let caches = IdempotencyCaches::new(limits.idempotency_capacity, ttl);
        Self {
            store,
            embedder,
            reranker,
            chunker,
            idempotency: caches,
            metrics: Metrics::global(),
            limits,
        }
    }
}

impl std::fmt::Debug for AppState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("AppState")
            .field("embedder_id", &self.embedder.id())
            .field("reranker_id", &self.reranker.id())
            .field("limits", &self.limits)
            .field("idempotency", &self.idempotency)
            .finish()
    }
}

// Compile-time assertion that `AppState` is thread-shareable. If a
// future field breaks this (e.g. a non-Send future), CI fails before
// the runtime panics.
const _: fn() = || {
    fn assert_send_sync<T: Send + Sync>() {}
    assert_send_sync::<Arc<AppState>>();
};
