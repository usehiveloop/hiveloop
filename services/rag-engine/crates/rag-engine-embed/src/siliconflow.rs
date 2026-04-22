//! SiliconFlow (OpenAI-compatible) embedder implementation.
//!
//! SiliconFlow publishes a drop-in replacement for OpenAI's
//! `/v1/embeddings` surface. The same code path works against OpenAI
//! itself by swapping the base URL and API key.
//!
//! # Retry policy
//!
//! Exponential backoff via `reqwest-retry` on 429 + 5xx + transient
//! network errors. Max retries is capped so upstream batches fail loudly
//! rather than silently stall the indexing pipeline.
//!
//! # Concurrency
//!
//! A `tokio::sync::Semaphore` bounds the number of simultaneous HTTP
//! calls. SiliconFlow rate-limits per API key; overshooting the semaphore
//! produces pointless retries that eat budget.
//!
//! # Prefixing
//!
//! Qwen3 embedding models expect `"query: "` / `"passage: "` prefixes.
//! If the caller supplied them at config time, they're prepended at the
//! wire layer so downstream code stays prefix-agnostic. The registry in
//! Go-land (`internal/rag/embedder/registry.go`) carries these per model.

use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use futures::stream::{FuturesUnordered, StreamExt};
use reqwest_middleware::{ClientBuilder, ClientWithMiddleware};
use reqwest_retry::{policies::ExponentialBackoff, RetryTransientMiddleware};
use tracing::{debug, instrument, warn};

pub use crate::batching::DEFAULT_BATCH_SIZE;
use crate::errors::EmbedError;
use crate::types::{EmbedKind, EmbeddingRequest, EmbeddingResponse};
use crate::Embedder;

/// Default upstream timeout per HTTP call.
pub const DEFAULT_TIMEOUT_SECS: u64 = 10;
/// Default bounded concurrency per embedder instance.
pub const DEFAULT_CONCURRENCY: usize = 4;
/// Default retry budget before giving up on a single sub-batch.
pub const DEFAULT_MAX_RETRIES: u32 = 4;

/// Options for constructing a `SiliconFlowEmbedder`. Parallel to the
/// `EmbedderConfig::SiliconFlow` variant — kept separate so direct
/// construction (in tests, in non-figment contexts) is ergonomic.
#[derive(Debug, Clone)]
pub struct SiliconFlowOptions {
    /// Stable id, e.g. `"siliconflow:qwen3-embedding-4b"`.
    pub id: String,
    /// Remote model name, e.g. `"Qwen/Qwen3-Embedding-4B"`.
    pub model_name: String,
    /// Expected output dimension. Each batch's response is validated
    /// against this; mismatches become `InvalidResponse`.
    pub dimension: u32,
    /// Reported `max_input_tokens` (informational for callers).
    pub max_input_tokens: u32,
    /// Base URL of the compatible endpoint. Must not include
    /// `/embeddings`.
    pub base_url: String,
    /// Bearer token injected via `authorization` header.
    pub api_key: String,
    /// Optional passage prefix (e.g. `"passage: "`).
    pub passage_prefix: Option<String>,
    /// Optional query prefix (e.g. `"query: "`).
    pub query_prefix: Option<String>,
    /// Per-call timeout.
    pub timeout: Duration,
    /// Max sub-batches in flight per call tree.
    pub concurrency: usize,
    /// Max per-call input size before we split.
    pub batch_size: usize,
    /// Retry budget for transient failures per sub-batch.
    pub max_retries: u32,
}

impl SiliconFlowOptions {
    fn validate(&self) -> Result<(), EmbedError> {
        if self.api_key.trim().is_empty() {
            return Err(EmbedError::Config("api_key is empty".into()));
        }
        if !(self.base_url.starts_with("http://") || self.base_url.starts_with("https://")) {
            return Err(EmbedError::Config(format!(
                "base_url must start with http(s)://, got: {}",
                self.base_url
            )));
        }
        if self.dimension == 0 {
            return Err(EmbedError::Config("dimension must be > 0".into()));
        }
        if self.model_name.trim().is_empty() {
            return Err(EmbedError::Config("model_name is empty".into()));
        }
        Ok(())
    }
}

