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

use async_trait::async_trait;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

mod gemini;
mod in_memory;
mod kimi;
mod pool;

#[cfg(test)]
mod tests;

pub use gemini::GeminiExplicitBackend;
pub use in_memory::InMemoryBackend;
pub use kimi::KimiV1Backend;
pub use pool::CacheResourcePool;

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
