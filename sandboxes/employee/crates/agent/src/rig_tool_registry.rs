use std::future::Future;
use std::pin::Pin;
use std::sync::Arc;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use chrono::Utc;
use domain::{event_types, OutboundEvent, Reply, SessionId, ToolSpec};
use gateway::ChannelGateway;
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use serde_json::{json, Value};
use storage::CronJobRepo;
use tools::{JsonTool, ProcessRegistry, ToolDefinition};

pub type ToolFuture = Pin<Box<dyn Future<Output = Result<Value>> + Send>>;

pub struct DynamicTool {
    definition: ToolDefinition,
    executor: Arc<dyn Fn(Value) -> ToolFuture + Send + Sync>,
}

impl DynamicTool {
    pub fn new(
        definition: ToolDefinition,
        executor: impl Fn(Value) -> ToolFuture + Send + Sync + 'static,
    ) -> Self {
        Self {
            definition,
            executor: Arc::new(executor),
        }
    }
}

#[async_trait]
impl JsonTool for DynamicTool {
    fn definition(&self) -> ToolDefinition {
        self.definition.clone()
    }

    async fn call(&self, args: Value) -> Result<Value> {
        (self.executor)(args).await
    }
}

pub struct ToolContext {
    pub gateway: Option<Arc<dyn ChannelGateway>>,
    pub cron_repo: Option<Arc<dyn CronJobRepo>>,
    pub process_registry: Option<Arc<ProcessRegistry>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
}

pub fn build_agent_tools(
    specs: &[ToolSpec],
    session_id: &SessionId,
    ctx: &ToolContext,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools: Vec<Arc<dyn JsonTool>> = Vec::new();
    let session_is_cron = session_id.as_str().contains("-cron-");

    for spec in specs {
        match spec {
            ToolSpec::PostStatusUpdate => {
                if let Some(gateway) = &ctx.gateway {
                    if session_is_cron {
                        tools.push(post_to_channel_tool(
                            gateway.clone(),
                            derive_channel(session_id),
                        ));
                    } else {
                        tools.push(status_update_tool(gateway.clone(), session_id.clone()));
                    }
                }
            }
            ToolSpec::PostToChannel => {
                if let Some(gateway) = &ctx.gateway {
                    if session_is_cron {
                        tools.push(post_to_channel_tool(
                            gateway.clone(),
                            derive_channel(session_id),
                        ));
                    }
                }
            }
            ToolSpec::Cron => {
                if let Some(repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(cron_tool(repo.clone(), session_id.clone()));
                    }
                }
            }
            ToolSpec::Wake => {
                if let Some(repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(wake_tool(repo.clone(), session_id.clone()));
                    }
                }
            }
            ToolSpec::CheckBashStatus => {
                if let Some(registry) = &ctx.process_registry {
                    tools.push(check_bash_status_tool(registry.clone()));
                }
            }
            ToolSpec::LoadTools => {
                if let Some(registry) = &ctx.mcp_registry {
                    tools.push(load_tools_tool(registry.clone(), session_id.clone()));
                }
            }
            ToolSpec::Delegate => {
                if let Some(repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(delegate_tool(repo.clone(), session_id.clone()));
                    }
                }
            }
            ToolSpec::CheckDelegatedStatus => {
                if let Some(repo) = &ctx.cron_repo {
                    tools.push(check_delegated_status_tool(repo.clone()));
                }
            }
            _ => {}
        }
    }

    tools
}

pub async fn emit_tool_invoked(
    emitter: Option<Arc<OutboundEmitter>>,
    session_id: &SessionId,
    tool: &str,
    args: &Value,
    result: &Value,
) {
    let Some(emitter) = emitter else { return };
    let summary: String = result.to_string().chars().take(200).collect();
    emitter
        .emit(OutboundEvent::new(
            event_types::TOOL_INVOKED,
            json!({
                "session_id": session_id.as_str(),
                "tool": tool,
                "args": args,
                "result_summary": summary,
            }),
        ))
        .await;
}

