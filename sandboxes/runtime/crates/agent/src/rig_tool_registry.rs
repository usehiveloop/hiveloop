use std::future::Future;
use std::path::PathBuf;
use std::pin::Pin;
use std::sync::Arc;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::agent_registry::AgentDefinitionRegistry;
use domain::cron::{CronJob, CronJobSource, CronJobState};
use domain::{event_types, DelegateConfig, OutboundEvent, SessionId, ToolSpec};
use gateway::ChannelGateway;
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use serde_json::{json, Value};
use storage::{CronJobRepo, EventRepo};
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
    pub event_repo: Option<Arc<dyn EventRepo>>,
    pub process_registry: Option<Arc<ProcessRegistry>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
    pub workspace_root: PathBuf,
    pub outbound_emitter: Option<Arc<OutboundEmitter>>,
    pub agent_registry: Arc<AgentDefinitionRegistry>,
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
            ToolSpec::Cron => {
                if let Some(repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(cron_tool(
                            repo.clone(),
                            session_id.clone(),
                            ctx.outbound_emitter.clone(),
                        ));
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
            ToolSpec::SearchSessions => {
                if let Some(repo) = &ctx.event_repo {
                    tools.push(search_sessions_tool(repo.clone()));
                }
            }
            ToolSpec::SkillsList => {
                tools.push(skills_list_tool(ctx.workspace_root.clone()));
            }
            ToolSpec::SkillView => {
                tools.push(skill_view_tool(ctx.workspace_root.clone()));
            }
            ToolSpec::SkillManage => {
                tools.push(skill_manage_tool(
                    ctx.workspace_root.clone(),
                    session_id.clone(),
                    ctx.outbound_emitter.clone(),
                ));
            }
            ToolSpec::Delegate(config) => {
                if let Some(repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(delegate_tool(
                            repo.clone(),
                            session_id.clone(),
                            ctx.agent_registry.clone(),
                            config.clone(),
                        ));
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
                "source": event_source_from_session(session_id),
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
                "source": event_source_from_session(session_id),
                "tool": tool,
                "args": args,
                "error": error,
            }),
        ))
        .await;
}

fn event_source_from_session(session_id: &SessionId) -> &'static str {
    if session_id.as_str().starts_with("http-") {
        "http"
    } else {
        "unknown"
    }
}

fn search_sessions_tool(repo: Arc<dyn EventRepo>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "search_sessions".into(),
            description: "Search recent local conversation history from this sandbox. Use it to find prior user messages, agent replies, and compact tool summaries before relying on memory.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search terms for recent conversations"
                    },
                    "session_id": {
                        "type": "string",
                        "description": "Optional exact session id to search within"
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum matches to return, default 8, max 20"
                    }
                },
                "required": ["query"]
            }),
        },
        move |args| {
            let repo = repo.clone();
            Box::pin(async move {
                let query = args
                    .get("query")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .ok_or_else(|| anyhow!("query required"))?;
                let limit = args
                    .get("limit")
                    .and_then(Value::as_u64)
                    .unwrap_or(8)
                    .clamp(1, 20) as u32;
                let session_id = args
                    .get("session_id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(SessionId::from);
                let matches = repo
                    .search_sessions(query, session_id.as_ref(), limit)
                    .await?;
                let items: Vec<Value> = matches
                    .into_iter()
                    .map(|item| {
                        json!({
                            "session_id": item.session_id,
                            "event_id": item.event_id,
                            "event_kind": item.kind,
                            "created_at": item.created_at.to_rfc3339(),
                            "score": item.score,
                            "snippet": item.snippet,
                            "text": truncate_search_text(&item.content, 700),
                        })
                    })
                    .collect();
                Ok(json!({ "matches": items }))
            })
        },
    ))
}

fn truncate_search_text(value: &str, max_chars: usize) -> String {
    let mut out = String::new();
    for ch in value.chars().take(max_chars) {
        out.push(ch);
    }
    out
}

fn cron_tool(
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
    emitter: Option<Arc<OutboundEmitter>>,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cron".into(),
            description: "Manage recurring scheduled cron jobs. Actions: create, list, update, cancel, pause, resume.".into(),
            parameters: json!({"type":"object","properties":{"action":{"type":"string"},"job_id":{"type":"string"},"task_prompt":{"type":"string"},"interval_seconds":{"type":"integer"},"description":{"type":"string"},"repeat_count":{"type":"integer"},"channel_id":{"type":"string"}},"required":["action"]}),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            let emitter = emitter.clone();
            Box::pin(async move { execute_cron(repo, session_id, emitter, args).await })
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
                    agent_name: None,
                    last_result: None,
                };
                repo.create(&job).await?;
                Ok(json!({"job_id": id, "next_run_at": job.next_run_at.to_rfc3339()}))
            })
        },
    ))
}

