use std::future::Future;
use std::path::PathBuf;
use std::pin::Pin;
use std::sync::Arc;

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobSource, CronJobState};
use domain::{event_types, OutboundEvent, Reply, SessionId, ToolSpec};
use gateway::ChannelGateway;
use mcp::McpRegistry;
use outbound::OutboundEmitter;
use serde_json::{json, Value};
use storage::CronJobRepo;
use tools::{JsonTool, ProcessRegistry, ToolDefinition};

use crate::cloud_agents::CloudAgentService;

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
    pub workspace_root: PathBuf,
    pub cloud_agents: Option<Arc<CloudAgentService>>,
    pub outbound_emitter: Option<Arc<OutboundEmitter>>,
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
            ToolSpec::LoadTools => {
                if let Some(registry) = &ctx.mcp_registry {
                    tools.push(load_tools_tool(registry.clone(), session_id.clone()));
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
            ToolSpec::CloudAgentLaunchTask => {
                if let Some(service) = &ctx.cloud_agents {
                    tools.push(cloud_agent_launch_task_tool(
                        service.clone(),
                        session_id.clone(),
                    ));
                }
            }
            ToolSpec::CloudAgentTaskStatus => {
                if let Some(service) = &ctx.cloud_agents {
                    tools.push(cloud_agent_task_status_tool(service.clone()));
                }
            }
            ToolSpec::CloudAgentListTasks => {
                if let Some(service) = &ctx.cloud_agents {
                    tools.push(cloud_agent_list_tasks_tool(service.clone()));
                }
            }
            ToolSpec::CloudAgentTaskSendMessage => {
                if let Some(service) = &ctx.cloud_agents {
                    tools.push(cloud_agent_task_send_message_tool(service.clone()));
                }
            }
            ToolSpec::CloudAgentTaskTerminate => {
                if let Some(service) = &ctx.cloud_agents {
                    tools.push(cloud_agent_task_terminate_tool(service.clone()));
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
        "slack"
    }
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

fn cloud_agent_launch_task_tool(
    service: Arc<CloudAgentService>,
    session_id: SessionId,
) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cloud_agent_launch_task".into(),
            description: "Launch a task on a cloud agent. Use for implementation-heavy, investigative, or long-running work. The prompt must be a complete standalone task prompt; the description is only short searchable metadata.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "agent_id": {"type": "string", "description": "Cloud agent id."},
                    "description": {"type": "string", "description": "Short searchable task description stored in metadata."},
                    "prompt": {"type": "string", "description": "Full standalone task prompt for the cloud agent."}
                },
                "required": ["agent_id", "description", "prompt"]
            }),
        },
        move |args| {
            let service = service.clone();
            let session_id = session_id.clone();
            Box::pin(async move {
                let agent_id = args
                    .get("agent_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("agent_id required"))?;
                let description = args
                    .get("description")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("description required"))?;
                let prompt = args
                    .get("prompt")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("prompt required"))?;
                service
                    .launch_task(agent_id, description, prompt, &session_id)
                    .await
            })
        },
    ))
}

fn cloud_agent_task_status_tool(service: Arc<CloudAgentService>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cloud_agent_task_status".into(),
            description: "Check a cloud-agent task status and recent events. Status is completed when events show ConversationEnded/done, failed on AgentError, otherwise running.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "task_id": {"type": "string", "description": "Cloud-agent task id."}
                },
                "required": ["task_id"]
            }),
        },
        move |args| {
            let service = service.clone();
            Box::pin(async move {
                let task_id = args
                    .get("task_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("task_id required"))?;
                service.task_status(task_id).await
            })
        },
    ))
}

fn cloud_agent_list_tasks_tool(service: Arc<CloudAgentService>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cloud_agent_list_tasks".into(),
            description: "List recent tasks for a cloud agent and cache task ids so later status/message/terminate calls can resolve the agent.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "agent_id": {"type": "string", "description": "Cloud agent id."}
                },
                "required": ["agent_id"]
            }),
        },
        move |args| {
            let service = service.clone();
            Box::pin(async move {
                let agent_id = args
                    .get("agent_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("agent_id required"))?;
                service.list_tasks(agent_id).await
            })
        },
    ))
}

fn cloud_agent_task_send_message_tool(service: Arc<CloudAgentService>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cloud_agent_task_send_message".into(),
            description:
                "Send feedback, a new request, or an update prompt to a running cloud-agent task."
                    .into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "task_id": {"type": "string", "description": "Cloud-agent task id."},
                    "message": {"type": "string", "description": "Message to send to the cloud-agent task."}
                },
                "required": ["task_id", "message"]
            }),
        },
        move |args| {
            let service = service.clone();
            Box::pin(async move {
                let task_id = args
                    .get("task_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("task_id required"))?;
                let message = args
                    .get("message")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("message required"))?;
                service.send_message(task_id, message).await
            })
        },
    ))
}

