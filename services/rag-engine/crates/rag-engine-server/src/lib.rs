//! `rag-engine-server` library surface.
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
pub mod lifecycle;
pub mod metrics;
pub mod middleware;
pub mod panic;
pub mod service;
pub mod telemetry;

pub use config::{Config, ConfigError};
pub use lifecycle::{
    body_size_limit_layer, concurrency_layer, grpc_catch_panic_layer, shutdown_signal,
    timeout_layer, BodyLimitLayer, ConcurrencyLayer, LimitsConfig, ShutdownConfig, TimeoutLayer,
};
pub use metrics::{Metrics, MetricsServerHandle};
pub use middleware::MetricsLayer;
pub use panic::install_panic_handler;
pub use service::RagEngineService;
pub use telemetry::TelemetryGuard;

/// Fully-qualified gRPC service name the health reporter advertises.
/// Must match the `package.service` name emitted by `tonic-build`.
pub const GRPC_SERVICE_NAME: &str = "hiveloop.rag.v1.RagEngine";
