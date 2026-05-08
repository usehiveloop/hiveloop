use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::fmt;

/// `SessionId` is the canonical identifier for a session.
///
/// Format for Slack: `"{channel}-{thread_ts}"`. Top-level Slack messages with
/// no `thread_ts` use the message's own `ts` (so each top-level @mention
/// becomes its own session).
#[derive(Debug, Clone, Hash, Eq, PartialEq, Serialize, Deserialize)]
pub struct SessionId(String);

impl SessionId {
    pub fn from_slack(channel: &str, thread_ts: &str) -> Self {
        Self(format!("{channel}-{thread_ts}"))
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }

    pub fn into_inner(self) -> String {
        self.0
    }
}

impl fmt::Display for SessionId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl From<String> for SessionId {
    fn from(s: String) -> Self {
        Self(s)
    }
}

impl From<&str> for SessionId {
    fn from(s: &str) -> Self {
        Self(s.to_owned())
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SessionStatus {
    Active,
    Completed,
    Errored,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Session {
    pub id: SessionId,
    pub channel: String,
    pub thread_ts: String,
    pub adk_session_id: String,
    pub status: SessionStatus,
    pub created_at: DateTime<Utc>,
    pub last_activity_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EventKind {
    UserMessage,
    AssistantMessage,
    ToolCall,
    ToolResult,
    Error,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionEvent {
    pub id: i64,
    pub session_id: SessionId,
    pub seq: i64,
    pub kind: EventKind,
    pub payload: serde_json::Value,
    pub created_at: DateTime<Utc>,
}
