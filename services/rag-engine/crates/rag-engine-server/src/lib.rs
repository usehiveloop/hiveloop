//! `rag-engine-server` library surface.
//!
//! The binary in `main.rs` is a thin wrapper around the runtime exposed
//! here. Exposing the runtime as a library (rather than living entirely
//! in `main.rs`) is what lets the integration tests under `tests/` spawn
//! a real server on a random port and exercise it over real gRPC — with
//! zero in-process trickery.
//!
//! See `plans/onyx-port-phase2.md` §Tranche 2A for the full scaffolding
//! spec.

pub mod auth;
pub mod config;
pub mod service;

pub use config::{Config, ConfigError};
pub use service::RagEngineService;

/// Fully-qualified gRPC service name the health reporter advertises.
/// Must match the `package.service` name emitted by `tonic-build`.
pub const GRPC_SERVICE_NAME: &str = "hiveloop.rag.v1.RagEngine";
