//! Tracing + OpenTelemetry wiring for `rag-engine`.
//!
//! This module supersedes the tiny `init_tracing` function that lived
//! in `main.rs` during Tranche 2A. The responsibilities are:
//!
//!   * JSON logs to stdout (as before).
//!   * `tracing_opentelemetry::layer()` bridged to an OTLP span exporter
//!     when `cfg.otel_endpoint` is non-empty.
//!   * A process-wide `TextMapPropagator` (W3C Trace Context) so the
//!     tonic middleware can read `traceparent` off inbound requests
//!     and chain Go → Rust spans.
//!   * A `TelemetryGuard` whose `Drop` calls
//!     `TracerProvider::shutdown()` synchronously so the last batch of
//!     spans flushes on process exit instead of vanishing.
//!
//! # OTLP endpoint precedence
//!
//! The OTLP SDK natively reads `OTEL_EXPORTER_OTLP_ENDPOINT`. We also
//! let operators set `RAG_ENGINE_OTEL_ENDPOINT` (which is what
//! `Config::otel_endpoint` holds). If the config field is set, we pass
//! it explicitly to the exporter; otherwise the SDK picks up the
//! env var on its own. If neither is set, we skip the exporter entirely
//! — "unset" is the signal that tracing should be logs-only.
//!
//! # Shutdown ordering
//!
//! On drop we:
//!   1. Call `provider.shutdown()` which flushes the BatchSpanProcessor.
//!   2. Drop the global tracer provider (replacing it with a NoopProvider).
//!
//! This must run on a thread that can block briefly; when the Tokio
//! runtime is still alive we still use a sync shutdown call because
//! the batch processor's shutdown is sync-blocking anyway.

use opentelemetry::global;
use opentelemetry::trace::TracerProvider as _;
use opentelemetry::KeyValue;
use opentelemetry_otlp::{SpanExporter, WithExportConfig};
use opentelemetry_sdk::propagation::TraceContextPropagator;
use opentelemetry_sdk::trace::TracerProvider;
use opentelemetry_sdk::{runtime, Resource};
use tracing_subscriber::layer::SubscriberExt;
use tracing_subscriber::util::SubscriberInitExt;
use tracing_subscriber::{filter::EnvFilter, fmt};

/// Service name reported on every exported span. Keep in sync with the
/// Go-side registration under `internal/observability/`.
pub const SERVICE_NAME: &str = "rag-engine";

/// Holds the state that must outlive the tracing subscriber so spans
/// can still be flushed at shutdown. Drop this last (store it in
/// `main`'s local scope).
pub struct TelemetryGuard {
    // `Option` so `Drop` can take ownership and call `shutdown`.
    provider: Option<TracerProvider>,
}

impl TelemetryGuard {
    /// Force a synchronous flush of any pending spans. Useful in
    /// integration tests that need to assert on emitted spans without
    /// waiting for the guard to drop.
    pub fn force_flush(&self) {
        if let Some(provider) = &self.provider {
            for result in provider.force_flush() {
                if let Err(err) = result {
                    tracing::warn!(?err, "otel force_flush failed");
                }
            }
        }
    }
}

impl Drop for TelemetryGuard {
    fn drop(&mut self) {
        // Flush + shut down the batch exporter. `provider.shutdown()`
        // is synchronous, but the Tokio-runtime BatchSpanProcessor it
        // drives needs an async context to drain its queue. If Drop
        // runs on the multi-threaded Tokio runtime (the common case —
        // `main` finishing inside `#[tokio::main]`), a direct call is
        // fine. If it runs *after* the runtime has already shut down
        // (or on a non-Tokio thread), the shutdown can block waiting
        // for a background export that will never land.
        //
        // We guard against the second case with a short hard-timeout
        // enforced via a separate OS thread. The worst case is we drop
        // the last in-flight batch on a crash-path shutdown — which is
        // a deliberate tradeoff: unhanging beats last-second spans.
        if let Some(provider) = self.provider.take() {
            let done = std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false));
            let done_clone = done.clone();
            let join = std::thread::Builder::new()
                .name("otel-shutdown".to_string())
                .spawn(move || {
                    if let Err(err) = provider.shutdown() {
                        eprintln!("rag-engine: otel tracer provider shutdown failed: {err:?}");
                    }
                    done_clone.store(true, std::sync::atomic::Ordering::SeqCst);
                });

