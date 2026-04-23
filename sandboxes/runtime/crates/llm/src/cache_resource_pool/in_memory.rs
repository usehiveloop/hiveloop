//! Fake in-memory backend used for tests and as a reference implementation.

use async_trait::async_trait;
use chrono::{DateTime, Duration as ChronoDuration, Utc};
use dashmap::DashMap;

use super::{BackendError, CachePayload, CacheProvider, CacheResourceBackend, CreatedCache};

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
