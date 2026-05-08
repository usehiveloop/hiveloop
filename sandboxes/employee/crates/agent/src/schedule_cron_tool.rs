use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use chrono::Utc;
use domain::{CronJob, SessionId};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "schedule_cron";
const TOOL_DESCRIPTION: &str =
    "Schedule a recurring task that wakes up at the specified interval and executes the given prompt. \
     After scheduling, the task starts running after the first interval elapses. \
     The task posts results to the current channel unless a different channel is specified. \
     Provide interval_seconds (e.g. 3600 for hourly, 86400 for daily, 604800 for weekly). \
     Set interval_seconds to 0 for a one-shot task that runs once immediately.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct ScheduleCronArgs {
    pub description: String,
    pub task_prompt: String,
    pub interval_seconds: u64,
    #[serde(default)]
    pub channel_id: Option<String>,
}

pub struct ScheduleCronTool {
    repo: Arc<dyn CronJobRepo>,
    session_id: SessionId,
}

impl ScheduleCronTool {
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
        .with_parameters_schema::<ScheduleCronArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: ScheduleCronArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;

        let channel = parsed
            .channel_id
            .unwrap_or_else(|| derive_channel(&self.session_id));

        let now = Utc::now();
        let next_run_at = if parsed.interval_seconds == 0 {
            now
        } else {
            now + chrono::Duration::seconds(parsed.interval_seconds as i64)
        };

        let job_id = format!("cron-{}", chrono::Utc::now().timestamp_millis());

        let job = CronJob {
            id: job_id.clone(),
            description: parsed.description,
            channel,
            task_prompt: parsed.task_prompt,
            cron_expression: None,
            interval_seconds: Some(parsed.interval_seconds),
            next_run_at,
            created_at: now,
            created_by_session: self.session_id.as_str().to_string(),
        };

        self.repo
            .create(&job)
            .await
            .map_err(|e| AdkError::tool(format!("failed to create cron job: {e}")))?;

        Ok(serde_json::json!({
            "job_id": job_id,
            "next_run_at": next_run_at.to_rfc3339(),
            "interval_seconds": parsed.interval_seconds,
            "channel": job.channel,
        }))
    }
}

fn derive_channel(session_id: &SessionId) -> String {
    session_id
        .as_str()
        .split_once('-')
        .map(|(c, _)| c.to_string())
        .unwrap_or_else(|| session_id.as_str().to_string())
}
