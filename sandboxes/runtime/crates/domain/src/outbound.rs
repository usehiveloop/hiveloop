use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct OutboundEvent {
    pub event_type: String,
    pub payload: serde_json::Value,
    pub at: DateTime<Utc>,
}

impl OutboundEvent {
    pub fn new(event_type: impl Into<String>, payload: serde_json::Value) -> Self {
        let payload = with_runtime_mode(payload);
        Self {
            event_type: event_type.into(),
            payload,
            at: Utc::now(),
        }
    }
}

fn with_runtime_mode(mut payload: serde_json::Value) -> serde_json::Value {
    let mode = std::env::var("HIVY_RUNTIME_MODE")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| "employee".to_string());
    if let Some(obj) = payload.as_object_mut() {
        obj.entry("mode".to_string())
            .or_insert_with(|| serde_json::Value::String(mode));
    }
    payload
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct OutboundChannelSpec {
    pub name: String,
    #[serde(flatten)]
    pub kind: OutboundChannelKind,
    #[serde(default)]
    pub event_filter: Option<Vec<String>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum OutboundChannelKind {
    Webhook {
        url: String,
        secret_env: String,
        #[serde(default)]
        extra_headers: HashMap<String, String>,
    },
}

pub mod event_types {
    pub const USER_MESSAGE_RECEIVED: &str = "user.message.received";
    pub const SESSION_CREATED: &str = "session.created";
    pub const SESSION_COMPLETED: &str = "session.completed";
    pub const TOOL_INVOKED: &str = "tool.invoked";
    pub const AGENT_MESSAGE_SENT: &str = "agent.message.sent";
    pub const ERROR_TOOL: &str = "error.tool";
    pub const ERROR_MODEL: &str = "error.model";
    pub const CONFIG_APPLIED: &str = "config.applied";
    pub const SKILL_SYNCED: &str = "skill.synced";
    pub const SCHEDULE_CREATED: &str = "schedule.created";
    pub const SCHEDULE_UPDATED: &str = "schedule.updated";
    pub const SCHEDULE_PAUSED: &str = "schedule.paused";
    pub const SCHEDULE_RESUMED: &str = "schedule.resumed";
    pub const SCHEDULE_CANCELLED: &str = "schedule.cancelled";
    pub const SCHEDULE_RUN_STARTED: &str = "schedule.run_started";
    pub const SCHEDULE_RUN_COMPLETED: &str = "schedule.run_completed";
    pub const SCHEDULE_RUN_FAILED: &str = "schedule.run_failed";
}

pub const DATABASE_CHANNEL_NAME: &str = "database";
