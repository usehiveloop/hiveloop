//! Figment-driven configuration for the embedder factory.
//!
//! # Provider-agnostic env contract
//!
//! The OpenAI-compatible embedder is configured purely via env vars:
//!
//! | Env var                   | Required | Default                 | Notes                                                    |
//! |---------------------------|----------|-------------------------|----------------------------------------------------------|
//! | `LLM_API_URL`             | yes      | —                       | Base URL, e.g. `https://api.siliconflow.cn/v1`.          |
//! | `LLM_API_KEY`             | yes      | —                       | Bearer token.                                            |
//! | `LLM_MODEL`               | yes      | —                       | Model name, e.g. `Qwen/Qwen3-Embedding-4B`.              |
//! | `LLM_EMBEDDING_DIM`       | yes      | —                       | Output dimension. No safe default — must match Lance.    |
//! | `LLM_ID`                  | no       | derived from model      | Stable id for logs/metrics.                              |
//! | `LLM_QUERY_PREFIX`        | no       | `"query: "` for qwen*   | Set empty string to disable auto-default.                |
//! | `LLM_PASSAGE_PREFIX`      | no       | `"passage: "` for qwen* | Set empty string to disable auto-default.                |
//! | `LLM_MAX_INPUT_TOKENS`    | no       | `8192`                  |                                                          |
//! | `LLM_REQUEST_TIMEOUT_SECS`| no       | `30`                    |                                                          |
//! | `LLM_BATCH_SIZE`          | no       | `32`                    | Max inputs per sub-batch.                                |
//! | `LLM_CONCURRENCY`         | no       | `4`                     | Max in-flight sub-batches.                               |
//! | `LLM_MAX_RETRIES`         | no       | `4`                     | Retry budget for 429/5xx.                                |
//!
//! Same config works across SiliconFlow, OpenRouter, Groq, OpenAI,
//! Together, or any other provider that speaks `/v1/embeddings`. Only
//! `LLM_API_URL` + `LLM_API_KEY` + `LLM_MODEL` change per-provider.
//!
//! # Also supported
//!
//! - `LLM_PROVIDER` can be set to `"fake"` for the in-memory
//!   `FakeEmbedder` (tests + offline flows); defaults to `"openai_compat"`.
//! - Optional TOML overlay: pass a file path to `load_from_env_and_file`.

use std::sync::Arc;
use std::time::Duration;

use serde::{Deserialize, Serialize};

use crate::errors::EmbedError;
use crate::fake::FakeEmbedder;
use crate::openai_compat::{into_arc, OpenAICompatEmbedder, OpenAICompatOptions};
use crate::Embedder;

/// Top-level provider selector.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case", tag = "provider")]
pub enum Provider {
    /// Any OpenAI `/v1/embeddings`-compatible endpoint: SiliconFlow,
    /// OpenRouter, Groq, OpenAI, Together, etc.
    OpenAiCompat(OpenAICompatConfig),
    /// Deterministic in-memory embedder for tests and offline flows.
    Fake(FakeConfig),
}

/// OpenAI-compatible provider config. All fields have env equivalents
/// with the `LLM_` prefix — see module docs.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OpenAICompatConfig {
    /// Stable id used for logs/metrics. Defaults to
    /// `"openai-compat:<model>"` when unset.
    #[serde(default)]
    pub id: Option<String>,
    /// Remote model name, e.g. `"Qwen/Qwen3-Embedding-4B"`.
    pub model: String,
    /// Expected output dimension. Required — a wrong dim here means
    /// silently corrupt Lance columns on write, so we never default.
    pub dimension: u32,
    /// Base URL of the compatible endpoint, including `/v1` suffix.
    pub api_url: String,
    /// Bearer token.
    pub api_key: String,
    /// Optional passage prefix. See `effective_prefixes` for auto-default
    /// behavior with qwen* models.
    #[serde(default)]
    pub passage_prefix: Option<String>,
    /// Optional query prefix.
    #[serde(default)]
    pub query_prefix: Option<String>,
    #[serde(default = "default_max_input_tokens")]
    pub max_input_tokens: u32,
    #[serde(default = "default_request_timeout_secs")]
    pub request_timeout_secs: u64,
    #[serde(default = "default_concurrency")]
    pub concurrency: usize,
    #[serde(default = "default_batch_size")]
    pub batch_size: usize,
    #[serde(default = "default_max_retries")]
    pub max_retries: u32,
}

/// Fake embedder config. Used by tests and offline flows.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FakeConfig {
    #[serde(default = "default_fake_id")]
    pub id: String,
    pub dimension: u32,
    #[serde(default = "default_max_input_tokens")]
    pub max_input_tokens: u32,
}

/// Top-level wrapper. Keeps the provider-tagged enum flat under
/// `[embedder]`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EmbedderConfig {
    #[serde(flatten)]
    pub provider: Provider,
}

/// Compute effective prefixes. Qwen models get `"query: "` / `"passage: "`
/// auto-populated when the user did not set a value. Any explicit value
/// (including an empty string) wins.
fn effective_prefixes(
    model: &str,
    query: Option<String>,
    passage: Option<String>,
) -> (Option<String>, Option<String>) {
    let is_qwen = model.to_ascii_lowercase().contains("qwen");
    let q = match query {
        Some(s) => Some(s),
        None if is_qwen => Some("query: ".to_string()),
        None => None,
    };
    let p = match passage {
        Some(s) => Some(s),
        None if is_qwen => Some("passage: ".to_string()),
        None => None,
    };
    (q, p)
}

