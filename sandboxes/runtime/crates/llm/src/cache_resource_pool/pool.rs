//! `CacheResourcePool` impl — hit/miss lookup, LRU eviction, shutdown.

use std::sync::Arc;

use chrono::{DateTime, Duration as ChronoDuration, Utc};
use dashmap::DashMap;
use tokio::sync::Mutex;
use tracing::{info, warn};

use super::{
    BackendError, CacheEntry, CacheLookup, CachePayload, CacheProvider, CacheResourceBackend,
    PoolConfig, PrefixHash,
};

/// A pool of server-side cache resources keyed by prefix hash.
///
/// Thread-safe via `DashMap` + per-entry `Mutex`. All mutating operations
/// hold locks for the minimum required scope.
pub struct CacheResourcePool {
    backend: Arc<dyn CacheResourceBackend>,
    pub(crate) entries: DashMap<PrefixHash, Arc<Mutex<CacheEntry>>>,
    config: PoolConfig,
}

impl CacheResourcePool {
    pub fn new(backend: Arc<dyn CacheResourceBackend>, config: PoolConfig) -> Self {
        Self {
            backend,
            entries: DashMap::new(),
            config,
        }
    }

    pub fn backend_provider(&self) -> CacheProvider {
        self.backend.provider()
    }

    /// Aggregate retained token count across all live entries.
    pub fn storage_tokens(&self) -> u64 {
        let mut sum: u64 = 0;
        for entry in self.entries.iter() {
            // Non-blocking probe: if locked, approximate as zero for this
            // snapshot. Accounting corrects itself on the next eviction.
            if let Ok(g) = entry.value().try_lock() {
                sum = sum.saturating_add(g.token_count);
            }
        }
        sum
    }

    pub fn entry_count(&self) -> usize {
        self.entries.len()
    }

    /// Look up an entry by prefix hash. Returns metadata; does NOT renew
    /// the TTL (see [`Self::get_or_create`]).
    pub async fn peek(&self, prefix_hash: &PrefixHash) -> Option<CacheEntry> {
        let handle = self.entries.get(prefix_hash)?;
        let entry = handle.value().clone();
        drop(handle);
        let guard = entry.lock().await;
        Some(guard.clone())
    }

    /// Get a cached resource for `prefix_hash` or create one via the
    /// backend. On hit, the entry's `last_hit_at` and `expires_at` are
    /// bumped and the backend is asked to renew the TTL.
    pub async fn get_or_create(&self, payload: CachePayload) -> Result<CacheLookup, BackendError> {
        if payload.token_count < self.config.min_tokens_to_cache {
            return Ok(CacheLookup::Skipped);
        }

        // Fast path: hit.
        if let Some(handle) = self.entries.get(&payload.prefix_hash) {
            let entry_arc = handle.value().clone();
            drop(handle);
            let mut entry = entry_arc.lock().await;
            if entry.expires_at > Utc::now() {
                entry.hit_count = entry.hit_count.saturating_add(1);
                entry.last_hit_at = Utc::now();
                entry.expires_at = Utc::now() + ChronoDuration::seconds(payload.ttl_secs as i64);
                let cache_id = entry.provider_cache_id.clone();
                let exp = entry.expires_at;
                drop(entry);
                // Renew server-side TTL. Failure to renew is not fatal —
                // the cache will expire naturally; next caller creates a
                // fresh one.
                if let Err(e) = self.backend.renew_ttl(&cache_id, payload.ttl_secs).await {
                    warn!(
                        provider_cache_id = %cache_id,
                        error = %e,
                        "cache_renew_ttl_failed_non_fatal"
                    );
                }
                return Ok(CacheLookup::Hit {
                    provider_cache_id: cache_id,
                    expires_at: exp,
                });
            }
            // Expired; fall through to recreate.
        }

        // Slow path: create against backend.
        let created = self.backend.create(payload.clone()).await?;

        let entry = CacheEntry {
            prefix_hash: payload.prefix_hash.clone(),
            provider_cache_id: created.provider_cache_id.clone(),
            owner_agent_id: payload.owner_agent_id.clone(),
            provider: self.backend.provider(),
            created_at: Utc::now(),
            expires_at: created.expires_at,
            last_hit_at: Utc::now(),
            hit_count: 0,
            token_count: payload.token_count,
        };

        self.entries.insert(
            payload.prefix_hash.clone(),
            Arc::new(Mutex::new(entry.clone())),
        );

        info!(
            provider = ?entry.provider,
            provider_cache_id = %entry.provider_cache_id,
            owner_agent_id = %entry.owner_agent_id,
            token_count = entry.token_count,
            "cache_resource_created"
        );

        // Enforce budgets after insertion so the just-created entry has a
        // fair shot at surviving LRU.
        self.evict_to_budget().await;

        Ok(CacheLookup::Miss {
            provider_cache_id: created.provider_cache_id,
            expires_at: created.expires_at,
        })
    }

