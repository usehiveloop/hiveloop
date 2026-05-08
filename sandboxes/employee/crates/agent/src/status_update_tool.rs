use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use domain::{Reply, SessionId};
use gateway::ChannelGateway;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;

const TOOL_NAME: &str = "post_status_update";
const TOOL_DESCRIPTION: &str =
    "Post a brief status update to the thread so the user knows what you are working on. \
     Use this during long-running tasks that require multiple tool calls. \
     Provide a short markdown message (3-10 words) describing your current progress.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct PostStatusUpdateArgs {
    pub message: String,
}

pub struct PostStatusUpdateTool {
    gateway: Arc<dyn ChannelGateway>,
    session_id: SessionId,
}

impl PostStatusUpdateTool {
    pub fn new(gateway: Arc<dyn ChannelGateway>, session_id: SessionId) -> Self {
        Self {
            gateway,
            session_id,
        }
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<PostStatusUpdateArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: PostStatusUpdateArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let message = parsed.message.trim();
        if message.is_empty() {
            return Err(AdkError::tool("`message` must not be empty"));
        }
        self.gateway
            .reply(&self.session_id, Reply::Text(message.to_string()))
            .await
            .map_err(|e| AdkError::tool(format!("status update failed: {e}")))?;
        Ok(serde_json::json!({ "posted": true, "message": message }))
    }
}
