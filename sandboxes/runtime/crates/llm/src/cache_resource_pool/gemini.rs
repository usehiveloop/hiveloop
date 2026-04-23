//! Gemini explicit `CachedContent` backend.
//!
//! Endpoint: `POST {base}/v1beta/cachedContents`, body shape:
//! ```json
//! { "model": "models/gemini-2.5-pro",
//!   "contents": [...], "systemInstruction": {...},
//!   "tools": [...], "ttl": "3600s" }
//! ```
//! Response: `{ "name": "cachedContents/abc123", "expireTime": "..." }`.
//! Delete: `DELETE {base}/v1beta/{name}`. Update TTL:
//! `PATCH {base}/v1beta/{name}` with `{"ttl": "3600s"}`.

use async_trait::async_trait;
use chrono::{DateTime, Duration as ChronoDuration, Utc};

use super::{BackendError, CachePayload, CacheProvider, CacheResourceBackend, CreatedCache};

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