fn skills_list_tool(workspace_root: PathBuf) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "skills_list".into(),
            description:
                "List available skills (name + description). Use skill_view(name) to load full content."
                    .into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "category": {
                        "type": "string",
                        "description": "Optional category filter to narrow results"
                    }
                },
                "required": []
            }),
        },
        move |args| {
            let workspace_root = workspace_root.clone();
            Box::pin(async move {
                let store = skills::SkillStore::new(workspace_root);
                Ok(store.list(args.get("category").and_then(Value::as_str)))
            })
        },
    ))
}

fn skill_view_tool(workspace_root: PathBuf) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "skill_view".into(),
            description: "Skills allow loading task workflows plus linked files. Load a skill's full content or access linked files under references/, templates/, scripts/, or assets/.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "The skill name (use skills_list to see available skills)."
                    },
                    "file_path": {
                        "type": "string",
                        "description": "Optional linked file path within the skill, e.g. references/api.md or scripts/check.sh."
                    }
                },
                "required": ["name"]
            }),
        },
        move |args| {
            let workspace_root = workspace_root.clone();
            Box::pin(async move {
                let name = args
                    .get("name")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("name required"))?;
                let store = skills::SkillStore::new(workspace_root);
                store.view(name, args.get("file_path").and_then(Value::as_str))
            })
        },
    ))
}

fn skill_manage_tool(
    workspace_root: PathBuf,
    session_id: SessionId,
    emitter: Option<Arc<OutboundEmitter>>,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "skill_manage".into(),
            description: "Manage filesystem-backed skills in /workspace/.skills. Actions: create, patch, edit, delete, write_file, remove_file. Use only when asked, or after confirming the user wants to save/update procedural memory.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "action": {"type": "string", "enum": ["create", "patch", "edit", "delete", "write_file", "remove_file"]},
                    "name": {"type": "string", "description": "Skill name: lowercase, numbers, hyphens/underscores, max 64 chars."},
                    "content": {"type": "string", "description": "Full SKILL.md content. Required for create and edit."},
                    "old_string": {"type": "string", "description": "Text to find for patch."},
                    "new_string": {"type": "string", "description": "Replacement text for patch."},
                    "replace_all": {"type": "boolean", "description": "For patch, replace all matches instead of requiring uniqueness."},
                    "category": {"type": "string", "description": "Optional category for create when content has no frontmatter."},
                    "file_path": {"type": "string", "description": "Supporting file path under references/, templates/, scripts/, or assets/."},
                    "file_content": {"type": "string", "description": "Content for write_file."},
                    "absorbed_into": {"type": "string", "description": "For delete, skill this was merged into, or empty string for pruning."}
                },
                "required": ["action", "name"]
            }),
        },
        move |args| {
            let workspace_root = workspace_root.clone();
            let session_id = session_id.clone();
            let emitter = emitter.clone();
            Box::pin(async move {
                let store = skills::SkillStore::new(workspace_root);
                let action = args
                    .get("action")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string();
                let name = args
                    .get("name")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string();
                let absorbed_into = args
                    .get("absorbed_into")
                    .and_then(Value::as_str)
                    .map(ToString::to_string);
                let result = store.manage(skills::SkillManageArgs {
                    action: action.clone(),
                    name: name.clone(),
                    content: args.get("content").and_then(Value::as_str).map(ToString::to_string),
                    category: args.get("category").and_then(Value::as_str).map(ToString::to_string),
                    file_path: args.get("file_path").and_then(Value::as_str).map(ToString::to_string),
                    file_content: args.get("file_content").and_then(Value::as_str).map(ToString::to_string),
                    old_string: args.get("old_string").and_then(Value::as_str).map(ToString::to_string),
                    new_string: args.get("new_string").and_then(Value::as_str).map(ToString::to_string),
                    replace_all: args.get("replace_all").and_then(Value::as_bool).unwrap_or(false),
                    absorbed_into: absorbed_into.clone(),
                })?;
                emit_skill_synced(emitter, &session_id, &store, &action, &name, absorbed_into).await;
                Ok(result)
            })
        },
    ))
}

