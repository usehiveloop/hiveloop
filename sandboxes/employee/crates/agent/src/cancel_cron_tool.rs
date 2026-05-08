use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use storage::CronJobRepo;

const TOOL_NAME: &str = "cancel_cron";
const TOOL_DESCRIPTION: &str =
    "Cancel a scheduled cron job by its ID. The job will stop executing immediately.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct CancelCronArgs {
    pub job_id: String,
}

pub struct CancelCronTool {
    repo: Arc<dyn CronJobRepo>,
}

impl CancelCronTool {
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
        .with_parameters_schema::<CancelCronArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: CancelCronArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        self.repo
            .delete(&parsed.job_id)
            .await
            .map_err(|e| AdkError::tool(format!("failed to cancel cron job: {e}")))?;
        Ok(serde_json::json!({ "cancelled": true, "job_id": parsed.job_id }))
    }
}
