//! Cache resource pool for explicit-resource provider caches.
//!
//! Some providers expose prompt caching as a server-side *resource* that
//! you create once and reference by id:
//!
//! - **Gemini**: `POST /v1beta/cachedContents` → a `CachedContent` with a
//!   TTL. You reference it by its `name` on subsequent `generateContent`
//!   calls. Storage is billed per 1M token-hours.
//! - **Kimi moonshot-v1**: `POST /v1/caching` → a `cache-xxx` id. You
//!   reference it via an `X-Msh-Context-Cache` header. Historically also
//!   billed for storage per minute.
//!
//! Unlike implicit prefix caching (OpenAI, GLM, Anthropic's
//! `cache_control`), these are **billable assets**. Leak one and you pay
//! its storage fee until TTL expiry. This module manages their lifecycle:
//! creation on demand, LRU eviction under a storage-tokens budget,
//! per-agent bulk deletion on shutdown, and TTL renewal on cache hits.
//!
//! The pool is provider-agnostic. Each backend implementation speaks to
//! its provider's HTTP API; the pool handles accounting and eviction.
//!
//! ## Status
//!
//! The [`InMemoryBackend`] below is the test fixture used to validate pool
//! semantics. Real Gemini and Kimi backends are provided as scaffolds in
//! this module but their HTTP bodies are wired only enough to compile and
//! document the endpoint shapes — actual network I/O is left for a
//! follow-up once real API-key integration tests are wired.

use std::sync::Arc;

use async_trait::async_trait;
use chrono::{DateTime, Duration as ChronoDuration, Utc};
use dashmap::DashMap;
use serde::{Deserialize, Serialize};
use tokio::sync::Mutex;
use tracing::{info, warn};

/// Hex SHA-256 of the cacheable prefix (preamble + tool_defs + any stable
/// history). Used as the pool key — two requests with the same prefix
/// share a single server-side cache resource.
pub type PrefixHash = String;

/// Which provider a given backend talks to.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CacheProvider {
    /// Gemini `cachedContents` resources.
    GeminiExplicit,
    /// Kimi moonshot-v1 `/v1/caching` resources.
    KimiV1,
}

/// Live metadata about one server-side cache resource.
#[derive(Debug, Clone)]
pub struct CacheEntry {
    pub prefix_hash: PrefixHash,
    /// Provider-assigned id — Gemini `cachedContents/abc123`, Kimi `cache-abc`.
    pub provider_cache_id: String,
    /// Agent that created the entry. Used for bulk deletion on agent shutdown.
    pub owner_agent_id: String,
    pub provider: CacheProvider,
    pub created_at: DateTime<Utc>,
    pub expires_at: DateTime<Utc>,
    pub last_hit_at: DateTime<Utc>,
    pub hit_count: u64,
    /// Approximate cached-prefix token count. Drives storage-budget
    /// eviction.
    pub token_count: u64,
}

/// Payload required to create a new cache resource. Callers construct one
/// from their provider-shaped request (system, tools, messages, etc.).
#[derive(Debug, Clone)]
pub struct CachePayload {
    pub owner_agent_id: String,
    pub model: String,
    pub prefix_hash: PrefixHash,
    pub token_count: u64,
    pub ttl_secs: u32,
    /// Opaque JSON that the backend will use as the create-request body.
    /// Shape is provider-specific.
    pub body: serde_json::Value,
}

/// Result of a successful backend create.
#[derive(Debug, Clone)]
pub struct CreatedCache {
    pub provider_cache_id: String,
    pub expires_at: DateTime<Utc>,
}

/// Errors that a backend may surface.
#[derive(Debug, thiserror::Error)]
pub enum BackendError {
    #[error("backend http error: {0}")]
    Http(String),
    #[error("backend decode error: {0}")]
    Decode(String),
    #[error("backend rejected request: {0}")]
    BadRequest(String),
    #[error("cache resource not found")]
    NotFound,
}

/// Provider-specific HTTP handler for a single cache provider.
#[async_trait]
pub trait CacheResourceBackend: Send + Sync {
    async fn create(&self, req: CachePayload) -> Result<CreatedCache, BackendError>;
    async fn delete(&self, cache_id: &str) -> Result<(), BackendError>;
    async fn renew_ttl(&self, cache_id: &str, ttl_secs: u32) -> Result<(), BackendError>;
    fn provider(&self) -> CacheProvider;
}