async fn emit_skill_synced(
    emitter: Option<Arc<OutboundEmitter>>,
    session_id: &SessionId,
    store: &skills::SkillStore,
    action: &str,
    name: &str,
    absorbed_into: Option<String>,
) {
    let Some(emitter) = emitter else { return };
    let mut payload = json!({
        "session_id": session_id.as_str(),
        "source": event_source_from_session(session_id),
        "action": action,
        "name": name,
    });
    if action == "delete" {
        payload["deleted"] = Value::Bool(true);
        if let Some(absorbed_into) = absorbed_into {
            payload["absorbed_into"] = Value::String(absorbed_into);
        }
    } else {
        match store.sync_snapshot(name) {
            Ok(snapshot) => {
                if let Some(obj) = snapshot.as_object() {
                    for (key, value) in obj {
                        payload[key] = value.clone();
                    }
                }
            }
            Err(error) => {
                tracing::warn!(%error, skill = %name, "skill sync snapshot failed");
                return;
            }
        }
    }
    emitter
        .emit(OutboundEvent::new(event_types::SKILL_SYNCED, payload))
        .await;
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

fn build_agent_list_description(
    registry: &AgentDefinitionRegistry,
    allowlist: &[String],
) -> String {
    let agents = if allowlist.is_empty() {
        registry.available_agents()
    } else {
        allowlist.to_vec()
    };
    let mut parts = Vec::new();
    for name in &agents {
        let desc = registry.agent_description(name);
        if name == "self" {
            parts.push(format!("{} (Main agent, default)", desc));
        } else {
            parts.push(format!("{} - {}", name, desc));
        }
    }
    format!(
        "Sub-agent name. Default 'self'. Available: {}",
        parts.join(", ")
    )
}

fn delegate_tool(
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
    agent_registry: Arc<AgentDefinitionRegistry>,
    config: DelegateConfig,
) -> Arc<dyn JsonTool> {
    let agent_desc = build_agent_list_description(&agent_registry, &config.agents);
    let agent_names: Vec<String> = if config.agents.is_empty() {
        agent_registry.available_agents()
    } else {
        config.agents.clone()
    };
    let agent_names_clone = agent_names.clone();
    let config_agents = config.agents.clone();

    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "delegate".into(),
            description: "Delegate a task to a sub-agent in an isolated background session.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "goal": {
                        "type": "string",
                        "description": "The task to delegate."
                    },
                    "agent": {
                        "type": "string",
                        "description": agent_desc
                    }
                },
                "required": ["goal", "agent"]
            }),
        },
        move |args| {
            let repo = repo.clone();
            let session_id = session_id.clone();
            let agent_registry = agent_registry.clone();
            let agent_names = agent_names_clone.clone();
            let config_agents = config_agents.clone();
            Box::pin(async move {
                let goal = args
                    .get("goal")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("goal required"))?
                    .to_string();
                let agent_name = args
                    .get("agent")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("agent required"))?
                    .to_string();

                if !config_agents.is_empty() && !config_agents.contains(&agent_name) {
                    return Err(anyhow!(
                        "agent '{}' not in allowlist. Allowed: {:?}",
                        agent_name,
                        config_agents
                    ));
                }
                if agent_registry.resolve(&agent_name).is_none() {
                    return Err(anyhow!(
                        "unknown agent '{}'. Available: {:?}",
                        agent_name,
                        agent_names
                    ));
                }

                let now = Utc::now();
                let id = format!("delegate-{}", now.timestamp_millis());
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
                    agent_name: Some(agent_name),
                    last_result: None,
                };
                repo.create(&job).await?;
                Ok(json!({
                    "job_id": id,
                    "state": "queued",
                    "message": format!(
                        "The subagent is now working. You will be automatically notified once the subagent is done working. If you need to check on its progress, please call the check_delegate_status tool with job id {}.",
                        id
                    )
                }))
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
                    json!({"job_id": job.id, "state": format!("{:?}", job.state), "last_status": job.last_status, "last_error": job.last_error, "result": job.last_result, "session_id": job.delegated_session_id}),
                )
            })
        },
    ))
}

async fn execute_cron(
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
    emitter: Option<Arc<OutboundEmitter>>,
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
                agent_name: None,
                last_result: None,
            };
            repo.create(&job).await?;
            emit_schedule_event(
                emitter.clone(),
                event_types::SCHEDULE_CREATED,
                &job,
                &session_id,
                "tool",
                None,
            )
            .await;
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
            let job = repo
                .get(id)
                .await?
                .ok_or_else(|| anyhow!("job not found"))?;
            repo.delete(id).await?;
            emit_schedule_event(
                emitter.clone(),
                event_types::SCHEDULE_CANCELLED,
                &job,
                &session_id,
                "tool",
                None,
            )
            .await;
            Ok(json!({"cancelled": true, "job_id": id}))
        }
        "pause" => {
            let id = args
                .get("job_id")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("job_id required"))?;
            repo.set_state(id, domain::cron::CronJobState::Paused)
                .await?;
            if let Some(job) = repo.get(id).await? {
                emit_schedule_event(
                    emitter.clone(),
                    event_types::SCHEDULE_PAUSED,
                    &job,
                    &session_id,
                    "tool",
                    None,
                )
                .await;
            }
            Ok(json!({"paused": true, "job_id": id}))
        }
        "resume" => {
            let id = args
                .get("job_id")
                .and_then(Value::as_str)
                .ok_or_else(|| anyhow!("job_id required"))?;
            repo.set_state(id, domain::cron::CronJobState::Active)
                .await?;
            if let Some(job) = repo.get(id).await? {
                emit_schedule_event(
                    emitter.clone(),
                    event_types::SCHEDULE_RESUMED,
                    &job,
                    &session_id,
                    "tool",
                    None,
                )
                .await;
            }
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
            if let Some(interval_seconds) = args.get("interval_seconds").and_then(Value::as_u64) {
                repo.update_interval(id, interval_seconds).await?;
            }
            if let Some(job) = repo.get(id).await? {
                emit_schedule_event(
                    emitter.clone(),
                    event_types::SCHEDULE_UPDATED,
                    &job,
                    &session_id,
                    "tool",
                    None,
                )
                .await;
            }
            Ok(json!({"updated": true, "job_id": id}))
        }
        _ => Err(anyhow!("unknown cron action")),
    }
}

