use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use chrono::{DateTime, Utc};
use domain::cron::{CronJob, CronJobState};
use domain::{EventKind, Session, SessionId, SessionStatus};
use serde_json::Value;
use sqlx::SqliteConnection;
use tokio::sync::{mpsc, oneshot};

use crate::repos::{notify_write, Result, SharedWriteNotifier};

use super::cron_ops;
use super::event_ops;
use super::outbox_ops;
use super::session_ops;
use super::EventsLogWrite;

pub(super) const WRITE_QUEUE_CAPACITY: usize = 50_000;

pub(super) type Resp<T> = oneshot::Sender<Result<T>>;

pub(super) enum WriteRequest {
    ConfigUpsert {
        definition_json: String,
        updated_at: String,
        resp: Resp<()>,
    },
    SessionCreate {
        session: Box<Session>,
        resp: Resp<()>,
    },
    SessionTouch {
        id: SessionId,
        at: DateTime<Utc>,
        resp: Resp<()>,
    },
    SessionSetStatus {
        id: SessionId,
        status: SessionStatus,
        resp: Resp<()>,
    },
    EventAppend {
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
        resp: Resp<i64>,
    },
    EventAppendIdempotent {
        session_id: SessionId,
        kind: EventKind,
        payload: Value,
        idempotency_key: String,
        resp: Resp<Option<i64>>,
    },
    InboundDedupeCheckAndRecord {
        envelope_id: String,
        received_at: String,
        resp: Resp<bool>,
    },
    InboundDedupeCleanup {
        before: String,
        resp: Resp<u64>,
    },
    CronCreate {
        job: Box<CronJob>,
        resp: Resp<()>,
    },
    CronUpdatePrompt {
        id: String,
        task_prompt: String,
        resp: Resp<()>,
    },
    CronUpdateInterval {
        id: String,
        interval_seconds: u64,
        resp: Resp<()>,
    },
    CronUpdateNextRun {
        id: String,
        next_run_at: DateTime<Utc>,
        resp: Resp<()>,
    },
    CronSetState {
        id: String,
        state: CronJobState,
        resp: Resp<()>,
    },
    CronRecordRun {
        id: String,
        run_at: DateTime<Utc>,
        status: String,
        error: Option<String>,
        resp: Resp<()>,
    },
    CronIncrementRepeat {
        id: String,
        resp: Resp<()>,
    },
    CronRecordResult {
        id: String,
        result: String,
        resp: Resp<()>,
    },
    CronCompleteDelegateResult {
        id: String,
        completed_at: DateTime<Utc>,
        status: String,
        error: Option<String>,
        result: String,
        resp: Resp<()>,
    },
    CronDelete {
        id: String,
        resp: Resp<()>,
    },
    OutboxEnqueue {
        channel_name: String,
        event_type: String,
        payload_json: String,
        now: String,
        resp: Resp<i64>,
    },
    OutboxMarkDelivered {
        id: i64,
        resp: Resp<()>,
    },
    OutboxScheduleRetry {
        id: i64,
        attempts: i32,
        next_retry_at: DateTime<Utc>,
        resp: Resp<()>,
    },
    OutboxMarkFailed {
        id: i64,
        resp: Resp<()>,
    },
    EventsLogBatch {
        events: Vec<EventsLogWrite>,
        recorded_at: String,
        resp: Resp<()>,
    },
    Flush {
        resp: Resp<()>,
    },
}

pub(super) async fn run_writer(
    mut rx: mpsc::Receiver<WriteRequest>,
    mut conn: SqliteConnection,
    queued: Arc<AtomicUsize>,
    write_notifier: Option<SharedWriteNotifier>,
) {
    while let Some(request) = rx.recv().await {
        queued.fetch_sub(1, Ordering::Relaxed);
        let wrote = request.is_write();
        request.execute(&mut conn).await;
        if wrote {
            notify_write(&write_notifier);
        }
    }
}

impl WriteRequest {
    fn is_write(&self) -> bool {
        !matches!(self, WriteRequest::Flush { .. })
    }