/// Pool-level configuration.
#[derive(Debug, Clone)]
pub struct PoolConfig {
    /// Maximum number of live cache entries across all owners. Oldest-hit
    /// entries evicted first.
    pub max_entries: usize,
    /// Maximum aggregate token count retained in cache. Prevents runaway
    /// storage costs for Gemini, which bills per token-hour.
    pub max_storage_tokens: u64,
    /// Don't create a cache resource for prefixes under this size — the
    /// storage fee outweighs the saving.
    pub min_tokens_to_cache: u64,
    /// Default TTL if the caller doesn't supply one.
    pub default_ttl_secs: u32,
}

impl Default for PoolConfig {
    fn default() -> Self {
        Self {
            max_entries: 256,
            max_storage_tokens: 5_000_000,
            min_tokens_to_cache: 20_000,
            default_ttl_secs: 3600,
        }
    }
}

/// A pool of server-side cache resources keyed by prefix hash.
///
/// Thread-safe via `DashMap` + per-entry `Mutex`. All mutating operations
/// hold locks for the minimum required scope.
pub struct CacheResourcePool {
    backend: Arc<dyn CacheResourceBackend>,
    entries: DashMap<PrefixHash, Arc<Mutex<CacheEntry>>>,
    config: PoolConfig,
}

/// Outcome of [`CacheResourcePool::get_or_create`].
#[derive(Debug)]
pub enum CacheLookup {
    /// Found a live, non-expired entry. Prefer this path — zero provider
    /// spend. The entry's TTL is extended by `renew_ttl` on the backend.
    Hit {
        provider_cache_id: String,
        expires_at: DateTime<Utc>,
    },
    /// Created a fresh entry against the backend. This is a write that
    /// costs the base input price on most providers.
    Miss {
        provider_cache_id: String,
        expires_at: DateTime<Utc>,
    },
    /// Prefix was below `min_tokens_to_cache`; caller should skip caching
    /// and pay full input price.
    Skipped,
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

// ---------------------------------------------------------------------------
// In-memory backend — used for tests and as a reference implementation.
// ---------------------------------------------------------------------------

/// Fake backend that records operations in memory. Useful for tests and
/// for dry-running the pool without talking to a real provider.
pub struct InMemoryBackend {
    provider: CacheProvider,
    next_id: std::sync::atomic::AtomicU64,
    store: DashMap<String, DateTime<Utc>>,
    pub create_calls: std::sync::atomic::AtomicU64,
    pub delete_calls: std::sync::atomic::AtomicU64,
    pub renew_calls: std::sync::atomic::AtomicU64,
    pub fail_next_create: std::sync::atomic::AtomicBool,
}

impl InMemoryBackend {
    pub fn new(provider: CacheProvider) -> Self {
        Self {
            provider,
            next_id: std::sync::atomic::AtomicU64::new(1),
            store: DashMap::new(),
            create_calls: std::sync::atomic::AtomicU64::new(0),
            delete_calls: std::sync::atomic::AtomicU64::new(0),
            renew_calls: std::sync::atomic::AtomicU64::new(0),
            fail_next_create: std::sync::atomic::AtomicBool::new(false),
        }
    }

    pub fn live_ids(&self) -> Vec<String> {
        self.store.iter().map(|e| e.key().clone()).collect()
    }
}

#[async_trait]
impl CacheResourceBackend for InMemoryBackend {
    async fn create(&self, req: CachePayload) -> Result<CreatedCache, BackendError> {
        self.create_calls
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        if self
            .fail_next_create
            .swap(false, std::sync::atomic::Ordering::Relaxed)
        {
            return Err(BackendError::Http("injected failure".into()));
        }
        let id = format!(
            "mem-cache-{}",
            self.next_id
                .fetch_add(1, std::sync::atomic::Ordering::Relaxed)
        );
        let expires = Utc::now() + ChronoDuration::seconds(req.ttl_secs as i64);
        self.store.insert(id.clone(), expires);
        Ok(CreatedCache {
            provider_cache_id: id,
            expires_at: expires,
        })
    }

