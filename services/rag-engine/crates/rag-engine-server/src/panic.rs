//! Process-wide panic handler.
//!
//! Tranche 2H installs a custom `std::panic::set_hook` that:
//!
//!   1. Logs every panic via `tracing::error!` with the panic message,
//!      the source-file location, the thread name, and a captured
//!      backtrace when `RUST_BACKTRACE` is set. A structured log is
//!      strictly more useful than the default `write!(stderr, ...)` in a
//!      JSON-logged container.
//!
//!   2. Increments `rag_engine_panics_total`, a new counter registered
//!      on the same Prometheus registry the rest of the 2G metrics use.
//!      Operators can alert on this monotonically — a non-zero value
//!      outside of startup is always worth investigating.
//!
//! # What this hook does *not* do
//!
//! It does **not** abort the process. Tonic wraps every async handler
//! in `tokio::task::JoinHandle`; a panic inside a handler is caught by
//! the tokio runtime, converted to a `JoinError`, and surfaced to
//! tonic's top-level `make_service` as an `INTERNAL` gRPC status.
//! Letting the hook return means the rest of the process keeps running
//! — essential for a multi-tenant service that must survive a bad
//! request from one caller without dropping every other caller's RPCs.
//!
//! Unrecoverable panics that happen *outside* an async task (e.g. a
//! blocking thread we spawned, or the main thread itself before the
//! tokio runtime takes over) are caught by the hook too — we log them
//! loudly via `tracing::error!` and then let the default panic behavior
//! continue. The main binary's `tokio::main` wrapper will either
//! resume another task or, for a main-thread panic, exit non-zero. We
//! do not call `std::process::abort` — a loud log + non-zero exit is
//! sufficient, and an abort would prevent the OTel flush guard from
//! running.
//!
//! # Metric registration
//!
//! `install_panic_handler(metrics)` registers a single `IntCounter` on
//! the provided [`Metrics`] registry. A `OnceLock` guards first-time
//! registration so the hook can fire on multiple threads without a
//! race; subsequent calls are no-ops (idempotent).

use std::sync::OnceLock;

use parking_lot::Mutex;
use prometheus::{IntCounter, Opts};
use tracing::error;

use crate::metrics::Metrics;

/// Name of the panic counter exposed on `/metrics`.
pub const PANICS_METRIC_NAME: &str = "rag_engine_panics_total";

/// One-time slot for the panic counter. Initialised by
/// [`install_panic_handler`].
static PANIC_COUNTER: OnceLock<IntCounter> = OnceLock::new();

/// Guard against double-installing the hook. `set_hook` itself is
/// race-free (it takes a mutex internally), but we only want *our*
/// hook installed once per process — additional calls would just
/// replace the prior hook with an identical one, but a mutex here makes
/// that intent explicit.
static HOOK_INSTALLED: Mutex<bool> = Mutex::new(false);

/// Install the process-wide panic handler and register the
/// `rag_engine_panics_total` counter on the supplied metrics registry.
///
/// Call this exactly once from `main`, before any async work starts.
/// Safe to call multiple times — subsequent calls:
///   * do NOT re-install the hook (one process-wide hook),
///   * DO register the counter on the supplied registry if it isn't
///     there already. `IntCounter` is internally `Arc`-backed, so every
///     registry that holds the counter sees the same value.
pub fn install_panic_handler(metrics: &Metrics) {
    // Obtain or initialise the shared counter. The `OnceLock` gives us
    // a single process-wide `IntCounter` — all `Arc`-backed — so
    // increments from the panic hook are visible through *every*
    // registry we've registered a handle on.
    let counter = PANIC_COUNTER.get_or_init(|| {
        IntCounter::with_opts(Opts::new(
            PANICS_METRIC_NAME,
            "Total process-wide panics caught by the rag-engine panic hook.",
        ))
        .expect("build panic counter")
    });

    // Attempt to register a clone on the supplied registry. An
    // `AlreadyReg` error is the idempotent re-install path (same
    // registry, second call) and is silently ignored.
    if let Err(err) = metrics.registry().register(Box::new(counter.clone())) {
        match err {
            prometheus::Error::AlreadyReg => {}
            other => {
                eprintln!(
                    "rag-engine: panic counter registration failed on this registry: {other:?}"
                );
            }
        }
    }

    let mut installed = HOOK_INSTALLED.lock();
    if *installed {
        return;
    }
    *installed = true;
    drop(installed);

    std::panic::set_hook(Box::new(|info| {
        // Extract the panic payload as a printable string. Rust panics
        // carry an `Any` payload; `&str` and `String` are the common
        // concrete types when `panic!` or `assert!` fires.
        let payload = if let Some(s) = info.payload().downcast_ref::<&str>() {
            (*s).to_string()
        } else if let Some(s) = info.payload().downcast_ref::<String>() {
            s.clone()
        } else {
            "<non-string panic payload>".to_string()
        };

        let location = info
            .location()
            .map(|loc| format!("{}:{}:{}", loc.file(), loc.line(), loc.column()))
            .unwrap_or_else(|| "<unknown location>".to_string());

        let thread_name = std::thread::current()
            .name()
            .map(str::to_owned)
            .unwrap_or_else(|| format!("{:?}", std::thread::current().id()));

        // Captured backtrace. `RUST_BACKTRACE=1` is required for the
        // default backtrace provider to capture frames — a backtrace
        // here when the env is unset is typically a short "disabled"
        // placeholder, which we pass through unchanged.
        let backtrace = std::backtrace::Backtrace::capture().to_string();

        if let Some(counter) = PANIC_COUNTER.get() {
            counter.inc();
        }

        error!(
            panic.payload = %payload,
            panic.location = %location,
            panic.thread = %thread_name,
            panic.backtrace = %backtrace,
            "process panic caught by rag-engine hook"
        );

        // Also print to stderr. The JSON log layer goes through tracing
        // but a raw stderr line guarantees an operator tailing the pod
        // sees *something* even if the tracing subscriber has been torn
        // down (e.g. panic during shutdown).
        eprintln!("rag-engine PANIC @ {location} on thread {thread_name}: {payload}");
    }));
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn metric_name_matches_spec() {
        // The spec in the Tranche-2H brief fixes the metric name. Tests
        // that scrape /metrics match on this exact string, so breaking
        // it silently is exactly the kind of regression this test exists
        // to catch.
        assert_eq!(PANICS_METRIC_NAME, "rag_engine_panics_total");
    }
}
