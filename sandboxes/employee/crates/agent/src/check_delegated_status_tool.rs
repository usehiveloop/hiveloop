use std::sync::Arc;

use adk_rust::prelude::*;
use adk_rust::session::{GetRequest, SessionService};
use domain::cron::CronJobSource;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "check_delegated_status";
const TOOL_DESCRIPTION: &str =
    "Check the status of a background delegated task. Returns the task goal, state, \
     last status, and the last 10 conversation messages from the subagent.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct CheckDelegatedStatusArgs {
    pub job_id: String,
}

pub struct CheckDelegatedStatusTool {
    repo: Arc<dyn CronJobRepo>,
    session_service: Arc<dyn SessionService>,
}

impl CheckDelegatedStatusTool {
    pub fn new(repo: Arc<dyn CronJobRepo>, session_service: Arc<dyn SessionService>) -> Self {
        Self { repo, session_service }
    }

    pub fn into_adk_tool(self) -> Arc<dyn Tool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<CheckDelegatedStatusArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: CheckDelegatedStatusArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let job = self.repo.get(&parsed.job_id).await
            .map_err(|e| AdkError::tool(format!("get: {e}")))?
            .ok_or_else(|| AdkError::tool(format!("job '{}' not found", parsed.job_id)))?;
        if job.source != CronJobSource::Delegate {
            return Err(AdkError::tool("this job is not a delegated background task"));
        }
        let goal = job.task_prompt.clone();
        let messages = if let Some(ref sid) = job.delegated_session_id {
            extract_messages(&*self.session_service, sid).await
        } else {
            Vec::new()
        };
        Ok(serde_json::json!({
            "job_id": parsed.job_id,
            "goal": goal,
            "state": state_str(job.state),
            "last_status": job.last_status,
            "last_error": job.last_error,
            "messages": messages,
        }))
    }
}

async fn extract_messages(svc: &dyn SessionService, session_id: &str) -> Vec<Value> {
    let req = GetRequest {
        app_name: "employee-bridge".into(),
        user_id: "runtime".into(),
        session_id: session_id.into(),
        after: None,
        num_recent_events: None,
    };
    let Ok(session) = svc.get(req).await else {
        return Vec::new();
    };
    let events = session.events().all();
    let recent: Vec<_> = events.iter().rev().take(10).collect();
    let mut messages: Vec<Value> = Vec::new();
    for event in recent.into_iter().rev() {
        if let Some(content) = event.llm_response.content.as_ref() {
            let role = if event.author == "user" { "user" } else { "assistant" };
            let text = content.parts.iter().filter_map(|p| match p {
                Part::Text { text } => Some(text.as_str()),
                _ => None,
            }).collect::<Vec<_>>().join(" ");
            if !text.is_empty() {
                let truncated = if text.len() > 500 {
                    format!("{}...", &text[..500])
                } else {
                    text
                };
                messages.push(serde_json::json!({"role": role, "text": truncated}));
            }
        }
    }
    messages
}

fn state_str(state: domain::cron::CronJobState) -> &'static str {
    match state {
        domain::cron::CronJobState::Active => "active",
        domain::cron::CronJobState::Paused => "paused",
        domain::cron::CronJobState::Completed => "completed",
    }
}