    async fn execute(self, conn: &mut SqliteConnection) {
        match self {
            WriteRequest::ConfigUpsert {
                definition_json,
                updated_at,
                resp,
            } => respond(
                resp,
                session_ops::config_upsert(conn, definition_json, updated_at).await,
            ),
            WriteRequest::SessionCreate { session, resp } => {
                respond(resp, session_ops::session_create(conn, *session).await)
            }
            WriteRequest::SessionTouch { id, at, resp } => {
                respond(resp, session_ops::session_touch(conn, &id, at).await)
            }
            WriteRequest::SessionSetStatus { id, status, resp } => respond(
                resp,
                session_ops::session_set_status(conn, &id, status).await,
            ),
            WriteRequest::EventAppend {
                session_id,
                kind,
                payload,
                resp,
            } => respond(
                resp,
                event_ops::event_append(conn, &session_id, kind, payload).await,
            ),
            WriteRequest::EventAppendIdempotent {
                session_id,
                kind,
                payload,
                idempotency_key,
                resp,
            } => respond(
                resp,
                event_ops::event_append_idempotent(
                    conn,
                    &session_id,
                    kind,
                    payload,
                    &idempotency_key,
                )
                .await,
            ),
            WriteRequest::InboundDedupeCheckAndRecord {
                envelope_id,
                received_at,
                resp,
            } => respond(
                resp,
                session_ops::inbound_dedupe_check_and_record(conn, &envelope_id, &received_at)
                    .await,
            ),
            WriteRequest::InboundDedupeCleanup { before, resp } => respond(
                resp,
                session_ops::inbound_dedupe_cleanup(conn, &before).await,
            ),
            WriteRequest::CronCreate { job, resp } => {
                respond(resp, cron_ops::cron_create(conn, *job).await)
            }
            WriteRequest::CronUpdatePrompt {
                id,
                task_prompt,
                resp,
            } => respond(
                resp,
                cron_ops::cron_update_prompt(conn, &id, &task_prompt).await,
            ),
            WriteRequest::CronUpdateInterval {
                id,
                interval_seconds,
                resp,
            } => respond(
                resp,
                cron_ops::cron_update_interval(conn, &id, interval_seconds).await,
            ),
            WriteRequest::CronUpdateNextRun {
                id,
                next_run_at,
                resp,
            } => respond(
                resp,
                cron_ops::cron_update_next_run(conn, &id, next_run_at).await,
            ),
            WriteRequest::CronSetState { id, state, resp } => {
                respond(resp, cron_ops::cron_set_state(conn, &id, state).await)
            }
            WriteRequest::CronRecordRun {
                id,
                run_at,
                status,
                error,
                resp,
            } => respond(
                resp,
                cron_ops::cron_record_run(conn, &id, run_at, &status, error.as_deref()).await,
            ),
            WriteRequest::CronIncrementRepeat { id, resp } => {
                respond(resp, cron_ops::cron_increment_repeat(conn, &id).await)
            }
            WriteRequest::CronRecordResult { id, result, resp } => {
                respond(resp, cron_ops::cron_record_result(conn, &id, &result).await)
            }
            WriteRequest::CronCompleteDelegateResult {
                id,
                completed_at,
                status,
                error,
                result,
                resp,
            } => respond(
                resp,
                cron_ops::cron_complete_delegate_result(
                    conn,
                    &id,
                    completed_at,
                    &status,
                    error.as_deref(),
                    &result,
                )
                .await,
            ),
            WriteRequest::CronDelete { id, resp } => {
                respond(resp, cron_ops::cron_delete(conn, &id).await)
            }
            WriteRequest::OutboxEnqueue {
                channel_name,
                event_type,
                payload_json,
                now,
                resp,
            } => respond(
                resp,
                outbox_ops::outbox_enqueue(conn, &channel_name, &event_type, &payload_json, &now)
                    .await,
            ),
            WriteRequest::OutboxMarkDelivered { id, resp } => respond(
                resp,
                outbox_ops::outbox_mark_status(conn, id, "delivered").await,
            ),
            WriteRequest::OutboxScheduleRetry {
                id,
                attempts,
                next_retry_at,
                resp,
            } => respond(
                resp,
                outbox_ops::outbox_schedule_retry(conn, id, attempts, next_retry_at).await,
            ),
            WriteRequest::OutboxMarkFailed { id, resp } => respond(
                resp,
                outbox_ops::outbox_mark_status(conn, id, "failed").await,
            ),
            WriteRequest::EventsLogBatch {
                events,
                recorded_at,
                resp,
            } => respond(
                resp,
                outbox_ops::events_log_batch(conn, &events, &recorded_at).await,
            ),
            WriteRequest::Flush { resp } => respond(resp, Ok(())),
        }
    }
}

fn respond<T>(resp: Resp<T>, result: Result<T>) {
    let _ = resp.send(result);
}
