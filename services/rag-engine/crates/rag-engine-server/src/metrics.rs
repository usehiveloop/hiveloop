//! Prometheus metrics registry + named vectors used by `rag-engine`.
//!
//! Everything in this file is owned by Tranche 2G. Downstream tranches
//! (2F search, 2C ingest, etc.) increment these counters and histograms
//! from their own call sites via the exported helpers — never by
//! rebuilding them, never by reaching into `prometheus::default_registry`
//! directly. That keeps label names consistent and lets us add a test
//! assertion against any metric by string lookup.
//!
//! # Design notes
//!
//! * We use a `OnceLock<Metrics>` rather than `lazy_static!` because the
//!   metric names are fixed and we want a single construction site we
//!   can reach from tests. `Metrics::global()` is idempotent.
//!
//! * We use a process-local `Registry` (not `prometheus::default_registry`)
//!   because the default registry is a global that bleeds state across
//!   test binaries running in the same process. A dedicated registry
//!   means `gather()` always returns only our metrics and test
//!   assertions are hermetic.
//!
//! * Histogram buckets are chosen for the expected latency regime: gRPC
//!   calls in the 1 ms – 10 s range, embedding batches in the 10 ms –
//!   60 s range, Lance ops in the 1 ms – 30 s range. Anything faster
//!   than the lowest bucket is a noise floor; anything slower than the
//!   highest bucket is a timeout signal, not a tail-latency question.

use std::sync::OnceLock;

use prometheus::{
    Encoder, HistogramOpts, HistogramVec, IntCounterVec, IntGauge, IntGaugeVec, Opts, Registry,
    TextEncoder,
};

/// All named metrics the server exports. Every field is either a vector
/// or a plain metric — there is no `gather`-returns-nothing scenario.
#[derive(Clone)]
pub struct Metrics {
    registry: Registry,

    /// Total gRPC calls completed, labelled by `method` (e.g. `Search`,
    /// `IngestBatch`) and `code` (the gRPC status code string, e.g.
    /// `OK`, `UNIMPLEMENTED`, `INVALID_ARGUMENT`).
    pub rpc_total: IntCounterVec,

    /// Per-RPC duration histogram in seconds. One series per method.
    pub rpc_duration_seconds: HistogramVec,

    /// Inflight RPC gauge. Rises on RPC entry, falls on exit.
    /// Backpressure signal for load shedding.
    pub inflight_requests: IntGaugeVec,

    /// Documents ingested, by terminal status (`ok`, `error`,
    /// `duplicate`, etc.). Drives the ingest-error-rate SLO.
    pub ingest_docs_total: IntCounterVec,

    /// Size of each ingest batch the server accepted.
    pub ingest_batch_size: HistogramVec,

    /// End-to-end latency of one embedding batch call. Labelled later
    /// by tranche 2C when we have a model name.
    pub embedding_batch_latency_seconds: HistogramVec,

    /// Total tokens sent to the embedder, for billing reconciliation.
    pub embedding_tokens_total: IntCounterVec,

    /// Latency of a named Lance operation (`append`, `delete`,
    /// `compact`, `search`). Populated by tranche 2B.
    pub lance_operation_duration_seconds: HistogramVec,

    /// Current row count per dataset. A gauge so restarts don't lose
    /// history — tranche 2B sets this after any mutation.
    pub lance_dataset_rows: IntGaugeVec,

    /// Deliberate startup timestamp gauge — scraped once to assert
    /// "the process restarted at T". Makes it trivial to spot crash
    /// loops in a dashboard.
    pub process_start_time_seconds: IntGauge,
}

