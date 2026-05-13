mod bridge_gateway;
mod handler;
mod scheduler;
mod sentry_support;
mod session_coordinator;

use bridge_gateway::BridgeGateway;
use scheduler::CronScheduler;
use session_coordinator::SessionCoordinator;

use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;

use agent::{
    cloud_agents::{CloudAgentConfig, CloudAgentService},
    AgentRunner, RigAgentRunner,
};
use anyhow::{Context, Result};
use api::{ApiState, OutboundConfigReloader};
use async_trait::async_trait;
use domain::{
    AgentDefinition, AgentMeta, BashConfig, ConfigStore, InboundEvent, ModelConfig,
    OutboundChannelSpec, PromptFragment, PromptFragments, ReadFileConfig, ReasoningEffort,
    SlackConfig, ToolSpec, WriteFileConfig,
};
use gateway::{ChannelGateway, SlackGateway};
use mcp::McpRegistry;
use outbound::{build_registry, OutboundDispatcher, OutboundEmitter, OutboundRegistry};
use skills::SkillWriter;
use sqlx::SqlitePool;
use storage::{
    init_sqlite_pool, SqliteConfigRepo, SqliteCronJobRepo, SqliteEventRepo,
    SqliteInboundDedupeRepo, SqliteOutboxRepo, SqliteSessionRepo,
};
use tokio::sync::{mpsc, RwLock};
use tracing::{error, info, warn};