    async fn delete(&self, cache_id: &str) -> Result<(), BackendError> {
        self.delete_calls
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        if self.store.remove(cache_id).is_none() {
            return Err(BackendError::NotFound);
        }
        Ok(())
    }

    async fn renew_ttl(&self, cache_id: &str, ttl_secs: u32) -> Result<(), BackendError> {
        self.renew_calls
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        if let Some(mut e) = self.store.get_mut(cache_id) {
            *e = Utc::now() + ChronoDuration::seconds(ttl_secs as i64);
            Ok(())
        } else {
            Err(BackendError::NotFound)
        }
    }

    fn provider(&self) -> CacheProvider {
        self.provider
    }
}

// ---------------------------------------------------------------------------
// HTTP backend scaffolds — document provider endpoint shapes. Not yet
// wired to real networks (no API keys in test harness).
// ---------------------------------------------------------------------------

/// Gemini explicit `CachedContent` backend.
///
/// Endpoint: `POST {base}/v1beta/cachedContents`, body shape:
/// ```json
/// { "model": "models/gemini-2.5-pro",
///   "contents": [...], "systemInstruction": {...},
///   "tools": [...], "ttl": "3600s" }
/// ```
/// Response: `{ "name": "cachedContents/abc123", "expireTime": "..." }`.
/// Delete: `DELETE {base}/v1beta/{name}`. Update TTL:
/// `PATCH {base}/v1beta/{name}` with `{"ttl": "3600s"}`.
pub struct GeminiExplicitBackend {
    pub base_url: String,
    pub api_key: String,
    pub http: reqwest::Client,
}

impl GeminiExplicitBackend {
    pub fn new(base_url: impl Into<String>, api_key: impl Into<String>) -> Self {
        Self {
            base_url: base_url.into(),
            api_key: api_key.into(),
            http: reqwest::Client::new(),
        }
    }
}

#[async_trait]
impl CacheResourceBackend for GeminiExplicitBackend {
    async fn create(&self, req: CachePayload) -> Result<CreatedCache, BackendError> {
        let url = format!(
            "{}/v1beta/cachedContents?key={}",
            self.base_url.trim_end_matches('/'),
            self.api_key
        );
        // req.body is expected to be a fully-formed Gemini create payload.
        let resp = self
            .http
            .post(&url)
            .json(&req.body)
            .send()
            .await
            .map_err(|e| BackendError::Http(e.to_string()))?;
        if !resp.status().is_success() {
            return Err(BackendError::BadRequest(format!(
                "gemini create status={}",
                resp.status()
            )));
        }
        let body: serde_json::Value = resp
            .json()
            .await
            .map_err(|e| BackendError::Decode(e.to_string()))?;
        let name = body
            .get("name")
            .and_then(|v| v.as_str())
            .ok_or_else(|| BackendError::Decode("missing 'name' field".into()))?
            .to_string();
        // Gemini returns `expireTime` as RFC3339.
        let expires = body
            .get("expireTime")
            .and_then(|v| v.as_str())
            .and_then(|s| DateTime::parse_from_rfc3339(s).ok())
            .map(|d| d.with_timezone(&Utc))
            .unwrap_or_else(|| Utc::now() + ChronoDuration::seconds(req.ttl_secs as i64));
        Ok(CreatedCache {
            provider_cache_id: name,
            expires_at: expires,
        })
    }

    async fn delete(&self, cache_id: &str) -> Result<(), BackendError> {
        let url = format!(
            "{}/v1beta/{}?key={}",
            self.base_url.trim_end_matches('/'),
            cache_id,
            self.api_key
        );
        let resp = self
            .http
            .delete(&url)
            .send()
            .await
            .map_err(|e| BackendError::Http(e.to_string()))?;
        if resp.status().as_u16() == 404 {
            return Err(BackendError::NotFound);
        }
        if !resp.status().is_success() {
            return Err(BackendError::BadRequest(format!(
                "status={}",
                resp.status()
            )));
        }
        Ok(())
    }

    async fn renew_ttl(&self, cache_id: &str, ttl_secs: u32) -> Result<(), BackendError> {
        let url = format!(
            "{}/v1beta/{}?key={}",
            self.base_url.trim_end_matches('/'),
            cache_id,
            self.api_key
        );
        let body = serde_json::json!({ "ttl": format!("{}s", ttl_secs) });
        let resp = self
            .http
            .patch(&url)
            .json(&body)
            .send()
            .await
            .map_err(|e| BackendError::Http(e.to_string()))?;
        if !resp.status().is_success() {
            return Err(BackendError::BadRequest(format!(
                "status={}",
                resp.status()
            )));
        }
        Ok(())
    }

