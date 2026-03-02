use anyhow::Context;
use bridge_core::RuntimeConfig;
use figment::providers::{Env, Format, Serialized, Toml};
use figment::Figment;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio_util::sync::CancellationToken;
use tokio_util::task::TaskTracker;
use tracing::{error, info};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Load configuration from config.toml and environment variables
    let config: RuntimeConfig = Figment::from(Serialized::defaults(RuntimeConfig::default()))
        .merge(Toml::file("config.toml"))
        .merge(Env::prefixed("BRIDGE_"))
        .extract()
        .context("failed to load configuration")?;

    // Initialize logging
    init_logging(&config);

    info!("bridge starting");

    // Create global lifecycle primitives
    let cancel = CancellationToken::new();
    let tracker = TaskTracker::new();

    // Create shared services
    let mcp_manager = Arc::new(McpManager::new());
    let supervisor = Arc::new(AgentSupervisor::new(mcp_manager.clone(), cancel.clone()));

    // Fetch initial agents from control plane
    let client = reqwest::Client::new();
    match sync::poller::fetch_agents(&client, &config).await {
        Ok(agents) => {
            info!(count = agents.len(), "fetched initial agents");
            if let Err(e) = supervisor.load_agents(agents).await {
                error!(error = %e, "failed to load initial agents");
            }
        }
        Err(e) => {
            error!(error = %e, "failed to fetch initial agents from control plane");
            return Err(anyhow::anyhow!("failed to connect to control plane: {}", e));
        }
    }

    // Spawn sync poller
    {
        let supervisor = supervisor.clone();
        let client = client.clone();
        let config = config.clone();
        let cancel = cancel.clone();
        tracker.spawn(async move {
            sync::poller::run_sync_loop(&supervisor, &client, &config, cancel).await;
        });
    }

    // Build HTTP router
    let app_state = api::AppState::new(supervisor.clone());
    let app = api::build_router(app_state);

    // Bind and serve
    let listener = TcpListener::bind(&config.listen_addr)
        .await
        .context("failed to bind TCP listener")?;
    info!(addr = config.listen_addr, "listening");

    // Serve with graceful shutdown
    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal(cancel.clone()))
        .await
        .context("server error")?;

    // Shutdown sequence
    info!("shutting down...");
    cancel.cancel();
    tracker.close();
    tracker.wait().await;
    supervisor.shutdown().await;
    info!("bridge stopped");

    Ok(())
}

/// Initialize tracing/logging based on configuration.
fn init_logging(config: &RuntimeConfig) {
    use tracing_subscriber::EnvFilter;

    let env_filter =
        EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new(&config.log_level));

    match config.log_format {
        bridge_core::LogFormat::Json => {
            tracing_subscriber::fmt()
                .json()
                .with_env_filter(env_filter)
                .init();
        }
        bridge_core::LogFormat::Text => {
            tracing_subscriber::fmt().with_env_filter(env_filter).init();
        }
    }
}

/// Wait for a shutdown signal (SIGTERM, SIGINT, or cancellation token).
async fn shutdown_signal(cancel: CancellationToken) {
    let ctrl_c = async {
        tokio::signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => info!("received SIGINT"),
        _ = terminate => info!("received SIGTERM"),
        _ = cancel.cancelled() => info!("cancellation requested"),
    }
}