            if let Ok(handle) = join {
                // Wait up to 3s — more than enough for a healthy
                // collector roundtrip, short enough to never block a
                // SIGTERM-triggered exit visibly.
                let deadline = std::time::Instant::now() + std::time::Duration::from_secs(3);
                while std::time::Instant::now() < deadline
                    && !done.load(std::sync::atomic::Ordering::SeqCst)
                {
                    std::thread::sleep(std::time::Duration::from_millis(25));
                }
                if done.load(std::sync::atomic::Ordering::SeqCst) {
                    let _ = handle.join();
                } else {
                    // The shutdown thread is still running; leak the
                    // handle so it can finish in the background if the
                    // process hangs around, but don't block exit on it.
                    eprintln!(
                        "rag-engine: otel shutdown timed out after 3s, continuing process exit"
                    );
                }
            }
        }
    }
}

/// Initialise tracing + OpenTelemetry.
///
/// * `log_level` — default filter if neither `RUST_LOG` nor the OTEL
///   spec-level env var is set.
/// * `otel_endpoint` — OTLP collector endpoint (gRPC). `None` = no
///   exporter, logs-only.
/// * `service_version` — value for the `service.version` resource
///   attribute. Typically `env!("CARGO_PKG_VERSION")`.
///
/// Safe to call multiple times: subsequent calls become no-ops on the
/// subscriber registration (via `try_init`), but each call *does*
/// build its own tracer provider — so tests should scope the returned
/// guard to the test's lifetime and not leak it.
pub fn init(log_level: &str, otel_endpoint: Option<&str>, service_version: &str) -> TelemetryGuard {
    // Install the W3C propagator regardless of whether we're actually
    // exporting spans — the middleware uses it to *extract* incoming
    // trace context from Go callers so that "logs-only" mode still
    // carries the trace id through the structured logs.
    global::set_text_map_propagator(TraceContextPropagator::new());

    let filter = EnvFilter::try_from_default_env()
        .or_else(|_| EnvFilter::try_new(log_level))
        .unwrap_or_else(|_| EnvFilter::new("info"));

    let json_layer = fmt::layer()
        .json()
        .with_current_span(true)
        .with_span_list(false);

    // Build the OTLP tracer provider first (so we can reuse it in the
    // guard), then attach a `tracing_opentelemetry` layer in-line — doing
    // it in-line avoids the `Send + Sync` bounds that boxing a generic
    // `Layer<S>` would otherwise impose on the registry subscriber.
    let provider = build_provider(otel_endpoint, service_version);

    match &provider {
        Some(p) => {
            let tracer = p.tracer(SERVICE_NAME);
            let otel_layer = tracing_opentelemetry::layer().with_tracer(tracer);
            let _ = tracing_subscriber::registry()
                .with(filter)
                .with(json_layer)
                .with(otel_layer)
                .try_init();
        }
        None => {
            let _ = tracing_subscriber::registry()
                .with(filter)
                .with(json_layer)
                .try_init();
        }
    }

    TelemetryGuard { provider }
}

/// Build the OTLP span-exporting `TracerProvider`. Returns `None` when
/// no endpoint is configured (logs-only mode) or when the exporter
/// could not be built (malformed URL, missing TLS material, etc.) —
/// in either case the caller falls back to the log-only subscriber.
fn build_provider(otel_endpoint: Option<&str>, service_version: &str) -> Option<TracerProvider> {
    let endpoint = otel_endpoint?;

    // Attempt to construct the exporter. If the collector is
    // unreachable at boot, the batch exporter still initialises cleanly
    // — it'll just drop spans until the collector comes up. We only
    // fail if the URL is malformed.
    let exporter = match SpanExporter::builder()
        .with_tonic()
        .with_endpoint(endpoint)
        .build()
    {
        Ok(e) => e,
        Err(err) => {
            eprintln!(
                "rag-engine: failed to build OTLP exporter for {endpoint}: {err:?}; continuing without export"
            );
            return None;
        }
    };

    // Build a unique per-process instance id so dashboards can
    // distinguish replicas. The `service.instance.id` attribute is the
    // canonical OTel convention for this.
    let instance_id = ulid::Ulid::new().to_string();

    let resource = Resource::new(vec![
        KeyValue::new("service.name", SERVICE_NAME),
        KeyValue::new("service.version", service_version.to_string()),
        KeyValue::new("service.instance.id", instance_id),
    ]);

    #[allow(deprecated)] // `with_config` is deprecated in 0.27 but
    // no `with_resource` exists on `Builder` in that version — the
    // non-deprecated API is a 0.28+ change. Revisit when we bump.
    let provider = TracerProvider::builder()
        .with_config(opentelemetry_sdk::trace::Config::default().with_resource(resource))
        .with_batch_exporter(exporter, runtime::Tokio)
        .build();

    // Install provider globally so `tracing::info_span!` → OTel works
    // from anywhere, including modules that don't hold a `Tracer`.
    global::set_tracer_provider(provider.clone());

    Some(provider)
}