impl Metrics {
    /// Return the process-wide `Metrics` instance, initialising it on
    /// the first call. Safe to call from any thread, any time.
    pub fn global() -> &'static Metrics {
        static ONCE: OnceLock<Metrics> = OnceLock::new();
        ONCE.get_or_init(|| Metrics::new().expect("build metrics registry"))
    }

    /// Build a fresh metrics set registered in a fresh `Registry`. In
    /// production code you always want `global()`; this constructor is
    /// crate-public so tests can spin up an isolated registry when they
    /// explicitly need one.
    pub fn new() -> prometheus::Result<Self> {
        let registry = Registry::new();

        let rpc_total = IntCounterVec::new(
            Opts::new(
                "rag_engine_rpc_total",
                "Total gRPC calls completed, by method and status code.",
            ),
            &["method", "code"],
        )?;

        let rpc_duration_seconds = HistogramVec::new(
            HistogramOpts::new(
                "rag_engine_rpc_duration_seconds",
                "gRPC call duration in seconds, by method.",
            )
            .buckets(vec![
                0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
            ]),
            &["method"],
        )?;

        let inflight_requests = IntGaugeVec::new(
            Opts::new(
                "rag_engine_inflight_requests",
                "Currently in-flight gRPC calls, by method.",
            ),
            &["method"],
        )?;

        let ingest_docs_total = IntCounterVec::new(
            Opts::new(
                "rag_engine_ingest_docs_total",
                "Documents ingested, by terminal status.",
            ),
            &["status"],
        )?;

        let ingest_batch_size = HistogramVec::new(
            HistogramOpts::new(
                "rag_engine_ingest_batch_size",
                "Number of documents in each accepted ingest batch.",
            )
            .buckets(vec![
                1.0, 5.0, 10.0, 25.0, 50.0, 100.0, 250.0, 500.0, 1000.0,
            ]),
            &[],
        )?;

        let embedding_batch_latency_seconds = HistogramVec::new(
            HistogramOpts::new(
                "rag_engine_embedding_batch_latency_seconds",
                "End-to-end latency of one embedder batch call.",
            )
            .buckets(vec![
                0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0,
            ]),
            &["model"],
        )?;

        let embedding_tokens_total = IntCounterVec::new(
            Opts::new(
                "rag_engine_embedding_tokens_total",
                "Total tokens sent to the embedder, by model name.",
            ),
            &["model"],
        )?;

        let lance_operation_duration_seconds = HistogramVec::new(
            HistogramOpts::new(
                "rag_engine_lance_operation_duration_seconds",
                "Duration of a Lance operation in seconds, by operation name.",
            )
            .buckets(vec![
                0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0, 10.0, 30.0,
            ]),
            &["operation"],
        )?;

        let lance_dataset_rows = IntGaugeVec::new(
            Opts::new(
                "rag_engine_lance_dataset_rows",
                "Current row count per Lance dataset.",
            ),
            &["dataset_name"],
        )?;

        let process_start_time_seconds = IntGauge::with_opts(Opts::new(
            "rag_engine_process_start_time_seconds",
            "Unix timestamp (seconds) at which this process booted.",
        ))?;
        process_start_time_seconds.set(unix_time_now());

        registry.register(Box::new(rpc_total.clone()))?;
        registry.register(Box::new(rpc_duration_seconds.clone()))?;
        registry.register(Box::new(inflight_requests.clone()))?;
        registry.register(Box::new(ingest_docs_total.clone()))?;
        registry.register(Box::new(ingest_batch_size.clone()))?;
        registry.register(Box::new(embedding_batch_latency_seconds.clone()))?;
        registry.register(Box::new(embedding_tokens_total.clone()))?;
        registry.register(Box::new(lance_operation_duration_seconds.clone()))?;
        registry.register(Box::new(lance_dataset_rows.clone()))?;
        registry.register(Box::new(process_start_time_seconds.clone()))?;

        Ok(Metrics {
            registry,
            rpc_total,
            rpc_duration_seconds,
            inflight_requests,
            ingest_docs_total,
            ingest_batch_size,
            embedding_batch_latency_seconds,
            embedding_tokens_total,
            lance_operation_duration_seconds,
            lance_dataset_rows,
            process_start_time_seconds,
        })
    }

    /// Return the backing `Registry`. The HTTP handler in
    /// `serve_metrics` uses this to encode the text format; tests use
    /// it to gather and inspect named metrics directly.
    pub fn registry(&self) -> &Registry {
        &self.registry
    }

    /// Encode the current metric snapshot in Prometheus' text exposition
    /// format, returning the content-type to pair with it. The
    /// content-type string is part of the Prometheus scrape contract —
    /// scrapers that don't see a matching `Content-Type` will treat
    /// the payload as opaque and skip it.
    pub fn encode_text(&self) -> (String, String) {
        let encoder = TextEncoder::new();
        let families = self.registry.gather();
        let mut buf = Vec::new();
        // `encode` only fails if the writer fails; a `Vec<u8>` cannot.
        let _ = encoder.encode(&families, &mut buf);
        let body = String::from_utf8(buf).unwrap_or_default();
        (body, encoder.format_type().to_string())
    }
}