#[tokio::main]
async fn main() -> Result<()> {
    let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
    let _ = dotenvy::dotenv();

    let sentry_guard = sentry_support::init_sentry();
    sentry_support::init_tracing(sentry_guard.is_some());
    if sentry_guard.is_some() {
        info!("sentry reporting enabled");
    } else {
        info!("sentry reporting disabled; set SENTRY_DSN or SENTRY_SPOTLIGHT=true to enable");
    }

    let slack_bot_token = required_env("SLACK_BOT_TOKEN", "<slack-bot-token>")?;
    let slack_app_token = required_env("SLACK_APP_TOKEN", "<slack-app-token>")?;
    let runtime_secret = required_env("RUNTIME_SECRET", "shared bearer token")?;
    let bind_addr_text =
        std::env::var("RUNTIME_BIND_ADDR").unwrap_or_else(|_| "0.0.0.0:7080".into());
    let bind_addr: SocketAddr = bind_addr_text
        .parse()
        .context("RUNTIME_BIND_ADDR must be a socket address")?;
    let database_path =
        std::env::var("DB_PATH").unwrap_or_else(|_| "./data/employee-bridge.db".into());
    let workspace_root_string =
        std::env::var("WORKSPACE_ROOT").unwrap_or_else(|_| "./workspace".into());
    let workspace_root = PathBuf::from(&workspace_root_string);
    if let Err(error) = std::fs::create_dir_all(&workspace_root) {
        warn!(workspace = %workspace_root.display(), %error, "failed to create workspace root");
    }
    info!(workspace = %workspace_root.display(), "workspace ready");
    info!(database = %database_path, "initializing storage");
    let sqlite_pool = init_sqlite_pool(PathBuf::from(&database_path)).await?;
    let config_repo: Arc<dyn storage::ConfigRepo> =
        Arc::new(SqliteConfigRepo::new(sqlite_pool.clone()));
    let session_repo: Arc<dyn storage::SessionRepo> =
        Arc::new(SqliteSessionRepo::new(sqlite_pool.clone()));
    let event_repo: Arc<dyn storage::EventRepo> =
        Arc::new(SqliteEventRepo::new(sqlite_pool.clone()));
    let outbox_repo: Arc<dyn storage::OutboxRepo> =
        Arc::new(SqliteOutboxRepo::new(sqlite_pool.clone()));
    let _dedupe_repo: Arc<dyn storage::InboundDedupeRepo> =
        Arc::new(SqliteInboundDedupeRepo::new(sqlite_pool.clone()));
    let cron_repo: Arc<dyn storage::CronJobRepo> =
        Arc::new(SqliteCronJobRepo::new(sqlite_pool.clone()));

    let initial_definition = match config_repo.load().await? {
        Some(persisted) => {
            info!("loaded agent definition from database");
            persisted
        }
        None => {
            info!("no persisted definition; bootstrapping from environment");
            let bootstrapped = bootstrap_agent_definition()?;
            config_repo.upsert(&bootstrapped).await?;
            bootstrapped
        }
    };

    let cloud_agent_service = match CloudAgentConfig::from_env() {
        Some(config) => {
            let service = Arc::new(CloudAgentService::new(config));
            match service.discover().await {
                Ok(agents) => {
                    info!(
                        cloud_agent_count = agents.len(),
                        "cloud-agent discovery succeeded"
                    );
                    Some(service)
                }
                Err(error) => {
                    warn!(%error, "cloud-agent discovery failed; cloud-agent tools remain enabled but prompt context was not injected");
                    Some(service)
                }
            }
        }
        None => {
            info!("cloud-agent env vars missing; cloud-agent tools and prompt injection disabled");
            None
        }
    };

    let mcp_registry = Arc::new(McpRegistry::from_specs(&initial_definition.mcp_servers).await);
    {
        config_repo.upsert(&initial_definition).await?;
    }

    let registry = build_registry(sqlite_pool.clone(), &initial_definition.outbound_channels)
        .map_err(|e| anyhow::anyhow!("build outbound registry: {e}"))?;
    let registry = Arc::new(RwLock::new(registry));
    let outbound_reloader: Arc<dyn OutboundConfigReloader> = Arc::new(RegistryReloader {
        sqlite_pool: sqlite_pool.clone(),
        registry: registry.clone(),
    });

    let dispatcher = OutboundDispatcher::new(outbox_repo.clone(), registry.clone());
    let (dispatcher_handle, dispatcher_cancel) = dispatcher.spawn();

    let emitter = Arc::new(OutboundEmitter::new(outbox_repo.clone(), registry.clone()));

    let skill_writer = Arc::new(SkillWriter::new(workspace_root.clone()));
    skill_writer.sync(&initial_definition.skills);

    info!(
        agent = %initial_definition.agent.name,
        outbound_channels = initial_definition.outbound_channels.len(),
        "starting employee-bridge"
    );

    let config = ConfigStore::new(initial_definition.clone());

    let slack_gateway: Arc<dyn ChannelGateway> = Arc::new(SlackGateway::new(
        slack_bot_token,
        slack_app_token,
        config.clone(),
    )?);
    let http_stream_broker = Arc::new(api::HttpStreamBroker::new());
    let bridge_gateway: Arc<dyn ChannelGateway> = Arc::new(BridgeGateway::new(
        slack_gateway.clone(),
        http_stream_broker.clone(),
    ));

    let mut rig_runner = RigAgentRunner::new(config.clone(), workspace_root.clone())
        .with_outbound_emitter(emitter.clone())
        .with_gateway(bridge_gateway.clone())
        .with_cron_repo(cron_repo.clone())
        .with_event_repo(event_repo.clone())
        .with_mcp_registry(mcp_registry.clone());
    if let Some(service) = cloud_agent_service.as_ref() {
        rig_runner = rig_runner.with_cloud_agents(service.clone());
    }
    let agent_runner: Arc<dyn AgentRunner> = Arc::new(rig_runner);

    let (inbound_sink, mut inbound_events) = mpsc::channel::<InboundEvent>(256);

    let api_state = ApiState::new(
        config.clone(),
        config_repo.clone(),
        session_repo.clone(),
        event_repo.clone(),
        runtime_secret,
        skill_writer,
        Some(api::HttpGatewayState {
            inbound_sink: inbound_sink.clone(),
            broker: http_stream_broker.clone(),
        }),
        Some(mcp_registry.clone()),
        Some(outbound_reloader),
        cloud_agent_service.as_ref().map(|service| service.index()),
    );
    api_state.mark_config_loaded();
    let (api_handle, api_cancel) = api::serve(bind_addr, api_state.clone()).await;

    let coordinator = Arc::new(SessionCoordinator::new());

    let scheduler = CronScheduler::new(cron_repo.clone(), inbound_sink.clone());
    let _scheduler_handle = tokio::spawn(scheduler.run());

    let gateway_task = {
        let gateway = bridge_gateway.clone();
        let api_state_for_gateway = api_state.clone();
        tokio::spawn(async move {
            api_state_for_gateway.mark_gateway_ready();
            gateway.run(inbound_sink).await
        })
    };

    let event_loop = async {
        info!("listening for inbound events");
        while let Some(inbound) = inbound_events.recv().await {
            let runner = agent_runner.clone();
            let gateway = bridge_gateway.clone();
            let cfg = config.clone();
            let emitter = emitter.clone();
            let session_repo = session_repo.clone();
            let coordinator = coordinator.clone();
            let turn_event_sink: Arc<dyn handler::TurnEventSink> = http_stream_broker.clone();
            tokio::spawn(async move {
                if let Err(e) = handler::handle_inbound(
                    runner,
                    gateway,
                    cfg,
                    emitter,
                    session_repo,
                    coordinator,
                    turn_event_sink,
                    inbound,
                )
                .await
                {
                    error!(error = %e, "handler::handle_inbound failed");
                }
            });
        }
    };

    tokio::select! {
        result = gateway_task => match result {
            Ok(Ok(())) => info!("gateway exited cleanly"),
            Ok(Err(e)) => error!(error = %e, "gateway run failed"),
            Err(e) => error!(error = %e, "gateway task panicked"),
        },
        _ = event_loop => warn!("event loop exited"),
        _ = tokio::signal::ctrl_c() => info!("ctrl-c received; shutting down"),
    }

    let _ = dispatcher_cancel.send(());
    let _ = dispatcher_handle.await;
    let _ = api_cancel.send(());
    let _ = api_handle.await;
    drop(sentry_guard);
    Ok(())
}