fn cloud_agent_task_terminate_tool(service: Arc<CloudAgentService>) -> Arc<dyn JsonTool> {
    Arc::new(DynamicTool::new(
        ToolDefinition {
            name: "cloud_agent_task_terminate".into(),
            description: "Terminate a cloud-agent task with a reason. Use when the task is no longer needed or should stop.".into(),
            parameters: json!({
                "type": "object",
                "properties": {
                    "task_id": {"type": "string", "description": "Cloud-agent task id."},
                    "reason": {"type": "string", "description": "Reason for terminating the task."}
                },
                "required": ["task_id", "reason"]
            }),
        },
        move |args| {
            let service = service.clone();
            Box::pin(async move {
                let task_id = args
                    .get("task_id")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("task_id required"))?;
                let reason = args
                    .get("reason")
                    .and_then(Value::as_str)
                    .ok_or_else(|| anyhow!("reason required"))?;
                service.terminate_task(task_id, reason).await
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
    job.source == CronJobSource::Cron && job.session_continuation_id.is_none()
}

fn schedule_payload(job: &CronJob, session_id: &SessionId, origin: &str) -> Value {
    json!({
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
    })
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
mod tests {
    use super::*;

    use async_trait::async_trait;
    use chrono::{DateTime, Utc};
    use domain::ToolSpec;
    use outbound::{OutboundChannel, OutboundError, OutboundRegistry};
    use std::collections::HashMap;
    use std::fs;
    use std::sync::Mutex;
    use storage::{OutboxRepo, OutboxRow};
    use tokio::sync::RwLock;

    #[derive(Default)]
    struct FakeOutbox {
        rows: Mutex<Vec<(String, String, Value)>>,
    }

    #[derive(Default)]
    struct FakeCronRepo {
        jobs: Mutex<HashMap<String, CronJob>>,
    }

    #[async_trait]
    impl OutboxRepo for FakeOutbox {
        async fn enqueue(
            &self,
            channel_name: &str,
            event_type: &str,
            payload: Value,
        ) -> storage::Result<i64> {
            let mut rows = self.rows.lock().expect("outbox lock");
            rows.push((channel_name.to_string(), event_type.to_string(), payload));
            Ok(rows.len() as i64)
        }

        async fn claim_due(&self, _limit: u32) -> storage::Result<Vec<OutboxRow>> {
            Ok(Vec::new())
        }

        async fn mark_delivered(&self, _id: i64) -> storage::Result<()> {
            Ok(())
        }

        async fn schedule_retry(
            &self,
            _id: i64,
            _attempts: i32,
            _next_retry_at: DateTime<Utc>,
        ) -> storage::Result<()> {
            Ok(())
        }

        async fn mark_failed(&self, _id: i64) -> storage::Result<()> {
            Ok(())
        }
    }

    #[async_trait]
    impl CronJobRepo for FakeCronRepo {
        async fn create(&self, job: &CronJob) -> storage::Result<()> {
            self.jobs
                .lock()
                .expect("cron lock")
                .insert(job.id.clone(), job.clone());
            Ok(())
        }

        async fn get(&self, id: &str) -> storage::Result<Option<CronJob>> {
            Ok(self.jobs.lock().expect("cron lock").get(id).cloned())
        }

        async fn list_all(&self) -> storage::Result<Vec<CronJob>> {
            Ok(self
                .jobs
                .lock()
                .expect("cron lock")
                .values()
                .cloned()
                .collect())
        }

        async fn list_by_source(&self, source: CronJobSource) -> storage::Result<Vec<CronJob>> {
            Ok(self
                .jobs
                .lock()
                .expect("cron lock")
                .values()
                .filter(|job| job.source == source)
                .cloned()
                .collect())
        }

        async fn list_due(&self) -> storage::Result<Vec<CronJob>> {
            Ok(Vec::new())
        }

        async fn update_prompt(&self, id: &str, task_prompt: String) -> storage::Result<()> {
            if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
                job.task_prompt = task_prompt;
            }
            Ok(())
        }

        async fn update_interval(&self, id: &str, interval_seconds: u64) -> storage::Result<()> {
            if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
                job.interval_seconds = Some(interval_seconds);
            }
            Ok(())
        }

        async fn update_next_run(
            &self,
            id: &str,
            next_run_at: DateTime<Utc>,
        ) -> storage::Result<()> {
            if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
                job.next_run_at = next_run_at;
            }
            Ok(())
        }

        async fn set_state(&self, id: &str, state: CronJobState) -> storage::Result<()> {
            if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
                job.state = state;
            }
            Ok(())
        }

        async fn record_run(
            &self,
            id: &str,
            run_at: DateTime<Utc>,
            status: &str,
            error: Option<&str>,
        ) -> storage::Result<()> {
            if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
                job.last_run_at = Some(run_at);
                job.last_status = Some(status.to_string());
                job.last_error = error.map(ToString::to_string);
            }
            Ok(())
        }

        async fn increment_repeat(&self, id: &str) -> storage::Result<()> {
            if let Some(job) = self.jobs.lock().expect("cron lock").get_mut(id) {
                job.repeat_completed += 1;
            }
            Ok(())
        }

        async fn delete(&self, id: &str) -> storage::Result<()> {
            self.jobs.lock().expect("cron lock").remove(id);
            Ok(())
        }
    }

    struct SkillSyncChannel;

    #[async_trait]
    impl OutboundChannel for SkillSyncChannel {
        fn name(&self) -> &str {
            "skill-sync"
        }

        fn kind(&self) -> &'static str {
            "test"
        }

        fn accepts(&self, event_type: &str) -> bool {
            event_type == event_types::SKILL_SYNCED || event_type.starts_with("schedule.")
        }

        async fn deliver(&self, _event: &OutboundEvent) -> outbound::Result<()> {
            Err(OutboundError::Delivery("not used in emitter tests".into()))
        }
    }

    fn temp_workspace() -> PathBuf {
        let path = std::env::temp_dir().join(format!(
            "employee-bridge-skill-sync-{}",
            Utc::now().timestamp_nanos_opt().unwrap_or_default()
        ));
        fs::create_dir_all(&path).expect("create temp workspace");
        path
    }

    fn test_emitter(outbox: Arc<FakeOutbox>) -> Arc<OutboundEmitter> {
        let registry = OutboundRegistry::new().with_channel(Arc::new(SkillSyncChannel));
        Arc::new(OutboundEmitter::new(
            outbox,
            Arc::new(RwLock::new(registry)),
        ))
    }

    fn skill_manage_test_tool(workspace: PathBuf, outbox: Arc<FakeOutbox>) -> Arc<dyn JsonTool> {
        let emitter = test_emitter(outbox);
        let ctx = ToolContext {
            gateway: None,
            cron_repo: None,
            process_registry: None,
            mcp_registry: None,
            workspace_root: workspace,
            cloud_agents: None,
            outbound_emitter: Some(emitter),
        };
        build_agent_tools(
            &[ToolSpec::SkillManage],
            &SessionId::from("C123-456.789"),
            &ctx,
        )
        .into_iter()
        .find(|tool| tool.definition().name == "skill_manage")
        .expect("skill_manage tool")
    }

    #[tokio::test]
    async fn skill_manage_create_emits_complete_sync_snapshot() {
        let workspace = temp_workspace();
        let outbox = Arc::new(FakeOutbox::default());
        let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

        tool.call(json!({
            "action": "create",
            "name": "debug-deploys",
            "category": "engineering",
            "content": "---\nname: debug-deploys\ndescription: Debug deploy failures.\ntags: deploy, debug\n---\n# Debug\nCheck logs first."
        }))
        .await
        .expect("skill create");
        tool.call(json!({
            "action": "write_file",
            "name": "debug-deploys",
            "file_path": "references/errors.md",
            "file_content": "# Errors"
        }))
        .await
        .expect("supporting file write");

        let rows = outbox.rows.lock().expect("outbox lock");
        assert_eq!(rows.len(), 2);
        let (_, event_type, payload) = &rows[1];
        assert_eq!(event_type, event_types::SKILL_SYNCED);
        assert_eq!(payload["action"], "write_file");
        assert_eq!(payload["name"], "debug-deploys");
        assert_eq!(payload["source"], "slack");
        assert_eq!(payload["description"], "Debug deploy failures.");
        assert_eq!(payload["files"]["references/errors.md"], "# Errors");
        assert!(payload["content"]
            .as_str()
            .expect("content string")
            .contains("# Debug"));

        let _ = fs::remove_dir_all(workspace);
    }

    #[tokio::test]
    async fn skill_manage_failed_call_emits_no_sync_event() {
        let workspace = temp_workspace();
        let outbox = Arc::new(FakeOutbox::default());
        let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

        let result = tool
            .call(json!({
                "action": "write_file",
                "name": "missing-skill",
                "file_path": "references/errors.md",
                "content": "# Errors"
            }))
            .await;

        assert!(result.is_err());
        assert!(outbox.rows.lock().expect("outbox lock").is_empty());
        let _ = fs::remove_dir_all(workspace);
    }

    #[tokio::test]
    async fn skill_manage_delete_emits_tombstone() {
        let workspace = temp_workspace();
        let outbox = Arc::new(FakeOutbox::default());
        let tool = skill_manage_test_tool(workspace.clone(), outbox.clone());

        tool.call(json!({
            "action": "create",
            "name": "debug-deploys",
            "content": "---\nname: debug-deploys\n---\n# Debug"
        }))
        .await
        .expect("skill create");
        tool.call(json!({
            "action": "create",
            "name": "deploy-ops",
            "content": "---\nname: deploy-ops\n---\n# Deploy ops"
        }))
        .await
        .expect("absorbed target create");
        tool.call(json!({
            "action": "delete",
            "name": "debug-deploys",
            "absorbed_into": "deploy-ops"
        }))
        .await
        .expect("skill delete");

        let rows = outbox.rows.lock().expect("outbox lock");
        assert_eq!(rows.len(), 3);
        let (_, event_type, payload) = &rows[2];
        assert_eq!(event_type, event_types::SKILL_SYNCED);
        assert_eq!(payload["action"], "delete");
        assert_eq!(payload["deleted"], true);
        assert_eq!(payload["absorbed_into"], "deploy-ops");
        assert!(payload.get("content").is_none());

        let _ = fs::remove_dir_all(workspace);
    }

    #[tokio::test]
    async fn cron_create_update_pause_resume_cancel_emit_schedule_events() {
        let repo = Arc::new(FakeCronRepo::default());
        let outbox = Arc::new(FakeOutbox::default());
        let tool = cron_tool(
            repo,
            SessionId::from("C123-456.789"),
            Some(test_emitter(outbox.clone())),
        );

        let created = tool
            .call(json!({
                "action": "create",
                "task_prompt": "Check deploy health",
                "interval_seconds": 3600,
                "description": "Deploy health"
            }))
            .await
            .expect("create cron");
        let job_id = created["job_id"].as_str().expect("job id").to_string();
        tool.call(json!({"action": "update", "job_id": job_id, "task_prompt": "Check API health", "interval_seconds": 7200}))
            .await
            .expect("update cron");
        tool.call(json!({"action": "pause", "job_id": job_id}))
            .await
            .expect("pause cron");
        tool.call(json!({"action": "resume", "job_id": job_id}))
            .await
            .expect("resume cron");
        tool.call(json!({"action": "cancel", "job_id": job_id}))
            .await
            .expect("cancel cron");

        let rows = outbox.rows.lock().expect("outbox lock");
        let event_types: Vec<_> = rows
            .iter()
            .map(|(_, event_type, _)| event_type.as_str())
            .collect();
        assert_eq!(
            event_types,
            vec![
                event_types::SCHEDULE_CREATED,
                event_types::SCHEDULE_UPDATED,
                event_types::SCHEDULE_PAUSED,
                event_types::SCHEDULE_RESUMED,
                event_types::SCHEDULE_CANCELLED,
            ]
        );
        assert_eq!(rows[0].2["source"], "cron");
        assert_eq!(rows[0].2["origin"], "tool");
        assert_eq!(rows[0].2["task_prompt"], "Check deploy health");
    }

    #[tokio::test]
    async fn wake_jobs_do_not_emit_schedule_events() {
        let now = Utc::now();
        let outbox = Arc::new(FakeOutbox::default());
        let job = CronJob {
            id: "wake-1".to_string(),
            description: "Wake".to_string(),
            channel: "C123".to_string(),
            task_prompt: "Wake up".to_string(),
            cron_expression: None,
            interval_seconds: None,
            repeat_count: Some(1),
            repeat_completed: 0,
            state: CronJobState::Active,
            source: CronJobSource::Cron,
            next_run_at: now,
            last_run_at: None,
            last_status: None,
            last_error: None,
            delegated_session_id: None,
            session_continuation_id: Some("C123-456.789".to_string()),
            created_at: now,
            created_by_session: "C123-456.789".to_string(),
        };
        emit_schedule_event(
            Some(test_emitter(outbox.clone())),
            event_types::SCHEDULE_CREATED,
            &job,
            &SessionId::from("C123-456.789"),
            "tool",
            None,
        )
        .await;
        assert!(outbox.rows.lock().expect("outbox lock").is_empty());
    }
}