/// Real HTTP-backed embedder targeting a SiliconFlow (or OpenAI-compatible)
/// endpoint.
pub struct SiliconFlowEmbedder {
    opts: SiliconFlowOptions,
    client: ClientWithMiddleware,
    endpoint: String,
    sem: Arc<tokio::sync::Semaphore>,
}

impl SiliconFlowEmbedder {
    /// Construct an embedder. Returns `EmbedError::Config` if options are
    /// malformed (empty API key, bad base URL, zero dim).
    pub fn new(opts: SiliconFlowOptions) -> Result<Self, EmbedError> {
        opts.validate()?;

        let http = reqwest::Client::builder()
            .timeout(opts.timeout)
            .user_agent(concat!("rag-engine-embed/", env!("CARGO_PKG_VERSION")))
            .build()
            .map_err(|e| EmbedError::Config(format!("reqwest client build: {e}")))?;

        let retry_policy = ExponentialBackoff::builder()
            .retry_bounds(Duration::from_millis(100), Duration::from_secs(2))
            .build_with_max_retries(opts.max_retries);

        let client = ClientBuilder::new(http)
            .with(RetryTransientMiddleware::new_with_policy(retry_policy))
            .build();

        let endpoint = format!("{}/embeddings", opts.base_url.trim_end_matches('/'));

        let sem = Arc::new(tokio::sync::Semaphore::new(opts.concurrency.max(1)));

        Ok(Self {
            opts,
            client,
            endpoint,
            sem,
        })
    }

    fn apply_prefix(&self, text: &str, kind: EmbedKind) -> String {
        let prefix = match kind {
            EmbedKind::Passage => self.opts.passage_prefix.as_deref(),
            EmbedKind::Query => self.opts.query_prefix.as_deref(),
        };
        match prefix {
            Some(p) if !p.is_empty() => {
                let mut out = String::with_capacity(p.len() + text.len());
                out.push_str(p);
                out.push_str(text);
                out
            }
            _ => text.to_string(),
        }
    }

    /// Execute a single sub-batch against the upstream. Bounded by the
    /// per-instance semaphore.
    #[instrument(skip_all, fields(model = %self.opts.model_name, batch_size = texts.len()))]
    async fn embed_one_batch(&self, texts: Vec<String>) -> Result<Vec<Vec<f32>>, EmbedError> {
        let _permit = self
            .sem
            .acquire()
            .await
            .map_err(|e| EmbedError::Transport(format!("semaphore closed: {e}")))?;

        let body = EmbeddingRequest {
            model: &self.opts.model_name,
            input: texts,
            encoding_format: "float",
        };

        let resp = self
            .client
            .post(&self.endpoint)
            .bearer_auth(&self.opts.api_key)
            .json(&body)
            .send()
            .await
            .map_err(|e| EmbedError::Transport(e.to_string()))?;

        let status = resp.status();
        if status.as_u16() == 429 {
            warn!(status = 429, "siliconflow rate-limited after max retries");
            return Err(EmbedError::RateLimited);
        }
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(EmbedError::Upstream {
                status: status.as_u16(),
                body,
            });
        }

        let parsed: EmbeddingResponse = resp
            .json()
            .await
            .map_err(|e| EmbedError::InvalidResponse(format!("json parse: {e}")))?;

        if parsed.data.is_empty() {
            return Err(EmbedError::InvalidResponse("empty data array".into()));
        }

        // Upstream may return rows in `index` order; the OpenAI spec says
        // they come back in input order. SiliconFlow honors this. Trust it
        // but double-check dimensions.
        let dim = self.opts.dimension as usize;
        let mut out: Vec<Vec<f32>> = Vec::with_capacity(parsed.data.len());
        for row in parsed.data {
            if row.embedding.len() != dim {
                return Err(EmbedError::InvalidResponse(format!(
                    "expected dim {dim}, got {}",
                    row.embedding.len()
                )));
            }
            out.push(row.embedding);
        }
        debug!(count = out.len(), "siliconflow batch ok");
        Ok(out)
    }
}