struct RegistryReloader {
    sqlite_pool: Arc<SqlitePool>,
    registry: Arc<RwLock<OutboundRegistry>>,
}

#[async_trait]
impl OutboundConfigReloader for RegistryReloader {
    async fn reload_outbound_channels(&self, specs: &[OutboundChannelSpec]) -> anyhow::Result<()> {
        let next = build_registry(self.sqlite_pool.clone(), specs)
            .map_err(|error| anyhow::anyhow!("build outbound registry: {error}"))?;
        let names = next.names();
        *self.registry.write().await = next;
        info!(channels = ?names, "outbound registry reloaded from config");
        Ok(())
    }
}

fn required_env(key: &str, hint: &str) -> Result<String> {
    match std::env::var(key) {
        Ok(value) if !value.is_empty() => Ok(value),
        _ => anyhow::bail!("env var `{key}` must be set ({hint})"),
    }
}

fn bootstrap_agent_definition() -> Result<AgentDefinition> {
    let primary_model = build_model_from_env(
        "AGENT_MODEL",
        "AGENT_BASE_URL",
        "AGENT_API_KEY_ENV",
        Some("deepseek/deepseek-v4-flash"),
    )?
    .ok_or_else(|| anyhow::anyhow!("AGENT_MODEL must be set for the primary model"))?;
    let multimodal_model = build_model_from_env(
        "AGENT_MULTIMODAL_MODEL",
        "AGENT_MULTIMODAL_BASE_URL",
        "AGENT_MULTIMODAL_API_KEY_ENV",
        None,
    )?;
    Ok(AgentDefinition {
        agent: AgentMeta {
            name: "Aria".into(),
            description: "Hiveloop AI employee".into(),
            system_prompt: String::new(),
        },
        prompt_fragments: PromptFragments {
            identity: PromptFragment {
                title: "Identity".into(),
                content: "You are Aria, a friendly AI employee in Slack. Reply concisely using mrkdwn. Never invent features. If you do not know something, say so.".into(),
            },
            ..Default::default()
        },
        model: primary_model,
        multimodal_model,
        limits: Default::default(),
        context: Default::default(),
        tools: default_builtin_tool_specs(),
        mcp_servers: Vec::new(),
        skills: Vec::new(),
        subagents: Vec::new(),
        slack: SlackConfig::default(),
        outbound_channels: Vec::new(),
    })
}

