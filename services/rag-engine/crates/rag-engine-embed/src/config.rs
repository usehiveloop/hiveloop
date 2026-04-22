//! Figment-driven configuration for the embedder factory.
//!
//! Parallels the pattern in `rag-engine-server/src/config.rs` but is kept
//! local to the embed crate: callers compose this into a wider config
//! with figment's `.merge(...)` if they want.
//!
//! # Shape
//!
//! ```toml
//! [embedder]
//! provider = "siliconflow"
//! id = "siliconflow:qwen3-embedding-4b"
//! model_name = "Qwen/Qwen3-Embedding-4B"
//! dimension = 2560
//! max_input_tokens = 8192
//! base_url = "https://api.siliconflow.cn/v1"
//! api_key_env = "SILICONFLOW_API_KEY"
//! passage_prefix = "passage: "
//! query_prefix = "query: "
//! timeout_secs = 10
//! concurrency = 4
//! batch_size = 32
//! max_retries = 4
//!
//! # alternative: fake
//! # [embedder]
//! # provider = "fake"
//! # id = "fake:test"
//! # dimension = 128
//! ```
//!
//! # Env overlay
//!
//! - `SILICONFLOW_API_KEY` — bearer token
//! - `SILICONFLOW_BASE_URL` — overrides the `base_url` field
//!
//! These are pulled directly by `build()` to keep secret handling narrow
//! (the config struct doesn't serialize the key itself).

use std::sync::Arc;
use std::time::Duration;

use serde::{Deserialize, Serialize};

use crate::errors::EmbedError;
use crate::fake::FakeEmbedder;
use crate::siliconflow::{SiliconFlowEmbedder, SiliconFlowOptions};
use crate::Embedder;

/// Top-level provider selector.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "lowercase", tag = "provider")]
pub enum Provider {
    SiliconFlow(SiliconFlowConfig),
    Fake(FakeConfig),
}

/// SiliconFlow (or any OpenAI-compatible) config.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SiliconFlowConfig {
    pub id: String,
    pub model_name: String,
    pub dimension: u32,
    #[serde(default = "default_max_input_tokens")]
    pub max_input_tokens: u32,
    #[serde(default = "default_base_url")]
    pub base_url: String,
    /// Name of the env var that carries the API key. Defaults to
    /// `SILICONFLOW_API_KEY`.
    #[serde(default = "default_api_key_env")]
    pub api_key_env: String,
    #[serde(default)]
    pub passage_prefix: Option<String>,
    #[serde(default)]
    pub query_prefix: Option<String>,
    #[serde(default = "default_timeout_secs")]
    pub timeout_secs: u64,
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

/// Top-level wrapper. Lets the TOML put `[embedder]` at the top and carry
/// the provider enum as a tagged internal struct.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EmbedderConfig {
    #[serde(flatten)]
    pub provider: Provider,
}

/// Factory: build an `Arc<dyn Embedder>` from a typed config. Reads
/// secret env vars at build time; returns `EmbedError::Config` if a
/// required secret is missing.
pub fn build(cfg: &EmbedderConfig) -> Result<Arc<dyn Embedder>, EmbedError> {
    match &cfg.provider {
        Provider::Fake(f) => Ok(Arc::new(FakeEmbedder::with_max_tokens(
            f.id.clone(),
            f.dimension,
            f.max_input_tokens,
        ))),
        Provider::SiliconFlow(sf) => {
            let api_key = std::env::var(&sf.api_key_env).map_err(|_| {
                EmbedError::Config(format!(
                    "env var {} is required for siliconflow embedder but is unset",
                    sf.api_key_env
                ))
            })?;

            // SILICONFLOW_BASE_URL as an override convention, matching
            // the OpenAI SDK's OPENAI_BASE_URL pattern.
            let base_url = std::env::var("SILICONFLOW_BASE_URL")
                .ok()
                .filter(|s| !s.is_empty())
                .unwrap_or_else(|| sf.base_url.clone());

            let opts = SiliconFlowOptions {
                id: sf.id.clone(),
                model_name: sf.model_name.clone(),
                dimension: sf.dimension,
                max_input_tokens: sf.max_input_tokens,
                base_url,
                api_key,
                passage_prefix: sf.passage_prefix.clone(),
                query_prefix: sf.query_prefix.clone(),
                timeout: Duration::from_secs(sf.timeout_secs),
                concurrency: sf.concurrency,
                batch_size: sf.batch_size,
                max_retries: sf.max_retries,
            };
            Ok(Arc::new(SiliconFlowEmbedder::new(opts)?))
        }
    }
}

fn default_max_input_tokens() -> u32 {
    8192
}
fn default_base_url() -> String {
    "https://api.siliconflow.cn/v1".into()
}
fn default_api_key_env() -> String {
    "SILICONFLOW_API_KEY".into()
}
fn default_timeout_secs() -> u64 {
    10
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
