//! `rag-engine-server` library surface.
//!
//! # Crate-level clippy allows
//!
//! * `result_large_err` — tonic's `Status` type is ~176 bytes and every
//!   `Result<T, Status>` in handler/helper signatures trips this lint.
//!   Boxing it would add an allocation on every return path and buy
//!   nothing; we opt out crate-wide per the precedent set in 2A
//!   (`DECISIONS.md` — "Deviations from the tranche brief").

#![allow(clippy::result_large_err)]
//!
//! The binary in `main.rs` is a thin wrapper around the runtime exposed
//! here. Exposing the runtime as a library (rather than living entirely
//! in `main.rs`) is what lets the integration tests under `tests/` spawn
//! a real server on a random port and exercise it over real gRPC — with
//! zero in-process trickery.
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2A for the original
//! scaffolding spec; §Tranche 2G adds `telemetry`, `metrics`, and
//! `middleware`.

pub mod auth;
pub mod config;
pub mod error;
pub mod idempotency;
pub mod metrics;
pub mod middleware;
pub mod service;
pub mod state;
pub mod telemetry;

pub use config::{Config, ConfigError};
pub use metrics::{Metrics, MetricsServerHandle};
pub use middleware::MetricsLayer;
pub use service::RagEngineService;
pub use state::{AppState, IdempotencyCaches, StateLimits};
pub use telemetry::TelemetryGuard;

/// Fully-qualified gRPC service name the health reporter advertises.
/// Must match the `package.service` name emitted by `tonic-build`.
pub const GRPC_SERVICE_NAME: &str = "hiveloop.rag.v1.RagEngine";
