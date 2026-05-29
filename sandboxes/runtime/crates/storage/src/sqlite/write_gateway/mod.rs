mod cron_ops;
mod event_ops;
mod outbox_ops;
mod request;
mod session_ops;

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobState};
use domain::{AgentDefinition, EventKind, Session, SessionId, SessionStatus};
use serde_json::Value;
use sqlx::sqlite::SqliteConnectOptions;
use sqlx::{Connection, SqliteConnection};
use tokio::sync::{mpsc, oneshot};

use crate::repos::{Result, SharedWriteNotifier, StorageError};

use request::{run_writer, WriteRequest, WRITE_QUEUE_CAPACITY};

#[derive(Clone)]
pub struct SqliteWriteGateway {
    tx: mpsc::Sender<WriteRequest>,
    queued: Arc<AtomicUsize>,
}

#[derive(Clone)]
pub struct EventsLogWrite {
    pub event_type: String,
    pub payload_json: String,
    pub occurred_at: String,
}

impl SqliteWriteGateway {
    pub async fn spawn(
        options: SqliteConnectOptions,
        write_notifier: Option<SharedWriteNotifier>,
    ) -> Result<Arc<Self>> {
        let mut conn = SqliteConnection::connect_with(&options)
            .await
            .map_err(StorageError::from)?;
        session_ops::configure_write_connection(&mut conn).await?;
        let (tx, rx) = mpsc::channel(WRITE_QUEUE_CAPACITY);
        let gateway = Arc::new(Self {
            tx,
            queued: Arc::new(AtomicUsize::new(0)),
        });
        tokio::spawn(run_writer(rx, conn, gateway.queued.clone(), write_notifier));
        Ok(gateway)
    }

    pub fn queued_writes(&self) -> usize {
        self.queued.load(Ordering::Relaxed)
    }

    pub async fn upsert_config(&self, definition: &AgentDefinition) -> Result<()> {
        let definition_json = serde_json::to_string(definition)?;
        let updated_at = Utc::now().to_rfc3339();
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::ConfigUpsert {
            definition_json,
            updated_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn create_session(&self, session: Session) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SessionCreate {
            session: Box::new(session),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn touch_session(&self, id: SessionId, at: DateTime<Utc>) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SessionTouch { id, at, resp })
            .await?;
        recv(rx).await
    }

    pub async fn set_session_status(&self, id: SessionId, status: SessionStatus) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::SessionSetStatus { id, status, resp })
            .await?;
        recv(rx).await
    }

    pub async fn append_event(
        &self,
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
    ) -> Result<i64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::EventAppend {
            session_id,
            kind,
            payload,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn append_event_idempotent(
        &self,
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
        idempotency_key: String,
    ) -> Result<Option<i64>> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::EventAppendIdempotent {
            session_id,
            kind,
            payload,
            idempotency_key,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn check_and_record_inbound(
        &self,
        envelope_id: String,
        received_at: String,
    ) -> Result<bool> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::InboundDedupeCheckAndRecord {
            envelope_id,
            received_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn cleanup_inbound_before(&self, before: String) -> Result<u64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::InboundDedupeCleanup { before, resp })
            .await?;
        recv(rx).await
    }

    pub async fn create_cron(&self, job: CronJob) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronCreate {
            job: Box::new(job),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn update_cron_prompt(&self, id: String, task_prompt: String) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronUpdatePrompt {
            id,
            task_prompt,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn update_cron_interval(&self, id: String, interval_seconds: u64) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronUpdateInterval {
            id,
            interval_seconds,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn update_cron_next_run(&self, id: String, next_run_at: DateTime<Utc>) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronUpdateNextRun {
            id,
            next_run_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn set_cron_state(&self, id: String, state: CronJobState) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronSetState { id, state, resp })
            .await?;
        recv(rx).await
    }

    pub async fn record_cron_run(
        &self,
        id: String,
        run_at: DateTime<Utc>,
        status: String,
        error: Option<String>,
    ) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronRecordRun {
            id,
            run_at,
            status,
            error,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn increment_cron_repeat(&self, id: String) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronIncrementRepeat { id, resp })
            .await?;
        recv(rx).await
    }

    pub async fn record_cron_result(&self, id: String, result: String) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronRecordResult { id, result, resp })
            .await?;
        recv(rx).await
    }

    pub async fn complete_delegate_result(
        &self,
        id: String,
        completed_at: DateTime<Utc>,
        status: String,
        error: Option<String>,
        result: String,
    ) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronCompleteDelegateResult {
            id,
            completed_at,
            status,
            error,
            result,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn delete_cron(&self, id: String) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::CronDelete { id, resp }).await?;
        recv(rx).await
    }

    pub async fn enqueue_outbox(
        &self,
        channel_name: String,
        event_type: String,
        payload_json: String,
    ) -> Result<i64> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxEnqueue {
            channel_name,
            event_type,
            payload_json,
            now: Utc::now().to_rfc3339(),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn mark_outbox_delivered(&self, id: i64) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxMarkDelivered { id, resp })
            .await?;
        recv(rx).await
    }

    pub async fn schedule_outbox_retry(
        &self,
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
    ) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxScheduleRetry {
            id,
            attempts,
            next_retry_at,
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn mark_outbox_failed(&self, id: i64) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::OutboxMarkFailed { id, resp })
            .await?;
        recv(rx).await
    }

    pub async fn append_events_log_batch(&self, events: Vec<EventsLogWrite>) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::EventsLogBatch {
            events,
            recorded_at: Utc::now().to_rfc3339(),
            resp,
        })
        .await?;
        recv(rx).await
    }

    pub async fn flush(&self) -> Result<()> {
        let (resp, rx) = oneshot::channel();
        self.send(WriteRequest::Flush { resp }).await?;
        recv(rx).await
    }

    async fn send(&self, request: WriteRequest) -> Result<()> {
        self.queued.fetch_add(1, Ordering::Relaxed);
        if self.tx.send(request).await.is_err() {
            self.queued.fetch_sub(1, Ordering::Relaxed);
            return Err(StorageError::Other(anyhow::anyhow!(
                "sqlite write gateway is closed"
            )));
        }
        Ok(())
    }
}

async fn recv<T>(rx: oneshot::Receiver<Result<T>>) -> Result<T> {
    rx.await.map_err(|_| {
        StorageError::Other(anyhow::anyhow!(
            "sqlite write gateway dropped response before completing write"
        ))
    })?
}
