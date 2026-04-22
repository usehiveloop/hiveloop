//! OpenAI-compatible embedder, built on top of the `async-openai` crate.
//!
//! Works against any provider that speaks the OpenAI `/v1/embeddings`
//! surface — SiliconFlow, OpenRouter, Groq, OpenAI itself, Together, etc.
//! The provider is selected purely by `api_url` + `api_key`; there is no
//! per-provider branching in this module.
//!
//! # Retry policy
//!
//! `async-openai` has built-in exponential backoff on 429 + 5xx, wired via
//! the `backoff` crate. We configure it at construction time with a
//! bounded retry budget so upstream batches fail loudly rather than
//! silently stall the indexing pipeline.
//!
//! # Concurrency
//!
//! Per-instance bounded concurrency across sub-batches via a
//! `FuturesUnordered` loop, identical in shape to the pre-refactor impl.
//!
//! # Prefixing
//!
//! Qwen3 embedding models expect `"query: "` / `"passage: "` prefixes.
//! If configured, they're prepended at the wire layer so downstream code
//! stays prefix-agnostic. The catalog in Go-land
//! (`internal/rag/embedder/registry.go`) carries defaults per model.

use std::sync::Arc;
use std::time::Duration;

use async_openai::config::OpenAIConfig;
use async_openai::error::OpenAIError;
use async_openai::types::embeddings::{CreateEmbeddingRequestArgs, EmbeddingInput};
use async_openai::Client;
use async_trait::async_trait;
use futures::stream::{FuturesUnordered, StreamExt};
use tracing::{debug, instrument, warn};

pub use crate::batching::DEFAULT_BATCH_SIZE;
use crate::errors::EmbedError;
use crate::types::EmbedKind;
use crate::Embedder;

/// Default upstream timeout per HTTP call.
pub const DEFAULT_TIMEOUT_SECS: u64 = 30;
/// Default bounded concurrency per embedder instance.
pub const DEFAULT_CONCURRENCY: usize = 4;
/// Default retry budget before giving up on a single sub-batch. Passed to
/// `async-openai`'s built-in `backoff::ExponentialBackoff` as
/// `max_elapsed_time ≈ max_retries * max_interval`.
pub const DEFAULT_MAX_RETRIES: u32 = 4;

/// Options for constructing an `OpenAICompatEmbedder`. Parallel to
/// `EmbedderConfig::OpenAICompat` — kept separate so direct construction
/// (in tests, in non-figment contexts) is ergonomic.
#[derive(Debug, Clone)]
pub struct OpenAICompatOptions {
    /// Stable id, e.g. `"siliconflow:qwen3-embedding-4b"`. Used for
    /// metrics tagging + logging; not sent to the upstream.
    pub id: String,
    /// Remote model name, e.g. `"Qwen/Qwen3-Embedding-4B"`.
    pub model_name: String,
    /// Expected output dimension. Each batch's response is validated
    /// against this; mismatches become `InvalidResponse`.
    pub dimension: u32,
    /// Reported `max_input_tokens` (informational for callers).
    pub max_input_tokens: u32,
    /// Base URL of the compatible endpoint, including the `/v1` suffix
    /// but NOT `/embeddings` (e.g. `https://api.siliconflow.cn/v1`).
    pub api_url: String,
    /// Bearer token. Passed through `OpenAIConfig::with_api_key`.
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
    /// Retry budget for transient failures (429, 5xx) per sub-batch.
    pub max_retries: u32,
}

