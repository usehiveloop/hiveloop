//! `rag-engine-server` binary entrypoint.
//!
//! Responsibilities after Tranche 2G:
//!   * Load typed config via `figment` (env + optional TOML).
//!   * Initialise structured JSON tracing + OpenTelemetry span export
//!     (`telemetry::init`).
//!   * Start a Prometheus `/metrics` HTTP server on `metrics_addr`.
//!   * Bring up a tonic gRPC server on `listen_addr`:
//!       - `grpc.health.v1.Health` — unauthenticated.
//!       - `hiveloop.rag.v1.RagEngine` — gated by the shared-secret
//!         bearer-token interceptor, wrapped in the metrics/trace
//!         middleware.
//!   * Shut down both servers gracefully on SIGTERM / SIGINT and flush
//!     pending OTel spans before exit.

use std::process::ExitCode;
use std::sync::Arc;

use rag_engine_chunker::tokenizer::TiktokenTokenizer;
use rag_engine_chunker::{Chunker, ChunkerConfig};
use rag_engine_embed::{build as build_embedder, load_from_env_and_file as load_embedder_env};
use rag_engine_lance::{LanceStore, StoreConfig};
use rag_engine_proto::rag_engine_server::RagEngineServer;
use rag_engine_rerank::{build as build_reranker, RerankerConfig};
use rag_engine_server::auth::SharedSecretAuth;
use rag_engine_server::metrics::{spawn_metrics_server, Metrics};
use rag_engine_server::middleware::MetricsLayer;
use rag_engine_server::telemetry;
use rag_engine_server::{AppState, Config, RagEngineService, StateLimits, GRPC_SERVICE_NAME};
use tokio::signal;
use tonic::transport::Server;
use tracing::{error, info};

#[tokio::main]
async fn main() -> ExitCode {
    // Load config first so `log_level` + `otel_endpoint` influence the
    // subscriber and we fail fast on a missing shared secret.
    let config = match Config::load() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("rag-engine: failed to load config: {e}");
            return ExitCode::from(2);
        }
    };

    // `telemetry::init` installs the JSON log layer + (optional) OTLP
    // exporter. The returned guard MUST outlive `run`; its `Drop` is
    // what flushes pending spans.
    let _telemetry_guard = telemetry::init(
        &config.log_level,
        config.otel_endpoint.as_deref(),
        env!("CARGO_PKG_VERSION"),
    );

    info!(
        listen_addr = %config.listen_addr,
        metrics_addr = %config.metrics_addr,
        otel_enabled = config.otel_endpoint.is_some(),
        "rag-engine-server starting"
    );

    match run(config).await {
        Ok(()) => {
            info!("rag-engine-server shut down cleanly");
            ExitCode::SUCCESS
        }
        Err(e) => {
            error!(error = %e, "rag-engine-server terminated with error");
            ExitCode::FAILURE
        }
    }
}

