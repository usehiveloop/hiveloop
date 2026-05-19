use std::sync::Arc;

use api::CloudAgentCallbackDeliverer;
use async_trait::async_trait;
use domain::{Reply, SessionId};
use gateway::ChannelGateway;
use serde_json::Value;

pub struct GatewayCloudAgentCallbackDeliverer {
    gateway: Arc<dyn ChannelGateway>,
}

impl GatewayCloudAgentCallbackDeliverer {
    pub fn new(gateway: Arc<dyn ChannelGateway>) -> Self {
        Self { gateway }
    }
}

#[async_trait]
impl CloudAgentCallbackDeliverer for GatewayCloudAgentCallbackDeliverer {
    async fn deliver_cloud_agent_callback(
        &self,
        session_id: &SessionId,
        payload: Value,
    ) -> anyhow::Result<()> {
        self.gateway
            .reply(session_id, Reply::Text(callback_text(&payload)))
            .await?;
        Ok(())
    }
}

fn callback_text(payload: &Value) -> String {
    let event_type = payload
        .get("event_type")
        .and_then(Value::as_str)
        .unwrap_or("cloud_agent_event");
    let task_id = payload.get("task_id").and_then(Value::as_str).unwrap_or("");
    let task_suffix = if task_id.is_empty() {
        String::new()
    } else {
        format!(" for task {task_id}")
    };

    match event_type {
        "conversation_ended" | "ConversationEnded" | "done" | "Done" => {
            format!("Cloud agent finished{task_suffix}.")
        }
        "todo_updated" | "TodoUpdated" => {
            format!("Cloud agent updated the todo list{task_suffix}.")
        }
        _ => format!("Cloud agent sent {event_type}{task_suffix}."),
    }
}
