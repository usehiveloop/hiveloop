//! `rag-engine-server` binary entrypoint.
//!
//! Responsibilities after Tranche 2H:
//!   * Install a process-wide panic handler (2H) before any async work.
//!   * Load typed config via `figment` (env + optional TOML).
//!   * Initialise structured JSON tracing + OpenTelemetry span export
//!     (`telemetry::init`).
//!   * Start a Prometheus `/metrics` HTTP server on `metrics_addr`.
//!   * Bring up a tonic gRPC server on `listen_addr`, wrapped in the
//!     2H backpressure stack (body-size limit → concurrency cap → per-RPC
//!     timeout → 2G metrics layer → 2A auth interceptor → handler).
//!   * Shut down both servers gracefully on SIGTERM / SIGINT (2H) and
//!     flush pending OTel spans before exit, with an explicit drain
//!     deadline.

use std::process::ExitCode;

use rag_engine_proto::rag_engine_server::RagEngineServer;
use rag_engine_server::auth::SharedSecretAuth;
use rag_engine_server::metrics::{spawn_metrics_server, Metrics};
use rag_engine_server::middleware::MetricsLayer;
use rag_engine_server::{
    body_size_limit_layer, concurrency_layer, grpc_catch_panic_layer, install_panic_handler,
    shutdown_signal, telemetry, timeout_layer, Config, LimitsConfig, RagEngineService,
    ShutdownConfig, GRPC_SERVICE_NAME,
};
use tokio::sync::oneshot;
use tonic::transport::Server;
use tower::ServiceBuilder;
use tracing::{error, info, warn};

#[tokio::main]
async fn main() -> ExitCode {
    // lifecycle (2H): panic handler — register as early as we can, but
    // AFTER `Metrics::global()` has allocated the process-wide registry
    // (we need a registry to register the panic counter on). The global
    // is lazy-initialised by `Metrics::global()`, so calling that before
    // `install_panic_handler` gives us the invariant we want: any panic
    // from this point on is counted + logged.
    install_panic_handler(Metrics::global());

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

    // lifecycle (2H): resolve backpressure + drain limits from env.
    let limits = LimitsConfig::from_env();
    let shutdown_cfg = ShutdownConfig::from_env();
    info!(
        max_concurrent = limits.max_concurrent,
        rpc_timeout_secs = limits.rpc_timeout.as_secs_f64(),
        max_request_bytes = limits.max_request_bytes,
        drain_deadline_secs = shutdown_cfg.drain_deadline.as_secs_f64(),
        "lifecycle limits resolved"
    );

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
    //
    // NOTE to the 2F finalizer: `RagEngineService::new()` is the single
    // seam where 2F's stateful constructor (`RagEngineService::new(state)`)
    // plugs in. Everything else in this function is 2H territory.
    let auth = SharedSecretAuth::new(config.shared_secret.as_str());
    // lifecycle (2H): tonic's built-in `max_decoding_message_size`
    // catches oversize payloads that slip past the content-length
    // check in `body_size_limit_layer` — HTTP/2 streaming requests
    // without a content-length header arrive here, and the protobuf
    // decoder raises `RESOURCE_EXHAUSTED` when a single message
    // exceeds the configured limit. Apply the limit on the base
    // `RagEngineServer` *before* wrapping in the auth interceptor —
    // the generated wrapper (`RagEngineServer`) owns the method,
    // `InterceptedService` does not expose it.
    let rag_service_inner = RagEngineServer::new(RagEngineService::new())
        .max_decoding_message_size(limits.max_request_bytes);
    let rag_service = tonic::service::interceptor::InterceptedService::new(rag_service_inner, auth);

    // lifecycle (2H): middleware stack.
    //
    // Order (outermost → innermost) is deliberate:
    //   1. metrics layer (2G) — OUTERMOST so every request and every
    //      rejection (RESOURCE_EXHAUSTED / DEADLINE_EXCEEDED / INTERNAL)
    //      is counted with the correct code label. Ops dashboards would
    //      otherwise go silent during a saturation event — exactly when
    //      we need them loudest.
    //   2. body-size limit — reject oversize requests *before* we spend
    //      protobuf-decode cycles on them or take a concurrency permit.
    //   3. concurrency cap — fail-fast when the server is saturated.
    //   4. per-RPC timeout — cap the wall-clock any single handler can
    //      burn. Client `grpc-timeout` headers still win when shorter.
    //   5. panic catcher — INNERMOST, closest to the handler so the
    //      layers above observe the handler's panic-as-`INTERNAL`
    //      response the same way they would observe any other error
    //      status. The metric label will be `INTERNAL`, matching
    //      client-facing behaviour.
    let stack = ServiceBuilder::new()
        .layer(MetricsLayer::new(metrics.clone()))
        .layer(body_size_limit_layer(limits.max_request_bytes))
        .layer(concurrency_layer(limits.max_concurrent))
        .layer(timeout_layer(limits.rpc_timeout))
        .layer(grpc_catch_panic_layer());

    // lifecycle (2H): graceful shutdown + drain deadline.
    //
    // The shutdown signal is split via a oneshot so we can observe
    // "signal arrived" independently of tonic's own drain. Once the
    // signal fires, we start the drain timer; if tonic hasn't finished
    // by `drain_deadline`, we force-abort the server task.
    let (signal_tx, signal_rx) = oneshot::channel::<()>();
    let shutdown_future = async move {
        shutdown_signal().await;
        let _ = signal_tx.send(());
    };

    let mut grpc_task = tokio::spawn(async move {
        Server::builder()
            .layer(stack)
            .add_service(health_service)
            .add_service(rag_service)
            .serve_with_shutdown(grpc_addr, shutdown_future)
            .await
    });

    let drain_deadline = shutdown_cfg.drain_deadline;

    // Wait for either:
    //   (a) the server task to exit on its own (bind error / panic), or
    //   (b) the shutdown signal to fire — at which point we start the
    //       drain timer and race tonic's drain against `drain_deadline`.
    let final_status: Result<(), anyhow::Error> = tokio::select! {
        biased;

        server_res = &mut grpc_task => {
            match server_res {
                Ok(inner) => inner.map_err(Into::into),
                Err(join_err) => Err(anyhow::anyhow!("gRPC server task panicked: {join_err}")),
            }
        }

        _ = signal_rx => {
            info!(
                deadline_secs = drain_deadline.as_secs_f64(),
                "shutdown signal observed; starting drain window"
            );
            match tokio::time::timeout(drain_deadline, &mut grpc_task).await {
                Ok(Ok(server_res)) => {
                    info!("gRPC server drained within deadline");
                    server_res.map_err(Into::into)
                }
                Ok(Err(join_err)) => {
                    warn!(error = %join_err, "gRPC server task panicked during drain");
                    Err(anyhow::anyhow!("gRPC server task panicked: {join_err}"))
                }
                Err(_elapsed) => {
                    warn!(
                        deadline_secs = drain_deadline.as_secs_f64(),
                        "drain deadline exceeded; aborting in-flight RPCs"
                    );
                    grpc_task.abort();
                    Err(anyhow::anyhow!(
                        "drain deadline of {:?} exceeded; forced exit",
                        drain_deadline
                    ))
                }
            }
        }
    };

    // Metrics server drains alongside the gRPC server. We do this
    // after gRPC stops because scraping metrics during the final drain
    // is useful signal for operators watching shutdown.
    metrics_handle.shutdown().await;

    final_status
}
