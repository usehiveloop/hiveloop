use anyhow::Context;
use bridge_core::RuntimeConfig;
use figment::providers::{Env, Format, Serialized, Toml};
use figment::Figment;
use lsp::LspManager;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio_util::sync::CancellationToken;
use tracing::info;

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

    // Create shared services
    let mcp_manager = Arc::new(McpManager::new());

    // Create LSP manager for code intelligence
    let project_root = std::env::current_dir().unwrap_or_default();
    let lsp_config = config.lsp.clone().and_then(|lsp_cfg| {
        if lsp_cfg.is_disabled() {
            // LSP explicitly disabled — pass empty config so no servers are registered
            Some(std::collections::HashMap::new())
        } else {
            lsp_cfg.into_servers().map(|server_map| {
                server_map
                    .into_iter()
                    .map(|(id, cfg)| {
                        (
                            id,
                            lsp::LspServerConfig {
                                command: cfg.command,
                                extensions: cfg.extensions,
                                env: cfg.env,
                                initialization_options: cfg.initialization_options,
                                disabled: cfg.disabled,
                            },
                        )
                    })
                    .collect()
            })
        }
    });
    let lsp_manager = Arc::new(LspManager::new(project_root, lsp_config));

    let supervisor = Arc::new(AgentSupervisor::with_lsp(
        mcp_manager.clone(),
        lsp_manager,
        cancel.clone(),
    ));

    // Create app state — bridge starts with zero agents, waits for pushes
    let app_state = api::AppState::new(supervisor.clone(), config.control_plane_api_key.clone());

    // Build HTTP router
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