pub async fn emit_tool_error(
    emitter: Option<Arc<OutboundEmitter>>,
    session_id: &SessionId,
    tool: &str,
    args: &Value,
    error: &str,
) {
    let Some(emitter) = emitter else { return };
    emitter
        .emit(OutboundEvent::new(
            event_types::ERROR_TOOL,
            json!({
                "session_id": session_id.as_str(),
                "tool": tool,
                "args": args,
                "error": error,
            }),
        ))
        .await;
}

fn status_update_tool(
    gateway: Arc<dyn ChannelGateway>,
    session_id: SessionId,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "post_status_update".into(),
            description: "Post a brief status update to the thread so the user knows what you are working on.".into(),
            parameters: json!({"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}),
        },
        move |args| {
            let gateway = gateway.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let message = args.get("message").and_then(Value::as_str).ok_or_else(|| anyhow!("message required"))?;
                gateway.reply(&session_id, Reply::Text(message.to_string())).await?;
                Ok(json!({"message": message, "posted": true}))
            })
        },
    ))
}

fn post_to_channel_tool(gateway: Arc<dyn ChannelGateway>, channel: String) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "post_to_channel".into(),
            description: "Post a message to the current channel.".into(),
            parameters: json!({"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}),
        },
        move |args| {
            let gateway = gateway.clone();
            let channel = channel.clone();
            Box::pin(async move {
                let message = args
                    .get("message")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("message required"))?;
                gateway
                    .post_to_channel(&channel, Reply::Text(message.to_string()))
                    .await?;
                Ok(json!({"message": message, "posted": true, "channel": channel}))
            })
        },
    ))
}

fn cron_tool(repo: Arc<dyn CronJobRepo>, session_id: SessionId) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cron".into(),
            description: "Manage recurring scheduled cron jobs. Actions: create, list, update, cancel, pause, resume.".into(),
            parameters: json!({"type":"object","properties":{"action":{"type":"string"},"job_id":{"type":"string"},"task_prompt":{"type":"string"},"interval_seconds":{"type":"integer"},"description":{"type":"string"},"repeat_count":{"type":"integer"},"channel_id":{"type":"string"}},"required":["action"]}),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            Box::pin(async move { execute_cron(repo, session_id, args).await })
        },
    ))
}

fn wake_tool(repo: Arc<dyn CronJobRepo>, session_id: SessionId) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "wake".into(),
            description: "Schedule a wake-up reminder in this conversation.".into(),
            parameters: json!({"type":"object","properties":{"seconds":{"type":"integer"},"task_prompt":{"type":"string"}},"required":["seconds","task_prompt"]}),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let seconds = args
                    .get("seconds")
                    .and_then(Value::as_u64)
                    .ok_or_else(|| anyhow!("seconds required"))?;
                let task_prompt = args
                    .get("task_prompt")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("task_prompt required"))?
                    .to_string();
                let now = Utc::now();
                let id = format!("wake-{}", now.timestamp_millis());
                let job = domain::cron::CronJob {
                    id: id.clone(),
                    description: task_prompt.chars().take(80).collect(),
                    channel: derive_channel(&session_id),
                    task_prompt,
                    cron_expression: None,
                    interval_seconds: None,
                    repeat_count: Some(1),
                    repeat_completed: 0,
                    state: domain::cron::CronJobState::Active,
                    source: domain::cron::CronJobSource::Cron,
                    next_run_at: now + chrono::Duration::seconds(seconds as i64),
                    last_run_at: None,
                    last_status: None,
                    last_error: None,
                    delegated_session_id: None,
                    session_continuation_id: Some(session_id.as_str().to_string()),
                    created_at: now,
                    created_by_session: session_id.as_str().to_string(),
                };
                repo.create(&job).await?;
                Ok(json!({"job_id": id, "next_run_at": job.next_run_at.to_rfc3339()}))
            })
        },
    ))
}