fn unix_time_now() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or_default()
}

// ---------------------------------------------------------------------------
// /metrics HTTP server
// ---------------------------------------------------------------------------

use std::net::SocketAddr;

use axum::{extract::State, http::HeaderValue, response::IntoResponse, routing::get, Router};
use tokio::net::TcpListener;
use tokio::sync::oneshot;

/// Run the Prometheus metrics HTTP server until the given shutdown
/// future resolves. Returns once the server socket has closed.
///
/// The handler returns the text exposition format at `GET /metrics`.
/// Any other path returns 404 — we deliberately do NOT mount `/health`
/// here; gRPC health is the single source of truth for liveness/readiness.
///
/// The caller owns the `Metrics` instance so tests can spin up an
/// isolated registry without touching the global.
pub async fn serve_metrics<F>(
    addr: SocketAddr,
    metrics: Metrics,
    shutdown: F,
) -> std::io::Result<()>
where
    F: std::future::Future<Output = ()> + Send + 'static,
{
    let app = Router::new()
        .route("/metrics", get(metrics_handler))
        .with_state(metrics);

    let listener = TcpListener::bind(addr).await?;
    tracing::info!(%addr, "metrics server listening");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown)
        .await
}

/// Bind the metrics server to `addr` synchronously (so the caller knows
/// the port is claimed before returning) and return a running `JoinHandle`
/// + a oneshot sender that triggers graceful shutdown.
///
/// Used both by `main.rs` and by the test harness. Keeping the bind
/// synchronous matters for tests: they need the port to be accepting
/// connections before they issue the first scrape.
pub async fn spawn_metrics_server(
    addr: SocketAddr,
    metrics: Metrics,
) -> std::io::Result<MetricsServerHandle> {
    let listener = TcpListener::bind(addr).await?;
    let bound_addr = listener.local_addr()?;

    let (tx, rx) = oneshot::channel::<()>();

    let app = Router::new()
        .route("/metrics", get(metrics_handler))
        .with_state(metrics);

    let handle = tokio::spawn(async move {
        let _ = axum::serve(listener, app)
            .with_graceful_shutdown(async move {
                let _ = rx.await;
            })
            .await;
    });

    Ok(MetricsServerHandle {
        addr: bound_addr,
        shutdown_tx: Some(tx),
        join: Some(handle),
    })
}

/// Handle returned by `spawn_metrics_server`. Dropping the handle
/// requests shutdown on a best-effort basis; callers that need to
/// confirm the server exited cleanly should call `shutdown().await`.
pub struct MetricsServerHandle {
    pub addr: SocketAddr,
    shutdown_tx: Option<oneshot::Sender<()>>,
    join: Option<tokio::task::JoinHandle<()>>,
}

impl MetricsServerHandle {
    /// Signal shutdown and wait for the server task to exit.
    pub async fn shutdown(mut self) {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        if let Some(join) = self.join.take() {
            let _ = join.await;
        }
    }
}

impl Drop for MetricsServerHandle {
    fn drop(&mut self) {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        if let Some(join) = self.join.take() {
            join.abort();
        }
    }
}

async fn metrics_handler(State(metrics): State<Metrics>) -> impl IntoResponse {
    let (body, content_type) = metrics.encode_text();
    let mut response = body.into_response();
    // The Prometheus scraper requires the exact content-type string the
    // `TextEncoder` emits (it carries a `version=0.0.4` param). Using
    // the encoder's reported format rather than a hardcoded string
    // insulates us from a future format bump.
    if let Ok(value) = HeaderValue::from_str(&content_type) {
        response.headers_mut().insert("content-type", value);
    }
    response
}
