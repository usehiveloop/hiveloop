use std::sync::Arc;

use adk_rust::prelude::*;
use chrono::Utc;
use domain::cron::{CronJob, CronJobSource, CronJobState};
use domain::SessionId;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "wake";
const TOOL_DESCRIPTION: &str =
    "Schedule a wake-up reminder in this conversation. After the specified seconds, the agent \
     wakes up in the SAME thread with full conversation history. Use this instead of polling \
     check_bash_status in a loop — schedule a wake to check back later.\n\
     Example: wake(seconds=300, task_prompt=\"check on background build and report results\")";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct WakeArgs {
    pub seconds: u64,
    pub task_prompt: String,
}

pub struct WakeTool {
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
}

impl WakeTool {
    pub fn new(repo: Arc<dyn CronJobRepo>, session_id: SessionId) -> Self {
        Self { repo, session_id }
    }

    pub fn into_adk_tool(self) -> Arc<dyn Tool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<WakeArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value> {
        let parsed: WakeArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let now = Utc::now();
        let job_id = format!("wake-{}", now.timestamp_millis());
        let channel = self.session_id.as_str().split_once('-')
            .map(|(c, _)| c.to_string())
            .unwrap_or_else(|| self.session_id.as_str().to_string());

        let job = CronJob {
            id: job_id.clone(),
            description: parsed.task_prompt.chars().take(80).collect(),
            channel,
            task_prompt: parsed.task_prompt,
            cron_expression: None,
            interval_seconds: Some(parsed.seconds),
            repeat_count: None,
            repeat_completed: 0,
            state: CronJobState::Active,
            source: CronJobSource::Cron,
            next_run_at: now + chrono::Duration::seconds(parsed.seconds as i64),
            last_run_at: None,
            last_status: None,
            last_error: None,
            delegated_session_id: None,
            session_continuation_id: Some(self.session_id.as_str().to_string()),
            created_at: now,
            created_by_session: self.session_id.as_str().to_string(),
        };
        self.repo.create(&job).await
            .map_err(|e| AdkError::tool(format!("create: {e}")))?;
        Ok(serde_json::json!({
            "job_id": job_id,
            "wake_at": job.next_run_at.to_rfc3339(),
            "seconds": parsed.seconds,
        }))
    }
}
