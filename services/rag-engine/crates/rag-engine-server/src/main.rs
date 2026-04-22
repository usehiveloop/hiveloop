//! `rag-engine-server` binary entrypoint.
//!
//! Responsibilities in Tranche 2A:
//!   * Load typed config via `figment` (env + optional TOML).
//!   * Initialize structured JSON tracing (respects `log_level`).
//!   * Bring up a tonic gRPC server on `listen_addr`:
//!       - `grpc.health.v1.Health` — unauthenticated.
//!       - `hiveloop.rag.v1.RagEngine` — gated by the shared-secret
//!         bearer-token interceptor, all RPCs return `UNIMPLEMENTED`.
//!   * Shut down gracefully on SIGTERM / SIGINT.

use std::process::ExitCode;

use rag_engine_proto::rag_engine_server::RagEngineServer;
use rag_engine_server::auth::SharedSecretAuth;
use rag_engine_server::{Config, RagEngineService, GRPC_SERVICE_NAME};
use tokio::signal;
use tonic::transport::Server;
use tracing::{error, info};

#[tokio::main]
async fn main() -> ExitCode {
    // We load config first so `log_level` can influence the subscriber.
    let config = match Config::load() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("rag-engine: failed to load config: {e}");
            return ExitCode::from(2);
        }
    };

    init_tracing(&config.log_level);

    info!(
        listen_addr = %config.listen_addr,
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

fn init_tracing(level: &str) {
    use tracing_subscriber::{filter::EnvFilter, fmt, prelude::*};

    // Build the filter: respect `RUST_LOG` if set, otherwise fall back
    // to the config-supplied level.
    let filter = EnvFilter::try_from_default_env()
        .or_else(|_| EnvFilter::try_new(level))
        .unwrap_or_else(|_| EnvFilter::new("info"));

    // JSON formatter → stdout, structured.
    let json_layer = fmt::layer()
        .json()
        .with_current_span(true)
        .with_span_list(false);

    // `try_init` so repeated initialisation (tests, re-entry) is a
    // no-op instead of a panic.
    let _ = tracing_subscriber::registry()
        .with(filter)
        .with(json_layer)
        .try_init();
}

async fn run(config: Config) -> anyhow::Result<()> {
    let addr = config.listen_addr.parse()?;

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

    Server::builder()
        .add_service(health_service)
        .add_service(rag_service)
        .serve_with_shutdown(addr, shutdown)
        .await?;

    Ok(())
}
