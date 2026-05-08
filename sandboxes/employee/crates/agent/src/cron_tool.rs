use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use chrono::Utc;
use domain::cron::{CronJob, CronJobState};
use domain::SessionId;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "cron";
const TOOL_DESCRIPTION: &str =
    "Manage recurring scheduled cron jobs. Actions: create, list, update, cancel, pause, resume.\n\
     Use interval_seconds for schedule (e.g. 3600=hourly, 86400=daily). Set repeat_count to auto-delete after N runs.\n\
     The job posts to the current channel unless channel_id is specified.\n\
     For one-shot wake-up reminders in the same conversation, use the wake tool instead.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct CronArgs {
    pub action: String,
    #[serde(default)]
    pub job_id: Option<String>,
    #[serde(default)]
    pub task_prompt: Option<String>,
    #[serde(default)]
    pub interval_seconds: Option<u64>,
    #[serde(default)]
    pub description: Option<String>,
    #[serde(default)]
    pub repeat_count: Option<u32>,
    #[serde(default)]
    pub channel_id: Option<String>,
}

pub struct CronTool {
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
}

impl CronTool {
    pub fn new(repo: Arc<dyn CronJobRepo>, session_id: SessionId) -> Self {
        Self { repo, session_id }
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<CronArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: CronArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        match parsed.action.as_str() {
            "create" => self.do_create(parsed).await,
            "list" => self.do_list().await,
            "update" => self.do_update(parsed).await,
            "cancel" => self.do_cancel(parsed).await,
            "pause" => self.do_pause(parsed).await,
            "resume" => self.do_resume(parsed).await,
            _ => Err(AdkError::tool(format!(
                "unknown action '{}'. valid: create, list, update, cancel, pause, resume",
                parsed.action
            ))),
        }
    }

    async fn do_create(&self, args: CronArgs) -> Result<Value, AdkError> {
        let task_prompt = args.task_prompt.ok_or_else(|| AdkError::tool("task_prompt required for create"))?;
        let description = args.description.unwrap_or_else(|| task_prompt.chars().take(80).collect());
        let interval_seconds = args.interval_seconds
            .ok_or_else(|| AdkError::tool("interval_seconds required for create"))?;
        let channel = args.channel_id.unwrap_or_else(|| derive_channel(&self.session_id));
        let now = Utc::now();
        let next_run_at = if interval_seconds == 0 {
            now
        } else {
            now + chrono::Duration::seconds(interval_seconds as i64)
        };
        let job_id = format!("cron-{}", now.timestamp_millis());
        let job = CronJob {
            id: job_id.clone(),
            description,
            channel,
            task_prompt,
            cron_expression: None,
            interval_seconds: Some(interval_seconds),
            repeat_count: args.repeat_count,
            repeat_completed: 0,
            state: CronJobState::Active,
            source: domain::cron::CronJobSource::Cron,
            next_run_at,
            last_run_at: None,
            last_status: None,
            last_error: None,
            delegated_session_id: None,
            session_continuation_id: None,
            created_at: now,
            created_by_session: self.session_id.as_str().to_string(),
        };
        self.repo.create(&job).await.map_err(|e| AdkError::tool(format!("create: {e}")))?;
        Ok(serde_json::json!({
            "job_id": job_id, "next_run_at": next_run_at.to_rfc3339(),
            "interval_seconds": interval_seconds, "channel": job.channel, "repeat_count": args.repeat_count,
        }))
    }

    async fn do_list(&self) -> Result<Value, AdkError> {
        let jobs = self.repo.list_by_source(domain::cron::CronJobSource::Cron).await
            .map_err(|e| AdkError::tool(format!("list: {e}")))?;
        let list: Vec<Value> = jobs.into_iter().map(|j| serde_json::json!({
            "id": j.id, "description": j.description, "state": state_str(j.state),
            "channel": j.channel, "task_prompt": j.task_prompt,
            "interval_seconds": j.interval_seconds, "repeat_count": j.repeat_count,
            "repeat_completed": j.repeat_completed, "next_run_at": j.next_run_at.to_rfc3339(),
            "last_run_at": j.last_run_at.map(|t| t.to_rfc3339()),
            "last_status": j.last_status, "last_error": j.last_error,
        })).collect();
        Ok(serde_json::json!({"jobs": list, "total": list.len()}))
    }