/// Build an `Arc<dyn Embedder>` from a typed config.
pub fn build(cfg: &EmbedderConfig) -> Result<Arc<dyn Embedder>, EmbedError> {
    match &cfg.provider {
        Provider::Fake(f) => Ok(Arc::new(FakeEmbedder::with_max_tokens(
            f.id.clone(),
            f.dimension,
            f.max_input_tokens,
        ))),
        Provider::OpenAiCompat(c) => {
            let id =
                c.id.clone()
                    .unwrap_or_else(|| format!("openai-compat:{}", c.model));
            let (query_prefix, passage_prefix) =
                effective_prefixes(&c.model, c.query_prefix.clone(), c.passage_prefix.clone());

            let opts = OpenAICompatOptions {
                id,
                model_name: c.model.clone(),
                dimension: c.dimension,
                max_input_tokens: c.max_input_tokens,
                api_url: c.api_url.clone(),
                api_key: c.api_key.clone(),
                passage_prefix,
                query_prefix,
                timeout: Duration::from_secs(c.request_timeout_secs),
                concurrency: c.concurrency,
                batch_size: c.batch_size,
                max_retries: c.max_retries,
            };
            Ok(into_arc(OpenAICompatEmbedder::new(opts)?))
        }
    }
}

/// Load an `EmbedderConfig` from env (prefix `LLM_`) optionally merged
/// with a TOML file. Env wins when both are present.
///
/// Required env vars for the openai-compat path: `LLM_API_URL`,
/// `LLM_API_KEY`, `LLM_MODEL`, `LLM_EMBEDDING_DIM`. Missing any one of
/// them produces `EmbedError::Config` with a clear message.
pub fn load_from_env_and_file(toml_path: Option<&str>) -> Result<EmbedderConfig, EmbedError> {
    use figment::providers::{Env, Format, Toml};
    use figment::Figment;

    let provider_env = std::env::var("LLM_PROVIDER")
        .unwrap_or_else(|_| "openai_compat".to_string())
        .to_ascii_lowercase();

    if provider_env == "fake" {
        // Fake path: only `LLM_EMBEDDING_DIM` required.
        let dim = require_env_u32("LLM_EMBEDDING_DIM")?;
        let id = std::env::var("LLM_ID").unwrap_or_else(|_| default_fake_id());
        let max_input_tokens = std::env::var("LLM_MAX_INPUT_TOKENS")
            .ok()
            .and_then(|s| s.parse().ok())
            .unwrap_or_else(default_max_input_tokens);
        return Ok(EmbedderConfig {
            provider: Provider::Fake(FakeConfig {
                id,
                dimension: dim,
                max_input_tokens,
            }),
        });
    }

    // openai_compat path — required env.
    let api_url = require_env("LLM_API_URL")?;
    let api_key = require_env("LLM_API_KEY")?;
    let model = require_env("LLM_MODEL")?;
    let dimension = require_env_u32("LLM_EMBEDDING_DIM")?;

    // Optional TOML for non-required knobs. Env overlays on top.
    let mut fig = Figment::new();
    if let Some(path) = toml_path {
        fig = fig.merge(Toml::file(path));
    }
    fig = fig.merge(Env::prefixed("LLM_").ignore(&[
        "PROVIDER",
        "API_URL",
        "API_KEY",
        "MODEL",
        "EMBEDDING_DIM",
    ]));
    #[derive(Deserialize, Default)]
    struct OptionalKnobs {
        id: Option<String>,
        query_prefix: Option<String>,
        passage_prefix: Option<String>,
        max_input_tokens: Option<u32>,
        request_timeout_secs: Option<u64>,
        concurrency: Option<usize>,
        batch_size: Option<usize>,
        max_retries: Option<u32>,
    }
    let knobs: OptionalKnobs = fig.extract().unwrap_or_default();

    Ok(EmbedderConfig {
        provider: Provider::OpenAiCompat(OpenAICompatConfig {
            id: knobs.id,
            model,
            dimension,
            api_url,
            api_key,
            passage_prefix: knobs.passage_prefix,
            query_prefix: knobs.query_prefix,
            max_input_tokens: knobs
                .max_input_tokens
                .unwrap_or_else(default_max_input_tokens),
            request_timeout_secs: knobs
                .request_timeout_secs
                .unwrap_or_else(default_request_timeout_secs),
            concurrency: knobs.concurrency.unwrap_or_else(default_concurrency),
            batch_size: knobs.batch_size.unwrap_or_else(default_batch_size),
            max_retries: knobs.max_retries.unwrap_or_else(default_max_retries),
        }),
    })
}

fn require_env(name: &str) -> Result<String, EmbedError> {
    std::env::var(name)
        .ok()
        .filter(|s| !s.trim().is_empty())
        .ok_or_else(|| EmbedError::Config(format!("{name} is required but unset or empty")))
}

fn require_env_u32(name: &str) -> Result<u32, EmbedError> {
    require_env(name)?
        .parse::<u32>()
        .map_err(|e| EmbedError::Config(format!("{name} must be a u32: {e}")))
}

fn default_max_input_tokens() -> u32 {
    8192
}
fn default_request_timeout_secs() -> u64 {
    30
}
fn default_concurrency() -> usize {
    4
}
fn default_batch_size() -> usize {
    32
}
fn default_max_retries() -> u32 {
    4
}
fn default_fake_id() -> String {
    "fake:embedder".into()
}
