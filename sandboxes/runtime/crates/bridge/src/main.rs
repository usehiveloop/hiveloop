use anyhow::Context;
use bridge_core::RuntimeConfig;
use clap::{Parser, Subcommand};
use figment::providers::{Env, Format, Serialized, Toml};
use figment::Figment;
use lsp::LspManager;
use mcp::McpManager;
use runtime::AgentSupervisor;
use std::sync::Arc;
use storage::{StorageBackend, StorageHandle};
use tokio::net::TcpListener;
use tokio_util::sync::CancellationToken;
use tracing::{info, warn};
use webhooks::{WebhookContext, WebhookDispatcher, WsBroadcaster};

/// Bridge - AI Agent Runtime
#[derive(Parser)]
#[command(name = "bridge")]
#[command(about = "AI Agent Runtime with tool execution and MCP support")]
#[command(version = "0.6.2")]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,
    /// Install LSP servers on startup (comma-separated list or "all")
    #[arg(long, value_name = "SERVERS")]
    install_lsp_servers: Option<String>,
}

#[derive(Subcommand)]
enum Commands {
    /// List available tools
    Tools {
        #[command(subcommand)]
        action: Option<ToolCommands>,
    },
}

#[derive(Subcommand)]
enum ToolCommands {
    /// List all available tools
    List {
        /// Output as JSON
        #[arg(long, default_value_t = true)]
        json: bool,
        /// Show only read-only tools (tools that don't modify state)
        #[arg(long)]
        read_only: bool,
    },
}

/// Tool information for JSON output
#[derive(serde::Serialize)]
struct ToolInfo {
    name: String,
    description: String,
    category: String,
    #[serde(skip)]
    is_read_only: bool,
    parameters: serde_json::Value,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Some(Commands::Tools { action }) => {
            handle_tools_command(action).await?;
            Ok(())
        }
        None => {
            let servers_to_install = cli.install_lsp_servers.map(|s| {
                s.split(',')
                    .map(|s| s.trim().to_string())
                    .collect::<Vec<String>>()
            });
            run_server(servers_to_install).await
        }
    }
}

async fn handle_tools_command(action: Option<ToolCommands>) -> anyhow::Result<()> {
    let action = action.unwrap_or(ToolCommands::List {
        json: true,
        read_only: false,
    });

    match action {
        ToolCommands::List { json: _, read_only } => {
            let tools = get_tools_info(read_only)?;
            println!("{}", serde_json::to_string_pretty(&tools)?);
            Ok(())
        }
    }
}

fn get_tools_info(filter_read_only: bool) -> anyhow::Result<Vec<ToolInfo>> {
    use tools::{register_builtin_tools, ToolRegistry};

    let mut registry = ToolRegistry::new();
    register_builtin_tools(&mut registry);

    let mut tools: Vec<ToolInfo> = registry
        .snapshot()
        .values()
        .map(|tool| {
            let name = tool.name();
            let category = categorize_tool(name);
            let is_read_only = is_read_only_tool(name);

            ToolInfo {
                name: name.to_string(),
                description: tool.description().to_string(),
                category,
                is_read_only,
                parameters: tool.parameters_schema(),
            }
        })
        .collect();

    // Filter to read-only tools if requested
    if filter_read_only {
        tools.retain(|t| t.is_read_only);
    }

    Ok(tools)
}

fn categorize_tool(name: &str) -> String {
    match name {
        "bash" | "agent" | "parallel_agent" | "join" | "Batch" => "action".to_string(),
        "Read" | "write" | "edit" | "apply_patch" | "LS" | "Glob" | "Grep" => {
            "filesystem".to_string()
        }
        "web_fetch" | "WebSearch" => "web".to_string(),
        "TodoWrite" | "TodoRead" => "task".to_string(),
        "lsp" => "code".to_string(),
        "skill" => "skill".to_string(),
        _ => "other".to_string(),
    }
}

/// Check if a tool is read-only (doesn't modify state)
fn is_read_only_tool(name: &str) -> bool {
    matches!(
        name,
        "Read" | "Grep" | "Glob" | "LS" | "web_fetch" | "todoread"
    )
}

