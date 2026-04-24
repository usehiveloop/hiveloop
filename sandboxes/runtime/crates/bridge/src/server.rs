use anyhow::Context;
use bridge_core::RuntimeConfig;
use figment::providers::{Env, Format, Serialized, Toml};
use figment::Figment;
use lsp::LspManager;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use storage::{StorageBackend, StorageHandle};
use tokio::net::TcpListener;
use tokio_util::sync::CancellationToken;
use tracing::info;
use webhooks::EventBus;

use crate::logging::{init_logging, shutdown_signal};

pub(crate) async fn run_server() -> anyhow::Result<()> {
    // Load configuration from config.toml and environment variables
    let config: RuntimeConfig = Figment::from(Serialized::defaults(RuntimeConfig::default()))
        .merge(Toml::file("config.toml"))
        .merge(Env::prefixed("BRIDGE_"))
        .extract()
        .context("failed to load configuration")?;

    // Initialize logging
    init_logging(&config);

    info!("bridge starting");

    // Install rtk filter set to the user-global config (macOS:
    // ~/Library/Application Support/rtk/filters.toml, Linux: ~/.config/rtk/
    // filters.toml). This is idempotent and fast (~100KB write, skipped if
    // content is already identical). A failure here must NOT block startup —
    // rtk integration is opportunistic.
    match tools::bash::ensure_filters_installed() {
        Ok(path) => {
            if tools::bash::is_rtk_available() {
                info!(path = %path.display(), "rtk integration active: Bash tool will route through `rtk rewrite`");
            } else {
                info!(path = %path.display(), "rtk filters written but `rtk` binary not on PATH; Bash tool runs unchanged");
            }
        }
        Err(e) => {
            tracing::warn!(error = %e, "rtk filter bootstrap skipped — Bash tool runs unchanged");
        }
    }

    // Create global lifecycle primitives
    let cancel = CancellationToken::new();

    let (storage_backend, storage_handle): (
        Option<Arc<dyn StorageBackend>>,
        Option<StorageHandle>,
    ) = match storage::init_storage()
        .await
        .context("failed to initialize storage")?
    {
        Some((backend, handle)) => (Some(backend), Some(handle)),
        None => (None, None),
    };

    if storage_backend.is_some() {
        info!("storage persistence enabled");
    } else {
        info!("storage persistence disabled");
    }

    // Create the unified event bus with optional webhook HTTP delivery.
    let webhook_url = config.webhook_url.clone().unwrap_or_default();
    let webhook_secret = config.control_plane_api_key.clone();

    let webhook_tx = if config.webhook_url.is_some() {
        let webhook_config = config.webhook_config.clone().unwrap_or_default();
        let (tx, rx) = tokio::sync::mpsc::unbounded_channel();
        let client = reqwest::Client::new();
        let url = webhook_url.clone();
        let secret = webhook_secret.clone();
        tokio::spawn(webhooks::run_delivery(
            rx,
            client,
            cancel.clone(),
            webhook_config,
            url,
            secret,
            storage_handle.clone(),
        ));
        info!(url = %webhook_url, "webhook delivery started");
        Some(tx)
    } else {
        None
    };

    let event_bus = Arc::new(EventBus::new(
        webhook_tx,
        storage_handle.clone(),
        webhook_url,
        webhook_secret,
    ));

    if config.websocket_enabled {
        info!("WebSocket event stream enabled on /ws/events");
    }

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

    let supervisor = Arc::new(
        AgentSupervisor::with_lsp(mcp_manager.clone(), lsp_manager, cancel.clone())
            .with_capacity_limits(&config)
            .with_event_bus(Some(event_bus.clone()))
            .with_storage_backend(storage_backend.clone())
            .with_storage(storage_handle.clone()),
    );

    // Create app state — bridge starts with zero agents, waits for pushes
    let app_state = api::AppState::new(
        supervisor.clone(),
        config.control_plane_api_key.clone(),
        storage_backend.clone(),
        cancel.clone(),
        event_bus.clone(),
    );

    if let Some(backend) = &storage_backend {
        restore_from_storage(backend, &supervisor, &app_state, &event_bus).await?;
    }

    // Build HTTP router
    let app = api::build_router(app_state);

    if let Some(storage_handle) = storage_handle.clone() {
        spawn_metrics_collector(supervisor.clone(), storage_handle, cancel.clone());
    }

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
    if let Some(handle) = storage_handle {
        handle.drain().await;
    }

    info!("bridge stopped");

    Ok(())
}

async fn restore_from_storage(
    backend: &Arc<dyn StorageBackend>,
    supervisor: &Arc<AgentSupervisor>,
    app_state: &api::AppState,
    event_bus: &Arc<EventBus>,
) -> anyhow::Result<()> {
    let stored_agents = backend
        .load_all_agents()
        .await
        .context("failed to load stored agents")?;

    if !stored_agents.is_empty() {
        let agent_count = stored_agents.len();
        supervisor
            .load_agents(stored_agents.clone())
            .await
            .context("failed to restore stored agents")?;
        info!(count = agent_count, "restored agents from storage");
    }

    let mut restored_conversations = 0usize;
    for agent in &stored_agents {
        let records = backend
            .load_conversations(&agent.id)
            .await
            .with_context(|| format!("failed to load stored conversations for {}", agent.id))?;

        if records.is_empty() {
            continue;
        }

        let count = records.len();
        let sse_receivers = supervisor.hydrate_conversations(&agent.id, records).await;
        for (conv_id, sse_rx) in sse_receivers {
            app_state.sse_streams.insert(conv_id, sse_rx);
            restored_conversations += 1;
        }
        info!(agent_id = %agent.id, count = count, "restored conversations from storage");
    }

    if restored_conversations > 0 {
        info!(
            count = restored_conversations,
            "restored active conversations from storage"
        );
    }

    // Replay pending events through the event bus
    let pending_events = backend
        .load_pending_events()
        .await
        .context("failed to load pending events")?;

    if !pending_events.is_empty() {
        let count = pending_events.len();
        for event in pending_events {
            event_bus.emit_replayed(event);
        }
        info!(count = count, "replayed pending events from storage");
    }

    Ok(())
}

fn spawn_metrics_collector(
    supervisor: Arc<AgentSupervisor>,
    storage_handle: StorageHandle,
    cancel: CancellationToken,
) {
    tokio::spawn(async move {
        let mut interval = tokio::time::interval(std::time::Duration::from_secs(60));
        interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                _ = interval.tick() => {
                    let snapshots = supervisor.collect_metrics().await;
                    for snapshot in snapshots {
                        storage_handle.save_metrics_snapshot(snapshot.agent_id.clone(), snapshot);
                    }
                }
            }
        }
    });
}
