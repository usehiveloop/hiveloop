use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "lowercase")]
pub enum CronJobState {
    Active,
    Paused,
    Completed,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
#[serde(rename_all = "lowercase")]
pub enum CronJobSource {
    Cron,
    Delegate,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct CronJob {
    pub id: String,
    pub description: String,
    pub channel: String,
    pub task_prompt: String,
    pub cron_expression: Option<String>,
    pub interval_seconds: Option<u64>,
    pub repeat_count: Option<u32>,
    pub repeat_completed: u32,
    pub state: CronJobState,
    pub source: CronJobSource,
    pub next_run_at: DateTime<Utc>,
    pub last_run_at: Option<DateTime<Utc>>,
    pub last_status: Option<String>,
    pub last_error: Option<String>,
    pub delegated_session_id: Option<String>,
    pub session_continuation_id: Option<String>,
    pub created_at: DateTime<Utc>,
    pub created_by_session: String,
}
