//! Lifecycle concerns for the `rag-engine` binary — graceful shutdown,
//! backpressure (concurrency cap + per-RPC timeout + request-body size
//! limit), and the panic handler.
//!
//! Tranche 2H owns this module. Each submodule is independent and
//! individually testable; `main.rs` wires them together into the final
//! tonic `Server::builder()` stack.

pub mod backpressure;
pub mod catch_panic;
pub mod shutdown;

pub use backpressure::{
    body_size_limit_layer, concurrency_layer, timeout_layer, BodyLimitLayer, ConcurrencyLayer,
    LimitsConfig, TimeoutLayer,
};
pub use catch_panic::grpc_catch_panic_layer;
pub use shutdown::{shutdown_signal, ShutdownConfig};
