use super::*;
use chrono::{Duration as ChronoDuration, Utc};
use std::sync::atomic::Ordering;
use std::sync::Arc;

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