    fn provider(&self) -> CacheProvider {
        CacheProvider::GeminiExplicit
    }
}

/// Kimi moonshot-v1 `/v1/caching` backend.
///
/// Endpoint: `POST {base}/v1/caching` with bearer auth.
/// Body:
/// ```json
/// { "model": "moonshot-v1-128k",
///   "messages": [...], "tools": [...],
///   "name": "...", "description": "...", "ttl": 3600 }
/// ```
/// Response: `{ "id": "cache-xxx", "expires_at": <unix_ts> }`.
/// Delete: `DELETE {base}/v1/caching/{id}`. Renew:
/// `PUT {base}/v1/caching/{id}/reset` with `{"ttl": 3600}`.
pub struct KimiV1Backend {
    pub base_url: String,
    pub api_key: String,
    pub http: reqwest::Client,
}

impl KimiV1Backend {
    pub fn new(base_url: impl Into<String>, api_key: impl Into<String>) -> Self {
        Self {
            base_url: base_url.into(),
            api_key: api_key.into(),
            http: reqwest::Client::new(),
        }
    }
}

#[async_trait]
impl CacheResourceBackend for KimiV1Backend {
    async fn create(&self, req: CachePayload) -> Result<CreatedCache, BackendError> {
        let url = format!("{}/v1/caching", self.base_url.trim_end_matches('/'));
        let resp = self
            .http
            .post(&url)
            .bearer_auth(&self.api_key)
            .json(&req.body)
            .send()
            .await
            .map_err(|e| BackendError::Http(e.to_string()))?;
        if !resp.status().is_success() {
            return Err(BackendError::BadRequest(format!(
                "status={}",
                resp.status()
            )));
        }
        let body: serde_json::Value = resp
            .json()
            .await
            .map_err(|e| BackendError::Decode(e.to_string()))?;
        let id = body
            .get("id")
            .and_then(|v| v.as_str())
            .ok_or_else(|| BackendError::Decode("missing 'id'".into()))?
            .to_string();
        let expires_unix = body.get("expires_at").and_then(|v| v.as_i64());
        let expires = expires_unix
            .and_then(|ts| DateTime::<Utc>::from_timestamp(ts, 0))
            .unwrap_or_else(|| Utc::now() + ChronoDuration::seconds(req.ttl_secs as i64));
        Ok(CreatedCache {
            provider_cache_id: id,
            expires_at: expires,
        })
    }

    async fn delete(&self, cache_id: &str) -> Result<(), BackendError> {
        let url = format!(
            "{}/v1/caching/{}",
            self.base_url.trim_end_matches('/'),
            cache_id
        );
        let resp = self
            .http
            .delete(&url)
            .bearer_auth(&self.api_key)
            .send()
            .await
            .map_err(|e| BackendError::Http(e.to_string()))?;
        if resp.status().as_u16() == 404 {
            return Err(BackendError::NotFound);
        }
        if !resp.status().is_success() {
            return Err(BackendError::BadRequest(format!(
                "status={}",
                resp.status()
            )));
        }
        Ok(())
    }

    async fn renew_ttl(&self, cache_id: &str, ttl_secs: u32) -> Result<(), BackendError> {
        let url = format!(
            "{}/v1/caching/{}/reset",
            self.base_url.trim_end_matches('/'),
            cache_id
        );
        let body = serde_json::json!({ "ttl": ttl_secs });
        let resp = self
            .http
            .put(&url)
            .bearer_auth(&self.api_key)
            .json(&body)
            .send()
            .await
            .map_err(|e| BackendError::Http(e.to_string()))?;
        if !resp.status().is_success() {
            return Err(BackendError::BadRequest(format!(
                "status={}",
                resp.status()
            )));
        }
        Ok(())
    }