async fn run_server(servers_to_install: Option<Vec<String>>) -> anyhow::Result<()> {
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

    // Create WebSocket broadcaster if enabled
    let ws_broadcaster: Option<Arc<WsBroadcaster>> = if config.websocket_enabled {
        let broadcaster = Arc::new(WsBroadcaster::new());
        info!("WebSocket event stream enabled on /ws/events");
        Some(broadcaster)
    } else {
        None
    };

    // Create webhook dispatcher if webhooks or WebSocket are enabled.
    // The dispatcher is the single fan-out point — WebSocket piggybacks
    // on every dispatch() call via the attached broadcaster.
    let webhook_ctx: Option<WebhookContext> =
        if config.webhook_url.is_some() || config.websocket_enabled {
            let webhook_config = config.webhook_config.clone().unwrap_or_default();
            let (dispatcher, rx) = WebhookDispatcher::with_config(&webhook_config);
            let client = dispatcher.client();
            let dispatcher = Arc::new(
                dispatcher
                    .with_storage(storage_handle.clone())
                    .with_ws_broadcaster(ws_broadcaster.clone()),
            );

            if let Some(ref url) = config.webhook_url {
                // Webhooks enabled — spawn the HTTP delivery loop
                tokio::spawn(WebhookDispatcher::run(
                    rx,
                    client,
                    cancel.clone(),
                    webhook_config,
                    storage_handle.clone(),
                ));
                info!(url = %url, "webhook dispatcher started");
            } else {
                // WebSocket-only: drain the webhook channel so it doesn't fill up
                tokio::spawn(async move {
                    let mut rx = rx;
                    while rx.recv().await.is_some() {}
                });
            }

            Some(WebhookContext {
                dispatcher,
                url: config.webhook_url.clone().unwrap_or_default(),
                secret: config.control_plane_api_key.clone(),
            })
        } else {
            None
        };

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
            .with_webhooks(webhook_ctx.clone())
            .with_storage_backend(storage_backend.clone())
            .with_storage(storage_handle.clone()),
    );

    // Create app state — bridge starts with zero agents, waits for pushes
    let app_state = api::AppState::new(
        supervisor.clone(),
        config.control_plane_api_key.clone(),
        webhook_ctx,
        storage_backend.clone(),
        ws_broadcaster,
        cancel.clone(),
    );

    if let Some(backend) = &storage_backend {
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

        if let Some(ref webhook_ctx) = app_state.webhook_ctx {
            let pending_webhooks = backend
                .load_pending_webhooks()
                .await
                .context("failed to load pending webhooks")?;

            if !pending_webhooks.is_empty() {
                let count = pending_webhooks.len();
                for (_, payload) in pending_webhooks {
                    webhook_ctx.dispatcher.dispatch_replayed(payload);
                }
                info!(count = count, "replayed pending webhooks from storage");
            }
        }
    }

    // Build HTTP router
    let app = api::build_router(app_state);

    if let Some(storage_handle) = storage_handle.clone() {
        let supervisor = supervisor.clone();
        let cancel = cancel.clone();
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

    // Bind and serve
    let listener = TcpListener::bind(&config.listen_addr)
        .await
        .context("failed to bind TCP listener")?;
    info!(addr = config.listen_addr, "listening");

    // Spawn background LSP installer if requested
    if let Some(server_ids) = servers_to_install {
        tokio::spawn(async move {
            info!(servers = ?server_ids, "starting LSP server installation");
            let installer = lsp::LspInstaller::new();
            let results = installer.install(&server_ids).await;

            let mut succeeded = 0;
            let mut failed = 0;

            for (id, result) in results {
                match result {
                    Ok(_) => {
                        info!(server = %id, "installed successfully");
                        succeeded += 1;
                    }
                    Err(e) => {
                        warn!(server = %id, error = %e, "installation failed");
                        failed += 1;
                    }
                }
            }

            info!(succeeded, failed, "LSP server installation complete");
        });
    }

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

/// Initialize tracing/logging based on configuration.
/// When `BRIDGE_OTEL_ENDPOINT` is set, adds an OpenTelemetry layer that exports
/// spans via OTLP gRPC — all existing `tracing` spans become OTel spans.
fn init_logging(config: &RuntimeConfig) {
    use tracing_subscriber::layer::SubscriberExt;
    use tracing_subscriber::util::SubscriberInitExt;
    use tracing_subscriber::EnvFilter;

    // Build filter: honour RUST_LOG if set, otherwise use config log_level
    // with sensible defaults to suppress noisy library crates.
    let env_filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| {
        EnvFilter::new(format!(
            "{},rig=warn,h2=info,hyper_util=info,reqwest=info",
            config.log_level
        ))
    });

    // Optionally build OpenTelemetry layer for OTLP trace export
    let otel_layer = if let Some(ref endpoint) = config.otel_endpoint {
        match init_otel_tracer(endpoint, &config.otel_service_name) {
            Ok(tracer) => {
                eprintln!(
                    "OpenTelemetry tracing enabled: endpoint={}, service={}",
                    endpoint, config.otel_service_name
                );
                Some(tracing_opentelemetry::layer().with_tracer(tracer))
            }
            Err(e) => {
                eprintln!("Failed to initialize OpenTelemetry: {e}");
                None
            }
        }
    } else {
        None
    };

    // Compose: registry + env_filter + otel (optional) + fmt
    // OTel layer is added before fmt so it has the same subscriber type param.
    let registry = tracing_subscriber::registry()
        .with(env_filter)
        .with(otel_layer);

    match config.log_format {
        bridge_core::LogFormat::Json => {
            registry
                .with(tracing_subscriber::fmt::layer().json())
                .init();
        }
        bridge_core::LogFormat::Text => {
            registry.with(tracing_subscriber::fmt::layer()).init();
        }
    }
}

/// Initialize the OpenTelemetry OTLP tracer pipeline.
fn init_otel_tracer(
    endpoint: &str,
    service_name: &str,
) -> Result<opentelemetry_sdk::trace::SdkTracer, Box<dyn std::error::Error>> {
    use opentelemetry::trace::TracerProvider as _;
    use opentelemetry_otlp::WithExportConfig;
    use opentelemetry_sdk::trace::SdkTracerProvider;
    use opentelemetry_sdk::Resource;

    let exporter = opentelemetry_otlp::SpanExporter::builder()
        .with_tonic()
        .with_endpoint(endpoint)
        .build()?;

    let provider = SdkTracerProvider::builder()
        .with_simple_exporter(exporter)
        .with_resource(
            Resource::builder()
                .with_service_name(service_name.to_string())
                .build(),
        )
        .build();

    let tracer = provider.tracer("bridge");

    // Set as global provider so shutdown can flush
    opentelemetry::global::set_tracer_provider(provider);

    Ok(tracer)
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
