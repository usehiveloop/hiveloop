//! Idempotency LRU cache for RPC replays.
//!
//! # Why this exists
//!
//! Go's `asynq` worker pool retries tasks on transient failures. A retried
//! `IngestBatch` that already succeeded must NOT re-chunk / re-embed /
//! re-write — that would waste SiliconFlow tokens and race the sharing of
//! doc_ids. The proto contract (`idempotency_key` fields on every
//! mutating RPC) says "same key → same result" from the server's point
//! of view. This module implements that.
//!
//! Cache key: `{dataset_name}|{org_id}|{idempotency_key}`. Scoping by
//! dataset + org means two different orgs (or two datasets within one
//! org) that happen to pick the same idempotency_key never collide.
//!
//! Entries expire via TTL (default 1 hour) regardless of LRU position.
//! A replay outside the TTL is treated as a fresh request — the worker
//! that's been stuck for an hour will redo the work rather than get a
//! stale cached body.

use std::hash::Hash;
use std::num::NonZeroUsize;
use std::sync::Mutex;
use std::time::{Duration, Instant};

use lru::LruCache;

/// One cached RPC response, timestamped so expired entries can be
/// filtered out on read.
#[derive(Debug, Clone)]
pub struct CachedEntry<V: Clone> {
    pub value: V,
    pub inserted_at: Instant,
}

/// TTL-aware LRU wrapper. Thread-safe via a `std::sync::Mutex`;
/// operations are all O(1) around tiny critical sections so lock
/// contention is a non-issue.
pub struct IdempotencyCache<V: Clone> {
    inner: Mutex<LruCache<String, CachedEntry<V>>>,
    ttl: Duration,
}

impl<V: Clone> IdempotencyCache<V> {
    /// Build a cache with the given capacity + TTL. Capacity of 0 is
    /// silently bumped to 1 — a zero-capacity LRU would panic on first
    /// insert and crash the server, which is a worse failure mode than
    /// "caches nothing useful".
    pub fn new(capacity: usize, ttl: Duration) -> Self {
        let cap = NonZeroUsize::new(capacity.max(1)).expect("capacity >= 1");
        Self {
            inner: Mutex::new(LruCache::new(cap)),
            ttl,
        }
    }

    /// Compose the cache key from its three components.
    pub fn compose_key(dataset_name: &str, org_id: &str, idempotency_key: &str) -> String {
        // `|` is not a valid char in any of the three inputs per our
        // validation; using it as a delimiter guarantees no collision
        // between e.g. ("a|b", "c") and ("a", "b|c").
        format!("{dataset_name}|{org_id}|{idempotency_key}")
    }

    /// Look up a key. Returns `Some(value)` if the entry exists AND is
    /// within TTL. Expired entries are evicted as a side effect of the
    /// lookup so a stale key can be re-populated by the caller.
    pub fn get(&self, key: &str) -> Option<V> {
        let mut guard = self.inner.lock().ok()?;
        if let Some(entry) = guard.get(key) {
            if entry.inserted_at.elapsed() < self.ttl {
                return Some(entry.value.clone());
            }
        }
        // Either not present or expired; evict if present.
        guard.pop(key);
        None
    }

    /// Store a key → value mapping. LRU evicts the oldest entry if at
    /// capacity. Idempotent if the key is already present (replaces the
    /// existing entry, refreshing its timestamp).
    pub fn put(&self, key: String, value: V) {
        if let Ok(mut guard) = self.inner.lock() {
            guard.put(
                key,
                CachedEntry {
                    value,
                    inserted_at: Instant::now(),
                },
            );
        }
    }

    /// Current number of entries (including expired-but-not-evicted).
    /// Exposed for tests and admin metrics.
    #[allow(dead_code)]
    pub fn len(&self) -> usize {
        self.inner.lock().map(|g| g.len()).unwrap_or(0)
    }

    /// Convenience for `len() == 0`. Exists only to satisfy clippy's
    /// `len_without_is_empty`; callers rarely care.
    #[allow(dead_code)]
    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }
}

impl<V: Clone> std::fmt::Debug for IdempotencyCache<V> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("IdempotencyCache")
            .field("ttl", &self.ttl)
            .field("len", &self.len())
            .finish()
    }
}

// `Hash + Eq` bound kept trivial — we only key by `String`, but the
// generic bound lets future callers swap the key type without breaking
// the public surface.
const _: fn() = || {
    fn assert_send_sync<T: Send + Sync>() {}
    // `V` must be `Send + Sync` when the user wants the cache itself to
    // be. We can't assert that unconditionally; the compiler verifies
    // this per-instantiation at use sites.
    assert_send_sync::<std::sync::Mutex<LruCache<String, CachedEntry<String>>>>();
};

// Silence unused bound lint when `V` is string-like.
#[allow(dead_code)]
fn _trait_bounds<K: Hash + Eq>() {}