fn load_tools_tool(registry: Arc<McpRegistry>, session_id: SessionId) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "load_tools".into(),
            description: "Load MCP tool schemas into the agent for use.".into(),
            parameters: json!({"type":"object","properties":{"tool_names":{"type":"array","items":{"type":"string"}}},"required":["tool_names"]}),
        },
        move |args| {
            let registry = registry.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let names: Vec<String> = args
                    .get("tool_names")
                    .and_then(Value::as_array)
                    .unwrap_or(&Vec::new())
                    .iter()
                    .filter_map(|v| v.as_str().map(ToString::to_string))
                    .collect();
                let loaded = registry.load_tools_for_session(session_id.as_str(), &names);
                Ok(json!({
                    "loaded": loaded,
                    "total_loaded": registry.loaded_tool_names_for_session(session_id.as_str()).len(),
                    "still_unloaded": registry.unloaded_tool_names_for_session(session_id.as_str()).len()
                }))
            })
        },
    ))
}

fn check_bash_status_tool(registry: Arc<ProcessRegistry>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "check_bash_status".into(),
            description: "Check the status of a background bash process.".into(),
            parameters: json!({"type":"object","properties":{"process_id":{"type":"string"}},"required":["process_id"]}),
        },
        move |args| {
            let registry = registry.clone();
            Box::pin(async move {
                let id = args
                    .get("process_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("process_id required"))?;
                let status = registry
                    .status(id)
                    .ok_or_else(|| anyhow!("process not found"))?;
                Ok(json!({
                    "process_id": id,
                    "running": status.running,
                    "exit_code": status.exit_code,
                    "output": status.output,
                }))
            })
        },
    ))
}

fn delegate_tool(repo: Arc<dyn CronJobRepo>, session_id: SessionId) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "delegate".into(),
            description: "Spawn background delegated tasks in isolated conversations.".into(),
            parameters: json!({"type":"object","properties":{"run_in_background":{"type":"boolean"},"tasks":{"type":"array","items":{"type":"object","properties":{"goal":{"type":"string"},"context":{"type":["string","null"]},"toolsets":{"type":["array","null"],"items":{"type":"string"}}},"required":["goal"]}}},"required":["tasks"]}),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let tasks = args
                    .get("tasks")
                    .and_then(Value::as_array)
                    .ok_or_else(|| anyhow!("tasks required"))?;
                let now = Utc::now();
                let mut jobs = Vec::new();
                for (idx, task) in tasks.iter().enumerate() {
                    let goal = task
                        .get("goal")
                        .and_then(Value::as_str)
                        .ok_or_else(|| anyhow!("task.goal required"))?
                        .to_string();
                    let id = format!("delegate-{}-{idx}", now.timestamp_millis());
                    let child_session = format!("{}-delegate-{}", session_id.as_str(), id);
                    let job = domain::cron::CronJob {
                        id: id.clone(),
                        description: goal.chars().take(80).collect(),
                        channel: derive_channel(&session_id),
                        task_prompt: goal,
                        cron_expression: None,
                        interval_seconds: None,
                        repeat_count: Some(1),
                        repeat_completed: 0,
                        state: domain::cron::CronJobState::Active,
                        source: domain::cron::CronJobSource::Delegate,
                        next_run_at: now,
                        last_run_at: None,
                        last_status: Some("queued".into()),
                        last_error: None,
                        delegated_session_id: Some(child_session.clone()),
                        session_continuation_id: None,
                        created_at: now,
                        created_by_session: session_id.as_str().to_string(),
                    };
                    repo.create(&job).await?;
                    jobs.push(
                        json!({"job_id": id, "session_id": child_session, "state": "queued"}),
                    );
                }
                Ok(json!({"jobs": jobs}))
            })
        },
    ))
}

