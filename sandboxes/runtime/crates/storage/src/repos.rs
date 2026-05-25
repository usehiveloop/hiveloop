use async_trait::async_trait;
use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobSource, CronJobState};
use domain::{AgentDefinition, EventKind, Session, SessionEvent, SessionId, SessionStatus};
use std::sync::Arc;

#[derive(Debug, thiserror::Error)]
pub enum StorageError {
    #[error("not found")]
    NotFound,
    #[error("conflict")]
    Conflict,
    #[error(transparent)]
    Sqlx(#[from] sqlx::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

pub type Result<T> = std::result::Result<T, StorageError>;

pub trait WriteNotifier: Send + Sync + 'static {
    fn record_write(&self);
}

pub type SharedWriteNotifier = Arc<dyn WriteNotifier>;

pub fn notify_write(notifier: &Option<SharedWriteNotifier>) {
    if let Some(notifier) = notifier {
        notifier.record_write();
    }
}

#[async_trait]
pub trait ConfigRepo: Send + Sync + 'static {
    async fn load(&self) -> Result<Option<AgentDefinition>>;
    async fn upsert(&self, def: &AgentDefinition) -> Result<()>;
}

#[derive(Debug, Clone)]
pub struct SessionListCursor {
    pub last_activity_at: DateTime<Utc>,
    pub id: Option<String>,
}

#[derive(Debug, Clone, Default)]
pub struct SessionListFilter {
    pub cursor: Option<SessionListCursor>,
    pub status: Option<SessionStatus>,
    pub session_id: Option<String>,
    pub channel: Option<String>,
    pub thread_ts: Option<String>,
    pub agent_session_id: Option<String>,
    pub search: Option<String>,
}

#[derive(Debug, Clone)]
pub struct SessionSearchResult {
    pub session_id: String,
    pub event_id: String,
    pub kind: String,
    pub content: String,
    pub snippet: String,
    pub created_at: DateTime<Utc>,
    pub score: f64,
}

#[async_trait]
pub trait SessionRepo: Send + Sync + 'static {
    async fn get(&self, id: &SessionId) -> Result<Option<Session>>;
    async fn create(&self, session: &Session) -> Result<()>;
    async fn touch(&self, id: &SessionId, at: DateTime<Utc>) -> Result<()>;
    async fn set_status(&self, id: &SessionId, status: SessionStatus) -> Result<()>;
    async fn list(&self, filter: SessionListFilter, limit: u32) -> Result<Vec<Session>>;
}

#[async_trait]
pub trait EventRepo: Send + Sync + 'static {
    async fn append(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
    ) -> Result<i64>;
    async fn append_idempotent(
        &self,
        session_id: &SessionId,
        kind: EventKind,
        payload: serde_json::Value,
        idempotency_key: &str,
    ) -> Result<Option<i64>>;
    async fn list_recent(&self, session_id: &SessionId, limit: u32) -> Result<Vec<SessionEvent>>;
    async fn list_chronological(
        &self,
        session_id: &SessionId,
        limit: u32,
    ) -> Result<Vec<SessionEvent>>;
    async fn search_sessions(
        &self,
        query: &str,
        session_id: Option<&SessionId>,
        limit: u32,
    ) -> Result<Vec<SessionSearchResult>>;
}

#[derive(Debug, Clone)]
pub struct OutboxRow {
    pub id: i64,
    pub channel_name: String,
    pub event_type: String,
    pub payload: serde_json::Value,
    pub attempts: i32,
}

#[async_trait]
pub trait OutboxRepo: Send + Sync + 'static {
    async fn enqueue(
        &self,
        channel_name: &str,
        event_type: &str,
        payload: serde_json::Value,
    ) -> Result<i64>;
    async fn claim_due(&self, limit: u32) -> Result<Vec<OutboxRow>>;
    async fn mark_delivered(&self, id: i64) -> Result<()>;
    async fn schedule_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()>;
    async fn mark_failed(&self, id: i64) -> Result<()>;
}

#[async_trait]
pub trait InboundDedupeRepo: Send + Sync + 'static {
    async fn check_and_record(&self, envelope_id: &str) -> Result<bool>;
    async fn cleanup_older_than(&self, before: DateTime<Utc>) -> Result<u64>;
}

#[async_trait]
pub trait CronJobRepo: Send + Sync + 'static {
    async fn create(&self, job: &CronJob) -> Result<()>;
    async fn get(&self, id: &str) -> Result<Option<CronJob>>;
    async fn list_all(&self) -> Result<Vec<CronJob>>;
    async fn list_by_source(&self, source: CronJobSource) -> Result<Vec<CronJob>>;
    async fn list_due(&self) -> Result<Vec<CronJob>>;
    async fn update_prompt(&self, id: &str, task_prompt: String) -> Result<()>;
    async fn update_interval(&self, id: &str, interval_seconds: u64) -> Result<()>;
    async fn update_next_run(&self, id: &str, next_run_at: DateTime<Utc>) -> Result<()>;
    async fn set_state(&self, id: &str, state: CronJobState) -> Result<()>;
    async fn record_run(
        &self,
        id: &str,
        run_at: DateTime<Utc>,
        status: &str,
        error: Option<&str>,
    ) -> Result<()>;
    async fn increment_repeat(&self, id: &str) -> Result<()>;
    async fn delete(&self, id: &str) -> Result<()>;
}