fn default_builtin_tool_specs() -> Vec<ToolSpec> {
    vec![
        ToolSpec::Bash(BashConfig {
            workdir: ".".into(),
            timeout_seconds: 60,
            max_output_bytes: 5 * 1024 * 1024,
            deny_patterns: vec![
                "rm -rf /".into(),
                "rm -rf ~".into(),
                "mkfs".into(),
                "dd if=".into(),
                ":(){:|:&};:".into(),
                "shutdown".into(),
                "reboot".into(),
            ],
            env_passthrough: vec!["HOME".into(), "PATH".into(), "LANG".into(), "LC_ALL".into()],
            sandbox: "process_isolated".into(),
        }),
        ToolSpec::ReadFile(ReadFileConfig {
            allowed_roots: vec![],
            max_file_size_bytes: 5 * 1024 * 1024,
            deny_globs: vec![],
        }),
        ToolSpec::WriteFile(WriteFileConfig {
            allowed_roots: vec![],
            max_file_size_bytes: 5 * 1024 * 1024,
            deny_globs: vec![],
            atomic: true,
        }),
        ToolSpec::PostStatusUpdate,
        ToolSpec::PostToChannel,
        ToolSpec::Cron,
        ToolSpec::Delegate,
        ToolSpec::CheckDelegatedStatus,
        ToolSpec::CheckBashStatus,
        ToolSpec::Wake,
        ToolSpec::LoadTools,
        ToolSpec::SkillsList,
        ToolSpec::SkillView,
        ToolSpec::SkillManage,
        ToolSpec::CloudAgentLaunchTask,
        ToolSpec::CloudAgentTaskStatus,
        ToolSpec::CloudAgentListTasks,
        ToolSpec::CloudAgentTaskSendMessage,
        ToolSpec::CloudAgentTaskTerminate,
    ]
}

fn build_model_from_env(
    model_env_key: &str,
    base_url_env_key: &str,
    api_key_env_env_key: &str,
    fallback_model_id: Option<&str>,
) -> Result<Option<ModelConfig>> {
    let model_id_from_env = std::env::var(model_env_key).ok().filter(|v| !v.is_empty());
    let model_id = match (model_id_from_env, fallback_model_id) {
        (Some(value), _) => value,
        (None, Some(default)) => default.to_string(),
        (None, None) => return Ok(None),
    };
    let base_url = std::env::var(base_url_env_key)
        .ok()
        .filter(|v| !v.is_empty())
        .unwrap_or_else(|| "https://openrouter.ai/api/v1".into());
    let api_key_env = std::env::var(api_key_env_env_key)
        .ok()
        .filter(|v| !v.is_empty())
        .unwrap_or_else(|| "OPENROUTER_API_KEY".into());
    if std::env::var(&api_key_env).is_err() {
        anyhow::bail!("env var `{api_key_env}` must be set for model `{model_id}`");
    }
    Ok(Some(ModelConfig::OpenaiCompatible {
        base_url,
        model_id,
        api_key_env,
        temperature: Some(0.3),
        max_output_tokens: Some(1024),
        reasoning_effort: Some(ReasoningEffort::Low),
        extra_headers: Default::default(),
        fallback: None,
    }))
}
