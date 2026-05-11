use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OutboundEvent {
    pub event_type: String,
    pub payload: serde_json::Value,
    pub at: DateTime<Utc>,
}

impl OutboundEvent {
    pub fn new(event_type: impl Into<String>, payload: serde_json::Value) -> Self {
        Self {
            event_type: event_type.into(),
            payload,
            at: Utc::now(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OutboundChannelSpec {
    pub name: String,
    #[serde(flatten)]
    pub kind: OutboundChannelKind,
    #[serde(default)]
    pub event_filter: Option<Vec<String>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
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
    pub const SUBAGENT_INVOKED: &str = "subagent.invoked";
    pub const ERROR_TOOL: &str = "error.tool";
    pub const ERROR_MODEL: &str = "error.model";
    pub const CONFIG_APPLIED: &str = "config.applied";
}

pub const DATABASE_CHANNEL_NAME: &str = "database";
