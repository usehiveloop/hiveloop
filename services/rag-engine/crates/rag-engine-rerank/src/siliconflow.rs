//! `SiliconFlowReranker` — HTTP client for SiliconFlow's `/v1/rerank`
//! endpoint, compatible with Cohere-style payloads.
//!
//! Reference: <https://docs.siliconflow.com/en/api-reference/rerank/create-rerank>.
//! Endpoint: `POST {base_url}/rerank`.
//! Request:
//! ```json
//! { "model": "Qwen/Qwen3-Reranker-0.6B",
//!   "query": "...",
//!   "documents": ["...", "..."],
//!   "return_documents": false }
//! ```
//! Response:
//! ```json
//! { "id": "...",
//!   "results": [ { "index": 0, "relevance_score": 0.87 }, ... ],
//!   "tokens": { "input_tokens": N, "output_tokens": N } }
//! ```
//!
//! Transport stack mirrors Tranche 2C: `reqwest` + `reqwest-middleware`
//! with `reqwest-retry` `ExponentialBackoff` on 5xx / 429.

use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION, CONTENT_TYPE};
use reqwest::StatusCode;
use reqwest_middleware::{ClientBuilder, ClientWithMiddleware};
use reqwest_retry::policies::ExponentialBackoff;
use reqwest_retry::RetryTransientMiddleware;
use serde::{Deserialize, Serialize};
use url::Url;

use crate::batching::{self, MAX_CANDIDATES_PER_CALL};
use crate::{RerankError, Reranker};

pub const DEFAULT_BASE_URL: &str = "https://api.siliconflow.com/v1";
pub const DEFAULT_MODEL: &str = "Qwen/Qwen3-Reranker-0.6B";
pub const DEFAULT_TIMEOUT: Duration = Duration::from_secs(30);
pub const DEFAULT_MAX_RETRIES: u32 = 3;

/// Static configuration for a `SiliconFlowReranker`.
#[derive(Debug, Clone)]
pub struct SiliconFlowRerankerConfig {
    pub base_url: String,
    pub model: String,
    pub api_key: String,
    pub timeout: Duration,
    pub max_retries: u32,
    pub batch_size: usize,
}

impl Default for SiliconFlowRerankerConfig {
    fn default() -> Self {
        Self {
            base_url: DEFAULT_BASE_URL.to_string(),
            model: DEFAULT_MODEL.to_string(),
            api_key: String::new(),
            timeout: DEFAULT_TIMEOUT,
            max_retries: DEFAULT_MAX_RETRIES,
            batch_size: MAX_CANDIDATES_PER_CALL,
        }
    }
}

/// SiliconFlow Qwen3-Reranker client.
#[derive(Debug, Clone)]
pub struct SiliconFlowReranker {
    cfg: Arc<SiliconFlowRerankerConfig>,
    http: ClientWithMiddleware,
    rerank_url: Url,
    id: String,
}

#[derive(Debug, Serialize)]
struct RerankRequest<'a> {
    model: &'a str,
    query: &'a str,
    documents: &'a [String],
    return_documents: bool,
}

#[derive(Debug, Deserialize)]
struct RerankResponse {
    #[serde(default)]
    results: Vec<RerankResultItem>,
}

#[derive(Debug, Deserialize)]
struct RerankResultItem {
    index: usize,
    relevance_score: f32,
}

