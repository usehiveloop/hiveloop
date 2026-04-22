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

use rag_engine_proto::rag_engine_server::RagEngineServer;
use rag_engine_server::auth::SharedSecretAuth;
use rag_engine_server::metrics::{spawn_metrics_server, Metrics};
use rag_engine_server::middleware::MetricsLayer;
use rag_engine_server::telemetry;
use rag_engine_server::{Config, RagEngineService, GRPC_SERVICE_NAME};
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

    // Shared-secret auth interceptor wraps the rag-engine service ONLY.
    // The health service stays unauthenticated so liveness probes work.
    let auth = SharedSecretAuth::new(config.shared_secret.as_str());
    let rag_service = RagEngineServer::with_interceptor(RagEngineService::new(), auth);

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
