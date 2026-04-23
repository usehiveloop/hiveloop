//! Kimi moonshot-v1 `/v1/caching` backend.
//!
//! Endpoint: `POST {base}/v1/caching` with bearer auth.
//! Body:
//! ```json
//! { "model": "moonshot-v1-128k",
//!   "messages": [...], "tools": [...],
//!   "name": "...", "description": "...", "ttl": 3600 }
//! ```
//! Response: `{ "id": "cache-xxx", "expires_at": <unix_ts> }`.
//! Delete: `DELETE {base}/v1/caching/{id}`. Renew:
//! `PUT {base}/v1/caching/{id}/reset` with `{"ttl": 3600}`.

use async_trait::async_trait;
use chrono::{DateTime, Duration as ChronoDuration, Utc};

use super::{BackendError, CachePayload, CacheProvider, CacheResourceBackend, CreatedCache};

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