#[async_trait]
impl Embedder for SiliconFlowEmbedder {
    fn id(&self) -> &str {
        &self.opts.id
    }

    fn dimension(&self) -> u32 {
        self.opts.dimension
    }

    fn max_input_tokens(&self) -> u32 {
        self.opts.max_input_tokens
    }

    async fn embed(
        &self,
        texts: Vec<String>,
        kind: EmbedKind,
    ) -> Result<Vec<Vec<f32>>, EmbedError> {
        if texts.is_empty() {
            return Ok(Vec::new());
        }

        // Prefixing happens before batching so the wire format is always
        // prefix-aware.
        let prefixed: Vec<String> = texts.iter().map(|t| self.apply_prefix(t, kind)).collect();

        // If the input already fits in a single call, short-circuit to
        // avoid the fan-out overhead.
        if prefixed.len() <= self.opts.batch_size {
            return self.embed_one_batch(prefixed).await;
        }

        // Split into sub-batches and drive with bounded concurrency via
        // FuturesUnordered. We don't use the generic `embed_batched`
        // helper here because we already prefixed the inputs and don't
        // want the helper to re-enter `embed` (which would re-prefix).
        let batch_size = self.opts.batch_size.max(1);
        let mut sub_batches: Vec<(usize, Vec<String>)> = Vec::new();
        let mut offset = 0usize;
        let mut iter = prefixed.into_iter();
        loop {
            let chunk: Vec<String> = iter.by_ref().take(batch_size).collect();
            if chunk.is_empty() {
                break;
            }
            let len = chunk.len();
            sub_batches.push((offset, chunk));
            offset += len;
        }

        let total = offset;
        let mut out: Vec<Option<Vec<f32>>> = (0..total).map(|_| None).collect();

        let mut inflight = FuturesUnordered::new();
        let mut iter = sub_batches.into_iter();

        // Prime up to `concurrency` futures, then maintain that level as
        // tasks complete. Concurrency is also bounded by the per-instance
        // semaphore inside `embed_one_batch`, but capping here avoids
        // allocating a future per sub-batch up front.
        let cap = self.opts.concurrency.max(1);
        for _ in 0..cap {
            if let Some((idx, batch)) = iter.next() {
                inflight.push(run(self, idx, batch));
            } else {
                break;
            }
        }
        while let Some(res) = inflight.next().await {
            let (idx, vectors) = res?;
            for (i, v) in vectors.into_iter().enumerate() {
                out[idx + i] = Some(v);
            }
            if let Some((idx, batch)) = iter.next() {
                inflight.push(run(self, idx, batch));
            }
        }

        out.into_iter()
            .enumerate()
            .map(|(i, slot)| {
                slot.ok_or_else(|| {
                    EmbedError::InvalidResponse(format!(
                        "missing vector at position {i} after batching"
                    ))
                })
            })
            .collect()
    }
}

// Free helper that owns the returned future's lifetime tied to `&self`.
async fn run(
    emb: &SiliconFlowEmbedder,
    offset: usize,
    batch: Vec<String>,
) -> Result<(usize, Vec<Vec<f32>>), EmbedError> {
    let vectors = emb.embed_one_batch(batch).await?;
    Ok((offset, vectors))
}

impl Default for SiliconFlowOptions {
    fn default() -> Self {
        Self {
            id: "siliconflow:qwen3-embedding-4b".into(),
            model_name: "Qwen/Qwen3-Embedding-4B".into(),
            dimension: 2560,
            max_input_tokens: 8192,
            base_url: "https://api.siliconflow.cn/v1".into(),
            api_key: String::new(),
            passage_prefix: Some("passage: ".into()),
            query_prefix: Some("query: ".into()),
            timeout: Duration::from_secs(DEFAULT_TIMEOUT_SECS),
            concurrency: DEFAULT_CONCURRENCY,
            batch_size: DEFAULT_BATCH_SIZE,
            max_retries: DEFAULT_MAX_RETRIES,
        }
    }
}
