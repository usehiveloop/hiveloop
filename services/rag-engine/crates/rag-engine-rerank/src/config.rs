//! Configuration + factory for `Reranker` implementations.
//!
//! Loaded via figment so callers can compose TOML + environment variable
//! overrides. The factory `build` returns a boxed `dyn Reranker` based on
//! the `kind` field, letting the rest of the engine be parametric over
//! implementation.

use std::sync::Arc;
use std::time::Duration;

use figment::{
    providers::{Env, Format, Toml},
    Figment,
};
use serde::{Deserialize, Serialize};

use crate::batching::MAX_CANDIDATES_PER_CALL;
use crate::fake::FakeReranker;
use crate::siliconflow::{
    SiliconFlowReranker, SiliconFlowRerankerConfig, DEFAULT_BASE_URL, DEFAULT_MAX_RETRIES,
    DEFAULT_MODEL,
};
use crate::{RerankError, Reranker};

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum RerankerKind {
    #[default]
    Fake,
    Siliconflow,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RerankerConfig {
    #[serde(default)]
    pub kind: RerankerKind,

    #[serde(default = "default_base_url")]
    pub base_url: String,

    #[serde(default = "default_model")]
    pub model: String,

    #[serde(default)]
    pub api_key: String,

    /// Upstream call timeout in seconds.
    #[serde(default = "default_timeout_secs")]
    pub timeout_secs: u64,

    #[serde(default = "default_max_retries")]
    pub max_retries: u32,

    /// Max candidates per upstream call. Defaults to
    /// `MAX_CANDIDATES_PER_CALL` when zero/unset.
    #[serde(default)]
    pub batch_size: usize,
}

fn default_base_url() -> String {
    DEFAULT_BASE_URL.to_string()
}
fn default_model() -> String {
    DEFAULT_MODEL.to_string()
}
fn default_timeout_secs() -> u64 {
    30
}
fn default_max_retries() -> u32 {
    DEFAULT_MAX_RETRIES
}

impl Default for RerankerConfig {
    fn default() -> Self {
        Self {
            kind: RerankerKind::default(),
            base_url: default_base_url(),
            model: default_model(),
            api_key: String::new(),
            timeout_secs: default_timeout_secs(),
            max_retries: default_max_retries(),
            batch_size: 0,
        }
    }
}

impl RerankerConfig {
    /// Build a `RerankerConfig` from a TOML file merged with environment
    /// variables. Env prefix: `RERANKER_` (e.g. `RERANKER_API_KEY`).
    pub fn from_toml_and_env(path: Option<&str>) -> Result<Self, Box<figment::Error>> {
        let mut fig = Figment::new();
        if let Some(p) = path {
            fig = fig.merge(Toml::file(p));
        }
        fig = fig.merge(Env::prefixed("RERANKER_"));
        fig.extract().map_err(Box::new)
    }
}

/// Construct a concrete `Reranker` from config.
///
/// Returned as `Arc<dyn Reranker>` so it can be shared across tasks +
/// stored on handler state.
pub fn build(cfg: RerankerConfig) -> Result<Arc<dyn Reranker>, RerankError> {
    match cfg.kind {
        RerankerKind::Fake => Ok(Arc::new(FakeReranker::new())),
        RerankerKind::Siliconflow => {
            let sf = SiliconFlowRerankerConfig {
                base_url: cfg.base_url,
                model: cfg.model,
                api_key: cfg.api_key,
                timeout: Duration::from_secs(cfg.timeout_secs),
                max_retries: cfg.max_retries,
                batch_size: if cfg.batch_size == 0 {
                    MAX_CANDIDATES_PER_CALL
                } else {
                    cfg.batch_size
                },
            };
            Ok(Arc::new(SiliconFlowReranker::new(sf)?))
        }
    }
}