    async fn do_update(&self, args: CronArgs) -> Result<Value, AdkError> {
        let job_id = args.job_id.ok_or_else(|| AdkError::tool("job_id required for update"))?;
        require_cron_source(&*self.repo, &job_id).await?;
        if let Some(prompt) = args.task_prompt {
            self.repo.update_prompt(&job_id, prompt).await.map_err(|e| AdkError::tool(format!("update: {e}")))?;
        }
        if let Some(interval) = args.interval_seconds {
            self.repo.update_interval(&job_id, interval).await.map_err(|e| AdkError::tool(format!("update: {e}")))?;
            let next = Utc::now() + chrono::Duration::seconds(interval as i64);
            self.repo.update_next_run(&job_id, next).await.map_err(|e| AdkError::tool(format!("update: {e}")))?;
        }
        Ok(serde_json::json!({"updated": true, "job_id": job_id}))
    }

    async fn do_cancel(&self, args: CronArgs) -> Result<Value, AdkError> {
        let job_id = args.job_id.ok_or_else(|| AdkError::tool("job_id required for cancel"))?;
        require_cron_source(&*self.repo, &job_id).await?;
        self.repo.delete(&job_id).await.map_err(|e| AdkError::tool(format!("cancel: {e}")))?;
        Ok(serde_json::json!({"cancelled": true, "job_id": job_id}))
    }

    async fn do_pause(&self, args: CronArgs) -> Result<Value, AdkError> {
        let job_id = args.job_id.ok_or_else(|| AdkError::tool("job_id required for pause"))?;
        require_cron_source(&*self.repo, &job_id).await?;
        self.repo.set_state(&job_id, CronJobState::Paused).await.map_err(|e| AdkError::tool(format!("pause: {e}")))?;
        Ok(serde_json::json!({"paused": true, "job_id": job_id}))
    }

    async fn do_resume(&self, args: CronArgs) -> Result<Value, AdkError> {
        let job_id = args.job_id.ok_or_else(|| AdkError::tool("job_id required for resume"))?;
        require_cron_source(&*self.repo, &job_id).await?;
        let interval = self.repo.get(&job_id).await.map_err(|e| AdkError::tool(format!("get: {e}")))?
            .and_then(|j| j.interval_seconds).unwrap_or(3600);
        let next = Utc::now() + chrono::Duration::seconds(interval as i64);
        self.repo.update_next_run(&job_id, next).await.map_err(|e| AdkError::tool(format!("resume: {e}")))?;
        self.repo.set_state(&job_id, CronJobState::Active).await.map_err(|e| AdkError::tool(format!("resume: {e}")))?;
        Ok(serde_json::json!({"resumed": true, "job_id": job_id, "next_run_at": next.to_rfc3339()}))
    }
}

async fn require_cron_source(repo: &dyn CronJobRepo, id: &str) -> Result<(), AdkError> {
    let job = repo.get(id).await.map_err(|e| AdkError::tool(format!("get: {e}")))?
        .ok_or_else(|| AdkError::tool(format!("cron job '{}' not found", id)))?;
    if job.source != domain::cron::CronJobSource::Cron {
        return Err(AdkError::tool("delegated background tasks cannot be managed via cron tool. Use check_delegated_status instead."));
    }
    Ok(())
}

fn derive_channel(session_id: &SessionId) -> String {
    session_id.as_str().split_once('-').map(|(c, _)| c.to_string()).unwrap_or_else(|| session_id.as_str().to_string())
}

fn state_str(state: CronJobState) -> &'static str {
    match state {
        CronJobState::Active => "active",
        CronJobState::Paused => "paused",
        CronJobState::Completed => "completed",
    }
}