async fn run(config: Config) -> anyhow::Result<()> {
    let grpc_addr = config.listen_addr.parse()?;
    let metrics_addr = config.metrics_addr.parse()?;

    // Process-wide metrics registry. The gRPC middleware records into
    // the same `Metrics` instance the HTTP `/metrics` handler reads
    // from; they share a `Registry` internally.
    let metrics = Metrics::global().clone();

    // Start /metrics HTTP server first. If its bind fails, the gRPC
    // listener never starts — ops noticing "pod is up but /metrics is
    // 404" is a worse failure mode than "pod never went ready".
    let metrics_handle = spawn_metrics_server(metrics_addr, metrics.clone()).await?;
    info!(addr = %metrics_handle.addr, "metrics server bound");

    // Health reporter: Phase 2A advertises SERVING immediately because
    // the service has no external dependencies yet. Tranche 2B will
    // flip to NOT_SERVING until LanceDB is reachable.
    let (mut health_reporter, health_service) = tonic_health::server::health_reporter();
    health_reporter
        .set_serving::<RagEngineServer<RagEngineService>>()
        .await;
    health_reporter
        .set_service_status(GRPC_SERVICE_NAME, tonic_health::ServingStatus::Serving)
        .await;

    // --- 2F: build application state ---------------------------------
    // Embedder: `LLM_PROVIDER=fake|openai_compat` (or `--embedder` flag,
    // tranche 2J-owned). `FakeEmbedder` is used for tests and offline
    // flows; the openai-compat path is the production path and requires
    // `LLM_API_URL` / `LLM_API_KEY` / `LLM_MODEL` / `LLM_EMBEDDING_DIM`.
    let embedder_cfg =
        load_embedder_env(None).map_err(|e| anyhow::anyhow!("embedder config: {e}"))?;
    let embedder =
        build_embedder(&embedder_cfg).map_err(|e| anyhow::anyhow!("embedder build: {e}"))?;

    // Reranker: `RERANKER_KIND=fake|siliconflow`. Only `siliconflow`
    // talks to the network.
    let rerank_cfg = RerankerConfig::from_toml_and_env(None)
        .map_err(|e| anyhow::anyhow!("reranker config: {e}"))?;
    let reranker =
        build_reranker(rerank_cfg).map_err(|e| anyhow::anyhow!("reranker build: {e}"))?;

    // Chunker: one process-wide instance. The ~9 MB BPE table loads once.
    let chunker = Arc::new(Chunker::new(
        TiktokenTokenizer::cl100k_base(),
        ChunkerConfig::default(),
    ));

    // LanceDB store: S3/MinIO via `LANCE_S3_URI` (required to use S3) or
    // local disk via `LANCE_URI` (default: `./.lancedb`).
    let store_cfg = load_store_config()?;
    let store = LanceStore::open(store_cfg)
        .await
        .map_err(|e| anyhow::anyhow!("lance store open: {e}"))?;

    let limits = StateLimits::from_env();
    let app_state = Arc::new(AppState::new(store, embedder, reranker, chunker, limits));

    // Shared-secret auth interceptor wraps the rag-engine service ONLY.
    // The health service stays unauthenticated so liveness probes work.
    let auth = SharedSecretAuth::new(config.shared_secret.as_str());
    let rag_service = RagEngineServer::with_interceptor(RagEngineService::new(app_state), auth);

    let shutdown = async {
        // Wait for SIGTERM (docker stop) or SIGINT (ctrl-c). On any
        // other platform we only wait for ctrl-c.
        #[cfg(unix)]
        {
            let mut sigterm = signal::unix::signal(signal::unix::SignalKind::terminate())
                .expect("install sigterm handler");
            tokio::select! {
                _ = signal::ctrl_c() => {}
                _ = sigterm.recv() => {}
            }
        }
        #[cfg(not(unix))]
        {
            let _ = signal::ctrl_c().await;
        }
        info!("shutdown signal received, draining");
    };

    let server_result = Server::builder()
        .layer(MetricsLayer::new(metrics))
        .add_service(health_service)
        .add_service(rag_service)
        .serve_with_shutdown(grpc_addr, shutdown)
        .await;

    // Metrics server drains alongside the gRPC server. We do this
    // after gRPC stops because scraping metrics during the final drain
    // is useful signal for operators watching shutdown.
    metrics_handle.shutdown().await;

    server_result.map_err(Into::into)
}

/// Load a `StoreConfig` from env. `LANCE_S3_URI` selects the S3 path;
/// `LANCE_URI` (default `./.lancedb`) selects local disk.
///
/// Env:
///   * `LANCE_S3_URI` — e.g. `s3://bucket/prefix`
///   * `LANCE_S3_REGION` — default `us-east-1`
///   * `LANCE_S3_ENDPOINT` — for MinIO/custom endpoints
///   * `LANCE_ACCESS_KEY_ID`, `LANCE_SECRET_ACCESS_KEY`
///   * `LANCE_S3_ALLOW_HTTP` — "true" for MinIO over HTTP
///   * `LANCE_URI` — local path or `memory://` (ignored if LANCE_S3_URI set)
fn load_store_config() -> anyhow::Result<StoreConfig> {
    if let Ok(s3_uri) = std::env::var("LANCE_S3_URI") {
        let region = std::env::var("LANCE_S3_REGION").unwrap_or_else(|_| "us-east-1".into());
        let endpoint = std::env::var("LANCE_S3_ENDPOINT").ok();
        let access_key_id = std::env::var("LANCE_ACCESS_KEY_ID").map_err(|_| {
            anyhow::anyhow!("LANCE_ACCESS_KEY_ID is required when LANCE_S3_URI is set")
        })?;
        let secret_access_key = std::env::var("LANCE_SECRET_ACCESS_KEY").map_err(|_| {
            anyhow::anyhow!("LANCE_SECRET_ACCESS_KEY is required when LANCE_S3_URI is set")
        })?;
        let allow_http = std::env::var("LANCE_S3_ALLOW_HTTP")
            .map(|s| s.trim().eq_ignore_ascii_case("true"))
            .unwrap_or(false);
        Ok(StoreConfig::S3 {
            uri: s3_uri,
            access_key_id,
            secret_access_key,
            endpoint,
            region,
            allow_http,
        })
    } else {
        let uri = std::env::var("LANCE_URI").unwrap_or_else(|_| "./.lancedb".into());
        Ok(StoreConfig::Local { uri })
    }
}
