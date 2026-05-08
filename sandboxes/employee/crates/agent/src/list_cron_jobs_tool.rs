use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "list_cron_jobs";
const TOOL_DESCRIPTION: &str =
    "List all scheduled cron jobs with their IDs, descriptions, prompts, intervals, and next run times. \
     Use this to find jobs by description so you can cancel or update them by ID.";

pub struct ListCronJobsTool {
    repo: Arc<dyn CronJobRepo>,
}

impl ListCronJobsTool {
    pub fn new(repo: Arc<dyn CronJobRepo>) -> Self {
        Self { repo }
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, _args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute().await }
        });
        Arc::new(function_tool)
    }

    async fn execute(&self) -> Result<Value, AdkError> {
        let jobs = self
            .repo
            .list_all()
            .await
            .map_err(|e| AdkError::tool(format!("failed to list cron jobs: {e}")))?;
        let job_list: Vec<Value> = jobs
            .into_iter()
            .map(|j| {
                serde_json::json!({
                    "id": j.id,
                    "description": j.description,
                    "channel": j.channel,
                    "task_prompt": j.task_prompt,
                    "interval_seconds": j.interval_seconds,
                    "next_run_at": j.next_run_at.to_rfc3339(),
                })
            })
            .collect();
        Ok(serde_json::json!({ "jobs": job_list, "total": job_list.len() }))
    }
}