pub async fn emit_schedule_event(
    emitter: Option<Arc<OutboundEmitter>>,
    event_type: &str,
    job: &CronJob,
    session_id: &SessionId,
    origin: &str,
    run: Option<ScheduleRunPayload>,
) {
    if !is_persistent_schedule_job(job) {
        return;
    }
    let Some(emitter) = emitter else { return };
    let mut payload = schedule_payload(job, session_id, origin);
    if let Some(run) = run {
        payload["run_key"] = Value::String(run.run_key);
        payload["scheduled_at"] = Value::String(run.scheduled_at.to_rfc3339());
        if let Some(started_at) = run.started_at {
            payload["started_at"] = Value::String(started_at.to_rfc3339());
        }
        if let Some(completed_at) = run.completed_at {
            payload["completed_at"] = Value::String(completed_at.to_rfc3339());
        }
        if let Some(duration_ms) = run.duration_ms {
            payload["duration_ms"] = Value::from(duration_ms);
        }
        if let Some(error) = run.error {
            payload["error"] = Value::String(error);
        }
    }
    emitter.emit(OutboundEvent::new(event_type, payload)).await;
}

pub struct ScheduleRunPayload {
    pub run_key: String,
    pub scheduled_at: DateTime<Utc>,
    pub started_at: Option<DateTime<Utc>>,
    pub completed_at: Option<DateTime<Utc>>,
    pub duration_ms: Option<i64>,
    pub error: Option<String>,
}

pub fn schedule_run_key(job_id: &str, scheduled_at: DateTime<Utc>) -> String {
    format!("{job_id}:{}", scheduled_at.to_rfc3339())
}

fn is_persistent_schedule_job(job: &CronJob) -> bool {
    match job.source {
        CronJobSource::Cron => job.session_continuation_id.is_none(),
        CronJobSource::Delegate => true,
    }
}

fn schedule_payload(job: &CronJob, session_id: &SessionId, origin: &str) -> Value {
    let mut payload = json!({
        "job_id": job.id,
        "source": cron_source_string(job.source),
        "state": cron_state_string(job.state),
        "channel": job.channel,
        "description": job.description,
        "task_prompt": job.task_prompt,
        "cron_expression": job.cron_expression,
        "interval_seconds": job.interval_seconds,
        "repeat_count": job.repeat_count,
        "repeat_completed": job.repeat_completed,
        "next_run_at": job.next_run_at.to_rfc3339(),
        "last_run_at": job.last_run_at.map(|t| t.to_rfc3339()),
        "last_status": job.last_status,
        "last_error": job.last_error,
        "created_by_session": job.created_by_session,
        "created_at": job.created_at.to_rfc3339(),
        "session_id": session_id.as_str(),
        "origin": origin,
    });
    if let Some(ref agent_name) = job.agent_name {
        payload["agent_name"] = Value::String(agent_name.clone());
    }
    payload
}

fn cron_source_string(source: CronJobSource) -> &'static str {
    match source {
        CronJobSource::Cron => "cron",
        CronJobSource::Delegate => "delegate",
    }
}

fn cron_state_string(state: CronJobState) -> &'static str {
    match state {
        CronJobState::Active => "active",
        CronJobState::Paused => "paused",
        CronJobState::Completed => "completed",
    }
}

fn derive_channel(session_id: &SessionId) -> String {
    session_id
        .as_str()
        .split_once('-')
        .map(|(c, _)| c.to_string())
        .unwrap_or_else(|| session_id.as_str().to_string())
}

#[cfg(test)]
#[path = "rig_tool_registry_tests.rs"]
mod tests;