    fn provider(&self) -> CacheProvider {
        CacheProvider::KimiV1
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::Ordering;

    fn payload(hash: &str, owner: &str, tokens: u64) -> CachePayload {
        CachePayload {
            owner_agent_id: owner.into(),
            model: "test-model".into(),
            prefix_hash: hash.into(),
            token_count: tokens,
            ttl_secs: 3600,
            body: serde_json::json!({}),
        }
    }

    fn pool(backend: Arc<InMemoryBackend>, cfg: PoolConfig) -> CacheResourcePool {
        CacheResourcePool::new(backend, cfg)
    }

    #[tokio::test]
    async fn skipped_below_min_tokens() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 10_000,
                ..Default::default()
            },
        );
        let r = p.get_or_create(payload("h", "a", 1_000)).await.unwrap();
        assert!(matches!(r, CacheLookup::Skipped));
        assert_eq!(backend.create_calls.load(Ordering::Relaxed), 0);
    }

    #[tokio::test]
    async fn first_call_is_miss_and_creates_backend_entry() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 100,
                ..Default::default()
            },
        );
        let r = p.get_or_create(payload("h1", "a", 20_000)).await.unwrap();
        assert!(matches!(r, CacheLookup::Miss { .. }));
        assert_eq!(backend.create_calls.load(Ordering::Relaxed), 1);
        assert_eq!(p.entry_count(), 1);
    }

    #[tokio::test]
    async fn second_call_same_hash_is_hit_and_renews_ttl() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 100,
                ..Default::default()
            },
        );
        p.get_or_create(payload("h", "a", 20_000)).await.unwrap();
        let r = p.get_or_create(payload("h", "a", 20_000)).await.unwrap();
        assert!(matches!(r, CacheLookup::Hit { .. }));
        assert_eq!(backend.create_calls.load(Ordering::Relaxed), 1);
        assert_eq!(backend.renew_calls.load(Ordering::Relaxed), 1);
    }

    #[tokio::test]
    async fn different_hashes_create_separate_entries() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 100,
                ..Default::default()
            },
        );
        p.get_or_create(payload("a", "x", 20_000)).await.unwrap();
        p.get_or_create(payload("b", "x", 20_000)).await.unwrap();
        assert_eq!(p.entry_count(), 2);
        assert_eq!(backend.create_calls.load(Ordering::Relaxed), 2);
    }

    #[tokio::test]
    async fn lru_evicts_oldest_when_over_entry_budget() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                max_entries: 2,
                min_tokens_to_cache: 100,
                ..Default::default()
            },
        );
        p.get_or_create(payload("h1", "a", 10_000)).await.unwrap();
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
        p.get_or_create(payload("h2", "a", 10_000)).await.unwrap();
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
        // Inserting h3 should evict h1 (the oldest).
        p.get_or_create(payload("h3", "a", 10_000)).await.unwrap();

        assert_eq!(p.entry_count(), 2);
        assert!(p.peek(&"h1".into()).await.is_none());
        assert!(p.peek(&"h2".into()).await.is_some());
        assert!(p.peek(&"h3".into()).await.is_some());
        // Backend saw 3 creates + 1 delete for the eviction.
        assert_eq!(backend.create_calls.load(Ordering::Relaxed), 3);
        assert_eq!(backend.delete_calls.load(Ordering::Relaxed), 1);
    }

    #[tokio::test]
    async fn storage_token_budget_drives_eviction() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                max_entries: 100,
                max_storage_tokens: 25_000,
                min_tokens_to_cache: 1,
                ..Default::default()
            },
        );
        p.get_or_create(payload("h1", "a", 10_000)).await.unwrap();
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
        p.get_or_create(payload("h2", "a", 10_000)).await.unwrap();
        tokio::time::sleep(std::time::Duration::from_millis(5)).await;
        p.get_or_create(payload("h3", "a", 10_000)).await.unwrap();

        // After the 3rd insert (30k total > 25k budget), h1 was evicted.
        assert!(p.peek(&"h1".into()).await.is_none());
        assert!(p.peek(&"h2".into()).await.is_some());
        assert!(p.peek(&"h3".into()).await.is_some());
    }

    #[tokio::test]
    async fn evict_by_owner_only_targets_matching_owner() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 1,
                ..Default::default()
            },
        );
        p.get_or_create(payload("h1", "agent-a", 5_000))
            .await
            .unwrap();
        p.get_or_create(payload("h2", "agent-b", 5_000))
            .await
            .unwrap();
        p.get_or_create(payload("h3", "agent-a", 5_000))
            .await
            .unwrap();

        let deleted = p.evict_by_owner("agent-a").await;
        assert_eq!(deleted.len(), 2);
        assert_eq!(p.entry_count(), 1);
        assert!(p.peek(&"h2".into()).await.is_some());
    }

    #[tokio::test]
    async fn shutdown_deletes_all_entries() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 1,
                ..Default::default()
            },
        );
        p.get_or_create(payload("h1", "a", 1_000)).await.unwrap();
        p.get_or_create(payload("h2", "b", 1_000)).await.unwrap();
        p.get_or_create(payload("h3", "c", 1_000)).await.unwrap();

        p.shutdown().await;
        assert_eq!(p.entry_count(), 0);
        assert_eq!(backend.live_ids().len(), 0);
        assert_eq!(backend.delete_calls.load(Ordering::Relaxed), 3);
    }

    #[tokio::test]
    async fn backend_create_failure_propagates() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        backend.fail_next_create.store(true, Ordering::Relaxed);

        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 1,
                ..Default::default()
            },
        );
        let r = p.get_or_create(payload("h", "a", 1_000)).await;
        assert!(r.is_err());
        assert_eq!(p.entry_count(), 0);
    }

    #[tokio::test]
    async fn expired_entry_triggers_recreate() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 1,
                ..Default::default()
            },
        );
        // Create with a very short TTL.
        let mut pl = payload("h", "a", 1_000);
        pl.ttl_secs = 1;
        p.get_or_create(pl.clone()).await.unwrap();
        // Force expiry by rewinding the stored expires_at.
        {
            let handle = p.entries.get(&"h".to_string()).unwrap();
            let entry_arc = handle.value().clone();
            drop(handle);
            let mut e = entry_arc.lock().await;
            e.expires_at = Utc::now() - ChronoDuration::seconds(1);
        }
        let r = p.get_or_create(pl).await.unwrap();
        assert!(matches!(r, CacheLookup::Miss { .. }));
        assert_eq!(backend.create_calls.load(Ordering::Relaxed), 2);
    }

    #[tokio::test]
    async fn evict_expired_removes_only_past_due() {
        let backend = Arc::new(InMemoryBackend::new(CacheProvider::GeminiExplicit));
        let p = pool(
            backend.clone(),
            PoolConfig {
                min_tokens_to_cache: 1,
                ..Default::default()
            },
        );
        p.get_or_create(payload("live", "a", 1_000)).await.unwrap();
        p.get_or_create(payload("dead", "a", 1_000)).await.unwrap();

        // Mark "dead" expired.
        {
            let handle = p.entries.get(&"dead".to_string()).unwrap();
            let entry_arc = handle.value().clone();
            drop(handle);
            let mut e = entry_arc.lock().await;
            e.expires_at = Utc::now() - ChronoDuration::seconds(10);
        }

        let removed = p.evict_expired().await;
        assert_eq!(removed.len(), 1);
        assert!(p.peek(&"live".into()).await.is_some());
        assert!(p.peek(&"dead".into()).await.is_none());
    }

    #[test]
    fn pool_config_default_is_sensible() {
        let cfg = PoolConfig::default();
        assert!(cfg.max_entries >= 16);
        assert!(cfg.max_storage_tokens >= 1_000_000);
        assert!(cfg.min_tokens_to_cache >= 1_000);
        assert!(cfg.default_ttl_secs >= 60);
    }

    #[test]
    fn gemini_backend_urls_compose_correctly() {
        let b = GeminiExplicitBackend::new("https://generativelanguage.googleapis.com/", "KEY");
        // Smoke check the identifiers used in URL formation.
        assert_eq!(b.provider(), CacheProvider::GeminiExplicit);
        assert_eq!(b.base_url, "https://generativelanguage.googleapis.com/");
        assert_eq!(b.api_key, "KEY");
    }

    #[test]
    fn kimi_backend_urls_compose_correctly() {
        let b = KimiV1Backend::new("https://api.moonshot.ai", "KEY");
        assert_eq!(b.provider(), CacheProvider::KimiV1);
        assert_eq!(b.base_url, "https://api.moonshot.ai");
        assert_eq!(b.api_key, "KEY");
    }
}