    /// Evict everything owned by `agent_id` (typically called on agent
    /// shutdown). Returns the ids that were successfully deleted.
    pub async fn evict_by_owner(&self, agent_id: &str) -> Vec<String> {
        let targets: Vec<(PrefixHash, String)> = {
            let mut v = Vec::new();
            for entry in self.entries.iter() {
                if let Ok(g) = entry.value().try_lock() {
                    if g.owner_agent_id == agent_id {
                        v.push((entry.key().clone(), g.provider_cache_id.clone()));
                    }
                }
            }
            v
        };

        let mut deleted = Vec::with_capacity(targets.len());
        for (prefix, cache_id) in targets {
            if let Err(e) = self.backend.delete(&cache_id).await {
                warn!(
                    provider_cache_id = %cache_id,
                    error = %e,
                    "cache_resource_delete_failed"
                );
                continue;
            }
            self.entries.remove(&prefix);
            deleted.push(cache_id);
        }
        deleted
    }

    /// Remove entries whose `expires_at` is in the past. Called on a timer.
    pub async fn evict_expired(&self) -> Vec<String> {
        let now = Utc::now();
        let expired: Vec<(PrefixHash, String)> = {
            let mut v = Vec::new();
            for entry in self.entries.iter() {
                if let Ok(g) = entry.value().try_lock() {
                    if g.expires_at <= now {
                        v.push((entry.key().clone(), g.provider_cache_id.clone()));
                    }
                }
            }
            v
        };
        for (p, _) in &expired {
            self.entries.remove(p);
        }
        expired.into_iter().map(|(_, id)| id).collect()
    }

    /// Evict until both entry-count and storage-tokens are within budget.
    /// LRU by `last_hit_at`.
    pub async fn evict_to_budget(&self) {
        loop {
            let over_entries = self.entries.len() > self.config.max_entries;
            let over_storage = self.storage_tokens() > self.config.max_storage_tokens;
            if !over_entries && !over_storage {
                break;
            }
            // Find the LRU entry.
            let mut oldest: Option<(PrefixHash, String, DateTime<Utc>)> = None;
            for entry in self.entries.iter() {
                if let Ok(g) = entry.value().try_lock() {
                    let ts = g.last_hit_at;
                    let cid = g.provider_cache_id.clone();
                    let better = match oldest {
                        Some((_, _, prev)) => ts < prev,
                        None => true,
                    };
                    if better {
                        oldest = Some((entry.key().clone(), cid, ts));
                    }
                }
            }
            let Some((victim, cache_id, _)) = oldest else {
                break;
            };
            let _ = self.backend.delete(&cache_id).await;
            self.entries.remove(&victim);
        }
    }

    /// Bulk delete everything. Call on process shutdown — otherwise the
    /// server keeps billing for storage until TTL expiry.
    pub async fn shutdown(&self) {
        let ids: Vec<(PrefixHash, String)> = {
            let mut v = Vec::new();
            for entry in self.entries.iter() {
                if let Ok(g) = entry.value().try_lock() {
                    v.push((entry.key().clone(), g.provider_cache_id.clone()));
                }
            }
            v
        };
        for (prefix, cache_id) in ids {
            let _ = self.backend.delete(&cache_id).await;
            self.entries.remove(&prefix);
        }
    }
}