impl OpenAICompatOptions {
    fn validate(&self) -> Result<(), EmbedError> {
        if self.api_key.trim().is_empty() {
            return Err(EmbedError::Config("api_key is empty".into()));
        }
        if !(self.api_url.starts_with("http://") || self.api_url.starts_with("https://")) {
            return Err(EmbedError::Config(format!(
                "api_url must start with http(s)://, got: {}",
                self.api_url
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

impl Default for OpenAICompatOptions {
    fn default() -> Self {
        Self {
            id: "openai-compat:default".into(),
            model_name: "text-embedding-3-small".into(),
            dimension: 1536,
            max_input_tokens: 8192,
            api_url: "https://api.openai.com/v1".into(),
            api_key: String::new(),
            passage_prefix: None,
            query_prefix: None,
            timeout: Duration::from_secs(DEFAULT_TIMEOUT_SECS),
            concurrency: DEFAULT_CONCURRENCY,
            batch_size: DEFAULT_BATCH_SIZE,
            max_retries: DEFAULT_MAX_RETRIES,
        }
    }
}

/// OpenAI-compatible embedder. Drops into any provider that implements
/// the `/v1/embeddings` surface.
pub struct OpenAICompatEmbedder {
    opts: OpenAICompatOptions,
    client: Client<OpenAIConfig>,
}

impl OpenAICompatEmbedder {
    /// Construct an embedder. Returns `EmbedError::Config` if options are
    /// malformed (empty API key, bad api_url, zero dim, empty model).
    pub fn new(opts: OpenAICompatOptions) -> Result<Self, EmbedError> {
        opts.validate()?;

        // Custom reqwest client so we can pin timeout + user-agent. Other
        // knobs (connection pool, proxy, TLS backend) fall back to reqwest
        // defaults; the workspace Cargo.toml selects `rustls-tls`.
        let http = reqwest::Client::builder()
            .timeout(opts.timeout)
            .user_agent(concat!("rag-engine-embed/", env!("CARGO_PKG_VERSION")))
            .build()
            .map_err(|e| EmbedError::Config(format!("reqwest client build: {e}")))?;

        let config = OpenAIConfig::new()
            .with_api_base(opts.api_url.trim_end_matches('/'))
            .with_api_key(&opts.api_key);

        // `async-openai`'s retry budget is expressed as a `backoff::
        // ExponentialBackoff`. We translate our `max_retries` into a
        // total elapsed time budget: the default interval starts at
        // 100ms and doubles up to 2s, so ~4 retries corresponds to
        // roughly 100+200+400+800+1600 = 3100ms of retry time. Cap
        // `max_elapsed_time` accordingly so the client gives up instead
        // of backing off forever.
        let backoff = backoff::ExponentialBackoffBuilder::new()
            .with_initial_interval(Duration::from_millis(100))
            .with_max_interval(Duration::from_secs(2))
            .with_max_elapsed_time(Some(max_retries_to_elapsed(opts.max_retries)))
            .build();

        let client = Client::with_config(config)
            .with_http_client(http)
            .with_backoff(backoff);

        Ok(Self { opts, client })
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

    /// Execute a single sub-batch against the upstream.
    #[instrument(skip_all, fields(model = %self.opts.model_name, batch_size = texts.len()))]
    async fn embed_one_batch(&self, texts: Vec<String>) -> Result<Vec<Vec<f32>>, EmbedError> {
        let request = CreateEmbeddingRequestArgs::default()
            .model(&self.opts.model_name)
            .input(EmbeddingInput::StringArray(texts))
            .build()
            .map_err(|e| EmbedError::Config(format!("embedding request build: {e}")))?;

        let response = self
            .client
            .embeddings()
            .create(request)
            .await
            .map_err(map_openai_error)?;

        if response.data.is_empty() {
            return Err(EmbedError::InvalidResponse("empty data array".into()));
        }

        let dim = self.opts.dimension as usize;
        let mut out: Vec<Vec<f32>> = Vec::with_capacity(response.data.len());
        // OpenAI spec guarantees ordering by input index. SiliconFlow
        // honors it; we validate dimension per-row anyway.
        for row in response.data {
            if row.embedding.len() != dim {
                return Err(EmbedError::InvalidResponse(format!(
                    "expected dim {dim}, got {}",
                    row.embedding.len()
                )));
            }
            out.push(row.embedding);
        }
        debug!(count = out.len(), "openai-compat batch ok");
        Ok(out)
    }
}

/// Convert an `async-openai` error into our `EmbedError` variants.
///
/// `async-openai` loses the HTTP status code before the error reaches us:
/// non-2xx responses are parsed into an `ApiError` whose only surface is a
/// message string. We use a tiny heuristic on that message to separate
/// 429-after-retries from terminal 4xx. The retry budget itself is handled
/// *inside* `async-openai` via `backoff`, so by the time we see an error,
/// it's already exhausted.
fn map_openai_error(e: OpenAIError) -> EmbedError {
    match e {
        OpenAIError::Reqwest(err) => EmbedError::Transport(err.to_string()),
        OpenAIError::JSONDeserialize(err, content) => {
            EmbedError::InvalidResponse(format!("json parse: {err}; content: {content}"))
        }
        OpenAIError::ApiError(api_err) => {
            // The message is populated from the upstream body. Some
            // providers include `"rate limit"` / `"429"` / `"Too Many
            // Requests"` verbatim when they refuse. We flag those as
            // `RateLimited` so callers can distinguish transient from
            // terminal. Anything else becomes `Upstream` with status=0
            // (we lost it) — the body is preserved for triage.
            let message = api_err.message.clone();
            let lc = message.to_ascii_lowercase();
            if lc.contains("rate limit") || lc.contains("too many requests") || lc.contains("429") {
                warn!(msg = %message, "rate-limited after max retries");
                EmbedError::RateLimited
            } else {
                EmbedError::Upstream {
                    status: 0,
                    body: message,
                }
            }
        }
        OpenAIError::InvalidArgument(msg) => {
            EmbedError::InvalidResponse(format!("invalid argument: {msg}"))
        }
        OpenAIError::StreamError(err) => EmbedError::Transport(format!("stream: {err}")),
        OpenAIError::FileSaveError(msg) => EmbedError::Transport(format!("file save: {msg}")),
        OpenAIError::FileReadError(msg) => EmbedError::Transport(format!("file read: {msg}")),
    }
}

/// Translate `max_retries` to a `max_elapsed_time` budget. Each retry
/// doubles the interval starting at 100ms, capped at 2s: the sum of a
/// geometric series caps out around `max_retries * 2s` in the worst case.
/// We add 1s slack so the final retry has headroom before the deadline.
fn max_retries_to_elapsed(max_retries: u32) -> Duration {
    // Conservative upper bound that matches intuitive "retry N times" but
    // gives `backoff` enough time to actually execute the final attempt.
    Duration::from_secs(u64::from(max_retries) * 3 + 1)
}

// Free helper that owns the returned future's lifetime tied to `&self`.
async fn run(
    emb: &OpenAICompatEmbedder,
    offset: usize,
    batch: Vec<String>,
) -> Result<(usize, Vec<Vec<f32>>), EmbedError> {
    let vectors = emb.embed_one_batch(batch).await?;
    Ok((offset, vectors))
}

#[async_trait]
impl Embedder for OpenAICompatEmbedder {
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
        // prefix-aware regardless of sub-batch splits.
        let prefixed: Vec<String> = texts.iter().map(|t| self.apply_prefix(t, kind)).collect();

        // If the input already fits in a single call, short-circuit the
        // fan-out machinery.
        if prefixed.len() <= self.opts.batch_size {
            return self.embed_one_batch(prefixed).await;
        }

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
        // tasks complete.
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

/// Keep the `Arc` constructor visible to `build()`.
pub(crate) fn into_arc(emb: OpenAICompatEmbedder) -> Arc<dyn Embedder> {
    Arc::new(emb)
}
