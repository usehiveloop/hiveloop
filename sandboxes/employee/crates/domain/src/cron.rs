use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CronJob {
    pub id: String,
    pub description: String,
    pub channel: String,
    pub task_prompt: String,
    pub cron_expression: Option<String>,
    pub interval_seconds: Option<u64>,
    pub next_run_at: DateTime<Utc>,
    pub created_at: DateTime<Utc>,
    pub created_by_session: String,
}