impl SiliconFlowReranker {
    pub fn new(cfg: SiliconFlowRerankerConfig) -> Result<Self, RerankError> {
        if cfg.api_key.trim().is_empty() {
            return Err(RerankError::Config("api_key is empty".to_string()));
        }
        if cfg.base_url.trim().is_empty() {
            return Err(RerankError::Config("base_url is empty".to_string()));
        }
        if cfg.model.trim().is_empty() {
            return Err(RerankError::Config("model is empty".to_string()));
        }

        let base = Url::parse(cfg.base_url.trim_end_matches('/'))
            .map_err(|e| RerankError::Config(format!("invalid base_url: {e}")))?;
        let rerank_url = base
            .join(&format!("{}/rerank", base.path().trim_end_matches('/')))
            .or_else(|_| Url::parse(&format!("{}/rerank", cfg.base_url.trim_end_matches('/'))))
            .map_err(|e| RerankError::Config(format!("invalid base_url/rerank: {e}")))?;

        let mut headers = HeaderMap::new();
        let auth = HeaderValue::from_str(&format!("Bearer {}", cfg.api_key))
            .map_err(|e| RerankError::Config(format!("invalid api_key: {e}")))?;
        headers.insert(AUTHORIZATION, auth);
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));

        let inner = reqwest::Client::builder()
            .timeout(cfg.timeout)
            .default_headers(headers)
            .build()
            .map_err(|e| RerankError::Config(format!("client build: {e}")))?;

        let retry_policy = ExponentialBackoff::builder()
            .retry_bounds(Duration::from_millis(50), Duration::from_secs(5))
            .build_with_max_retries(cfg.max_retries);

        let http = ClientBuilder::new(inner)
            .with(RetryTransientMiddleware::new_with_policy(retry_policy))
            .build();

        let id = format!("siliconflow:{}", cfg.model);

        Ok(Self {
            cfg: Arc::new(cfg),
            http,
            rerank_url,
            id,
        })
    }

    async fn rerank_batch(
        &self,
        query: &str,
        documents: &[String],
    ) -> Result<Vec<f32>, RerankError> {
        let body = RerankRequest {
            model: &self.cfg.model,
            query,
            documents,
            return_documents: false,
        };

        let resp = self
            .http
            .post(self.rerank_url.clone())
            .json(&body)
            .send()
            .await
            .map_err(map_send_error)?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(RerankError::UpstreamStatus {
                status: status.as_u16(),
                body,
            });
        }

        let parsed: RerankResponse = resp
            .json()
            .await
            .map_err(|e| RerankError::Decode(format!("json: {e}")))?;

        let mut scores = vec![0.0f32; documents.len()];
        let mut seen = vec![false; documents.len()];
        for item in parsed.results {
            if item.index >= documents.len() {
                return Err(RerankError::Decode(format!(
                    "result index {} out of range (n={})",
                    item.index,
                    documents.len()
                )));
            }
            // Clamp to [0,1] so downstream code can rely on the contract.
            let s = item.relevance_score.clamp(0.0, 1.0);
            scores[item.index] = s;
            seen[item.index] = true;
        }
        // If upstream silently omitted entries (top_n behaviour we don't
        // request, or partial results), fill in 0.0 for unseen — callers
        // treat them as least relevant. Flag only if the entire response
        // was empty which would be a spec violation.
        if !seen.iter().any(|s| *s) && !documents.is_empty() {
            return Err(RerankError::Decode(
                "empty results for non-empty documents".to_string(),
            ));
        }
        Ok(scores)
    }
}

fn map_send_error(e: reqwest_middleware::Error) -> RerankError {
    match e {
        reqwest_middleware::Error::Middleware(err) => RerankError::Http(err.to_string()),
        reqwest_middleware::Error::Reqwest(err) => {
            if err.is_timeout() {
                RerankError::Timeout
            } else if let Some(status) = err.status() {
                if status == StatusCode::REQUEST_TIMEOUT {
                    RerankError::Timeout
                } else {
                    RerankError::Http(err.to_string())
                }
            } else {
                RerankError::Http(err.to_string())
            }
        }
    }
}

#[async_trait]
impl Reranker for SiliconFlowReranker {
    fn id(&self) -> &str {
        &self.id
    }

    async fn rerank(&self, query: &str, candidates: Vec<String>) -> Result<Vec<f32>, RerankError> {
        if candidates.is_empty() {
            return Ok(Vec::new());
        }
        if query.is_empty() {
            return Err(RerankError::Invalid("query is empty".to_string()));
        }

        let total = candidates.len();
        let batches = batching::split(&candidates, self.cfg.batch_size);
        let mut parts: Vec<(usize, Vec<f32>)> = Vec::with_capacity(batches.len());
        for batch in batches {
            let scores = self.rerank_batch(query, batch.items).await?;
            parts.push((batch.offset, scores));
        }
        Ok(batching::merge(total, parts))
    }
}