fn check_delegated_status_tool(repo: Arc<dyn CronJobRepo>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "check_delegated_status".into(),
            description: "Check the status of a background delegated task.".into(),
            parameters: json!({"type":"object","properties":{"job_id":{"type":"string"}},"required":["job_id"]}),
        },
        move |args| {
            let repo = repo.clone();
            Box::pin(async move {
                let id = args
                    .get("job_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("job_id required"))?;
                let job = repo
                    .get(id)
                    .await?
                    .ok_or_else(|| anyhow!("job not found"))?;
                Ok(
                    json!({"job_id": job.id, "state": format!("{:?}", job.state), "last_status": job.last_status, "last_error": job.last_error, "session_id": job.delegated_session_id}),
                )
            })
        },
    ))
}

async fn execute_cron(
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
    args: Value,
) -> Result<Value> {
    let action = args
        .get("action")
        .and_then(Value::as_str)
        .ok_or_else(|| anyhow!("action required"))?;
    match action {
        "create" => {
            let task_prompt = args
                .get("task_prompt")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("task_prompt required"))?
                .to_string();
            let interval_seconds = args
                .get("interval_seconds")
                .and_then(Value::as_u64)
                .ok_or_else(|| anyhow!("interval_seconds required"))?;
            let now = Utc::now();
            let id = format!("cron-{}", now.timestamp_millis());
            let channel = args
                .get("channel_id")
                .and_then(Value::as_str)
                .map(ToString::to_string)
                .unwrap_or_else(|| derive_channel(&session_id));
            let job = domain::cron::CronJob {
                id: id.clone(),
                description: args
                    .get("description")
                    .and_then(Value::as_str)
                    .map(ToString::to_string)
                    .unwrap_or_else(|| task_prompt.chars().take(80).collect()),
                channel,
                task_prompt,
                cron_expression: None,
                interval_seconds: Some(interval_seconds),
                repeat_count: args
                    .get("repeat_count")
                    .and_then(Value::as_u64)
                    .map(|v| v as u32),
                repeat_completed: 0,
                state: domain::cron::CronJobState::Active,
                source: domain::cron::CronJobSource::Cron,
                next_run_at: now + chrono::Duration::seconds(interval_seconds as i64),
                last_run_at: None,
                last_status: None,
                last_error: None,
                delegated_session_id: None,
                session_continuation_id: None,
                created_at: now,
                created_by_session: session_id.as_str().to_string(),
            };
            repo.create(&job).await?;
            Ok(
                json!({"job_id": id, "next_run_at": job.next_run_at.to_rfc3339(), "interval_seconds": interval_seconds, "channel": job.channel}),
            )
        }
        "list" => {
            let jobs = repo
                .list_by_source(domain::cron::CronJobSource::Cron)
                .await?;
            Ok(json!({"jobs": jobs, "total": jobs.len()}))
        }
        "cancel" => {
            let id = args
                .get("job_id")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("job_id required"))?;
            repo.delete(id).await?;
            Ok(json!({"cancelled": true, "job_id": id}))
        }
        "pause" => {
            let id = args
                .get("job_id")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("job_id required"))?;
            repo.set_state(id, domain::cron::CronJobState::Paused)
                .await?;
            Ok(json!({"paused": true, "job_id": id}))
        }
        "resume" => {
            let id = args
                .get("job_id")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("job_id required"))?;
            repo.set_state(id, domain::cron::CronJobState::Active)
                .await?;
            Ok(json!({"resumed": true, "job_id": id}))
        }
        "update" => {
            let id = args
                .get("job_id")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("job_id required"))?;
            if let Some(prompt) = args.get("task_prompt").and_then(Value::as_str) {
                repo.update_prompt(id, prompt.to_string()).await?;
            }
            Ok(json!({"updated": true, "job_id": id}))
        }
        _ => Err(anyhow!("unknown cron action")),
    }
}

fn derive_channel(session_id: &SessionId) -> String {
    session_id
        .as_str()
        .split_once('-')
        .map(|(c, _)| c.to_string())
        .unwrap_or_else(|| session_id.as_str().to_string())
}
