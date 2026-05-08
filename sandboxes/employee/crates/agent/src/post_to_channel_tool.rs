use std::sync::Arc;

use adk_rust::prelude::{FunctionTool, Tool as AdkTool};
use adk_rust::AdkError;
use domain::Reply;
use gateway::ChannelGateway;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use serde_json::Value;

const TOOL_NAME: &str = "post_to_channel";
const TOOL_DESCRIPTION: &str =
    "Post a message directly to the channel (not in a thread). \
     Use this when running cron/scheduled tasks that don't have a thread context. \
     Provide markdown text for the message.";

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct PostToChannelArgs {
    pub message: String,
}

pub struct PostToChannelTool {
    gateway: Arc<dyn ChannelGateway>,
    channel: String,
}

impl PostToChannelTool {
    pub fn new(gateway: Arc<dyn ChannelGateway>, channel: String) -> Self {
        Self { gateway, channel }
    }

    pub fn into_adk_tool(self) -> Arc<dyn AdkTool> {
        let inner = Arc::new(self);
        let inner_for_closure = inner.clone();
        let function_tool = FunctionTool::new(TOOL_NAME, TOOL_DESCRIPTION, move |_ctx, args| {
            let inner = inner_for_closure.clone();
            async move { inner.execute(args).await }
        })
        .with_parameters_schema::<PostToChannelArgs>();
        Arc::new(function_tool)
    }

    async fn execute(&self, args: Value) -> Result<Value, AdkError> {
        let parsed: PostToChannelArgs = serde_json::from_value(args)
            .map_err(|e| AdkError::tool(format!("invalid arguments: {e}")))?;
        let message = parsed.message.trim();
        if message.is_empty() {
            return Err(AdkError::tool("`message` must not be empty"));
        }
        self.gateway
            .post_to_channel(&self.channel, Reply::Text(message.to_string()))
            .await
            .map_err(|e| AdkError::tool(format!("post to channel failed: {e}")))?;
        Ok(serde_json::json!({ "posted": true, "message": message }))
    }
}
