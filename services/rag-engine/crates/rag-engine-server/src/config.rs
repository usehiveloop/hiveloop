//! Typed configuration for `rag-engine-server`.
//!
//! The loader merges (in increasing order of precedence):
//!   1. Hard-coded defaults (`Config::defaults`)
//!   2. Optional `rag-engine.toml` file (path via `RAG_ENGINE_CONFIG`)
//!   3. `RAG_ENGINE_*` environment variables
//!
//! # Required keys
//!
//! `shared_secret` has no default. If neither TOML nor env provides it,
//! `Config::load` returns an error — we refuse to boot with an empty
//! secret because a Rust service reachable from anywhere in the private
//! network without auth is a tenant-isolation hazard.

use figment::{
    providers::{Env, Format, Serialized, Toml},
    Figment,
};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use thiserror::Error;

/// Runtime configuration for the server.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Address the gRPC server binds to. Example: `0.0.0.0:50051`.
    pub listen_addr: String,

    /// Shared secret required on every non-health RPC via
    /// `authorization: Bearer <secret>` metadata. No default — must be
    /// supplied explicitly. Compared in constant time.
    pub shared_secret: String,

    /// Tracing / log level filter (e.g. `info`, `debug`, `warn`).
    pub log_level: String,

    /// Address the Prometheus `/metrics` HTTP server binds to. Always
    /// separate from `listen_addr` so a gRPC auth failure never hides
    /// the metrics endpoint and a scraper misconfiguration never loads
    /// the gRPC listener. Default: `0.0.0.0:9090`.
    ///
    /// Env: `RAG_ENGINE_METRICS_ADDR`.
    pub metrics_addr: String,

    /// Optional OTLP endpoint for OpenTelemetry trace export. When unset
    /// (the default), the process runs in logs-only mode — spans are
    /// still emitted to the JSON log layer but never shipped anywhere.
    /// When set, spans are batch-exported to the collector.
    ///
    /// Operators can also set `OTEL_EXPORTER_OTLP_ENDPOINT` directly;
    /// the OTLP SDK honours that env var natively. This field lets the
    /// same value flow from our TOML/env convention.
    ///
    /// Env: `RAG_ENGINE_OTEL_ENDPOINT`.
    #[serde(default)]
    pub otel_endpoint: Option<String>,
}

impl Config {
    fn defaults() -> DefaultsShape {
        DefaultsShape {
            listen_addr: "0.0.0.0:50051".to_string(),
            log_level: "info".to_string(),
            metrics_addr: "0.0.0.0:9090".to_string(),
        }
    }

    /// Load config from environment variables (and optional TOML file).
    ///
    /// Env var convention: `RAG_ENGINE_LISTEN_ADDR`,
    /// `RAG_ENGINE_SHARED_SECRET`, `RAG_ENGINE_LOG_LEVEL`,
    /// `RAG_ENGINE_METRICS_ADDR`, `RAG_ENGINE_OTEL_ENDPOINT`. A path to
    /// a TOML file may be supplied via `RAG_ENGINE_CONFIG`.
    pub fn load() -> Result<Self, ConfigError> {
        let mut fig = Figment::from(Serialized::defaults(Self::defaults()));

        if let Ok(path) = std::env::var("RAG_ENGINE_CONFIG") {
            let p = PathBuf::from(path);
            if p.exists() {
                fig = fig.merge(Toml::file(p));
            }
        }

        fig = fig.merge(Env::prefixed("RAG_ENGINE_"));

        let loaded: LoadedShape = fig
            .extract()
            .map_err(|e| ConfigError::Parse(e.to_string()))?;

        if loaded.shared_secret.trim().is_empty() {
            return Err(ConfigError::MissingSharedSecret);
        }

        // Normalise an empty string OTLP endpoint to `None` so operators
        // can explicitly unset the collector target via env without the
        // OTLP SDK interpreting "" as localhost.
        let otel_endpoint =
            loaded
                .otel_endpoint
                .and_then(|s| if s.trim().is_empty() { None } else { Some(s) });

        Ok(Config {
            listen_addr: loaded.listen_addr,
            shared_secret: loaded.shared_secret,
            log_level: loaded.log_level,
            metrics_addr: loaded.metrics_addr,
            otel_endpoint,
        })
    }
}

// `figment` serializes the defaults struct to merge them in. We keep a
// tiny shape with no `shared_secret` to guarantee the env / TOML MUST
// supply one.
#[derive(Serialize)]
struct DefaultsShape {
    listen_addr: String,
    log_level: String,
    metrics_addr: String,
}

// Internal shape used for extraction — lets us detect a missing
// `shared_secret` (empty string) and convert it to a typed error.
#[derive(Deserialize)]
struct LoadedShape {
    listen_addr: String,
    #[serde(default)]
    shared_secret: String,
    log_level: String,
    metrics_addr: String,
    #[serde(default)]
    otel_endpoint: Option<String>,
}

/// Typed errors from config loading. `Display` messages are deploy-clear
/// on purpose — operators read these in pod logs.
#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("RAG_ENGINE_SHARED_SECRET is required (got empty value); refusing to boot without a shared-secret auth token")]
    MissingSharedSecret,

    #[error("failed to parse rag-engine configuration: {0}")]
    Parse(String),
}
