use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "update_cron";
const TOOL_DESCRIPTION: &str =
    "Update an existing cron job's prompt or interval. Only the fields you provide are changed. \
     The next run time is recalculated from now based on the new interval.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct UpdateCronArgs {
    pub job_id: String,
    #[serde(default)]
    pub task_prompt: Option<String>,
    #[serde(default)]
    pub interval_seconds: Option<u64>,
}

pub struct UpdateCronTool {
    repo: Arc<dyn CronJobRepo>,
}

impl UpdateCronTool {
    pub fn new(repo: Arc<dyn CronJobRepo>) -> Self {
        Self { repo }
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<UpdateCronArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: UpdateCronArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;

        let existing = self
            .repo
            .get(&parsed.job_id)
            .await
            .map_err(|e| AdkError::tool(format!("failed to find cron job: {e}")))?;

        let Some(existing) = existing else {
            return Err(AdkError::tool(format!(
                "cron job `{}` not found",
                parsed.job_id
            )));
        };

        self.repo
            .update(&parsed.job_id, parsed.task_prompt, parsed.interval_seconds)
            .await
            .map_err(|e| AdkError::tool(format!("failed to update cron job: {e}")))?;

        let new_interval = parsed.interval_seconds.unwrap_or(existing.interval_seconds.unwrap_or(0));
        let next_run_at = chrono::Utc::now() + chrono::Duration::seconds(new_interval as i64);
        self.repo
            .update_next_run(&parsed.job_id, next_run_at)
            .await
            .map_err(|e| AdkError::tool(format!("failed to update next run: {e}")))?;

        Ok(serde_json::json!({
            "updated": true,
            "job_id": parsed.job_id,
            "next_run_at": next_run_at.to_rfc3339(),
        }))
    }
}
