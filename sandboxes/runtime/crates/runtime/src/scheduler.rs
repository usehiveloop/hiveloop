use std::sync::Arc;
use std::time::Duration;

use agent::rig_tool_registry::{emit_schedule_event, schedule_run_key, ScheduleRunPayload};
use chrono::Utc;
use domain::agent_registry::AgentDefinitionRegistry;
use domain::cron::{CronJobSource, CronJobState};
use domain::event_types;
use domain::{CronJob, InboundEvent, MessageHandle, SessionId};
use outbound::OutboundEmitter;
use storage::CronJobRepo;
use tokio::sync::mpsc;
use tracing::{error, info};

use crate::handler::TurnEventSink;

const POLL_INTERVAL_SECONDS: u64 = 5;
const STALE_GRACE_MULTIPLIER: f64 = 0.5;

pub struct CronScheduler {
    repo: Arc<dyn CronJobRepo>,
    inbound_sink: mpsc::Sender<InboundEvent>,
    emitter: Option<Arc<OutboundEmitter>>,
    agent_registry: Arc<AgentDefinitionRegistry>,
    event_sink: Arc<dyn TurnEventSink>,
}

impl CronScheduler {
    pub fn new(
        repo: Arc<dyn CronJobRepo>,
        inbound_sink: mpsc::Sender<InboundEvent>,
        emitter: Option<Arc<OutboundEmitter>>,
        agent_registry: Arc<AgentDefinitionRegistry>,
        event_sink: Arc<dyn TurnEventSink>,
    ) -> Self {
        Self {
            repo,
            inbound_sink,
            emitter,
            agent_registry,
            event_sink,
        }
    }

    pub async fn run(self) {
        let mut interval = tokio::time::interval(Duration::from_secs(POLL_INTERVAL_SECONDS));
        loop {
            interval.tick().await;
            let due = match self.repo.list_due().await {
                Ok(jobs) => jobs,
                Err(e) => {
                    error!(error = %e, "cron scheduler: list_due failed");
                    continue;
                }
            };
            for job in due {
                if let Some(final_job) = self.fast_forward_if_stale(job) {
                    self.dispatch_job(final_job).await;
                }
            }
        }
    }

    fn fast_forward_if_stale(&self, job: CronJob) -> Option<CronJob> {
        let is_recurring = job.interval_seconds.map(|v| v > 0).unwrap_or(false);
        if !is_recurring {
            return Some(job);
        }
        let interval = job.interval_seconds?;
        let stale_threshold = (interval as f64 * STALE_GRACE_MULTIPLIER).max(120.0) as i64;
        let lag = Utc::now()
            .signed_duration_since(job.next_run_at)
            .num_seconds();
        if lag > stale_threshold * 2 {
            info!(
                job_id = %job.id,
                lag_seconds = lag,
                "cron: fast-forwarding stale recurring job"
            );
            None
        } else {
            Some(job)
        }
    }

    async fn dispatch_job(&self, job: CronJob) {
        let scheduled_at = job.next_run_at;
        let started_at = Utc::now();
        let run_key = schedule_run_key(&job.id, scheduled_at);
        let is_recurring = job.interval_seconds.map(|v| v > 0).unwrap_or(false);
        let is_one_shot = job.interval_seconds == Some(0);
        let is_wake = job.session_continuation_id.is_some();

        if is_recurring && !is_wake {
            let next = Utc::now() + chrono::Duration::seconds(job.interval_seconds.unwrap() as i64);
            if let Err(e) = self.repo.update_next_run(&job.id, next).await {
                error!(job_id = %job.id, error = %e, "cron: failed to advance next_run");
                return;
            }
        }

        let session_id = SessionId::from(
            job.session_continuation_id
                .clone()
                .or_else(|| job.delegated_session_id.clone())
                .unwrap_or_else(|| format!("{}-cron-{}", job.channel, job.id)),
        );
        let envelope_id = format!("cron-{}", Utc::now().timestamp_millis());

        if let Err(e) = self
            .repo
            .record_run(&job.id, started_at, "running", None)
            .await
        {
            error!(job_id = %job.id, error = %e, "cron: failed to record run start");
        }
        let running_job = self
            .repo
            .get(&job.id)
            .await
            .ok()
            .flatten()
            .unwrap_or_else(|| {
                let mut job = job.clone();
                job.last_run_at = Some(started_at);
                job.last_status = Some("running".to_string());
                job
            });
        emit_schedule_event(
            self.emitter.clone(),
            event_types::SCHEDULE_RUN_STARTED,
            &running_job,
            &session_id,
            "scheduler",
            Some(ScheduleRunPayload {
                run_key: run_key.clone(),
                scheduled_at,
                started_at: Some(started_at),
                completed_at: None,
                duration_ms: None,
                error: None,
            }),
        )
        .await;

        let agent_definition = job
            .agent_name
            .as_deref()
            .and_then(|name| self.agent_registry.resolve(name));

        if let Some(ref name) = job.agent_name {
            if agent_definition.is_none() {
                error!(
                    job_id = %job.id,
                    agent_name = %name,
                    "cron: sub-agent not found in registry"
                );
                let _ = self
                    .repo
                    .record_run(
                        &job.id,
                        Utc::now(),
                        "failed",
                        Some(&format!("sub-agent '{}' not found", name)),
                    )
                    .await;
                if job.source == CronJobSource::Delegate {
                    let _ = self.repo.set_state(&job.id, CronJobState::Completed).await;
                }
                return;
            }
        }

        let mut raw = serde_json::json!({
            "source": "cron",
            "job_id": job.id,
            "agent_name": job.agent_name,
            "parent_session_id": job.created_by_session,
            "delegate_goal": job.task_prompt,
        });

        // For delegates with a stream, inject http_stream_id so events flow to the delegate's SSE stream
        if job.source == CronJobSource::Delegate {
            if let Some(ref stream_id) = job.delegate_stream_id {
                raw.as_object_mut()
                    .unwrap()
                    .insert("http_stream_id".to_string(), serde_json::json!(stream_id));

                // Emit subagent_started on the parent's stream
                let stream_url = format!("/gateway/http/streams/{}", stream_id);
                let agent_name = job.agent_name.as_deref().unwrap_or("sub-agent");
                self.event_sink
                    .publish_subagent_started(
                        &job.created_by_session,
                        &job.id,
                        agent_name,
                        &stream_url,
                    )
                    .await;
            }
        }

        let inbound = InboundEvent {
            envelope_id: envelope_id.clone(),
            session_id: session_id.clone(),
            user: "cron".to_string(),
            user_display_name: Some("Scheduler".to_string()),
            text: job.task_prompt.clone(),
            attachments: Vec::new(),
            raw,
            inbound_handle: MessageHandle {
                channel: job.channel.clone(),
                ts: String::new(),
            },
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
            agent_definition,
        };

        info!(
            job_id = %job.id,
            session = %session_id,
            channel = %job.channel,
            "cron: dispatching scheduled task"
        );

        if let Err(e) = self.inbound_sink.send(inbound).await {
            error!(job_id = %job.id, error = %e, "cron: failed to dispatch");
            let failed_at = Utc::now();
            let _ = self
                .repo
                .record_run(&job.id, failed_at, "error", Some(&e.to_string()))
                .await;
            let failed_job = self
                .repo
                .get(&job.id)
                .await
                .ok()
                .flatten()
                .unwrap_or_else(|| {
                    let mut job = running_job.clone();
                    job.last_run_at = Some(failed_at);
                    job.last_status = Some("error".to_string());
                    job.last_error = Some(e.to_string());
                    job
                });
            emit_schedule_event(
                self.emitter.clone(),
                event_types::SCHEDULE_RUN_FAILED,
                &failed_job,
                &session_id,
                "scheduler",
                Some(ScheduleRunPayload {
                    run_key,
                    scheduled_at,
                    started_at: Some(started_at),
                    completed_at: Some(failed_at),
                    duration_ms: Some((failed_at - started_at).num_milliseconds()),
                    error: Some(e.to_string()),
                }),
            )
            .await;
            return;
        }

        let completed_at = Utc::now();
        let completed_job = self
            .repo
            .get(&job.id)
            .await
            .ok()
            .flatten()
            .unwrap_or(running_job);
        // Delegate jobs are completed by the handler after recording the result.
        // Skip SCHEDULE_RUN_COMPLETED and cleanup — the handler manages the lifecycle.
        if job.source == CronJobSource::Delegate {
            return;
        }

        emit_schedule_event(
            self.emitter.clone(),
            event_types::SCHEDULE_RUN_COMPLETED,
            &completed_job,
            &session_id,
            "scheduler",
            Some(ScheduleRunPayload {
                run_key,
                scheduled_at,
                started_at: Some(started_at),
                completed_at: Some(completed_at),
                duration_ms: Some((completed_at - started_at).num_milliseconds()),
                error: None,
            }),
        )
        .await;

        if is_one_shot || is_wake {
            let _ = self.repo.set_state(&job.id, CronJobState::Completed).await;
            let _ = self.repo.delete(&job.id).await;
            info!(job_id = %job.id, "cron: completed, removed");
            return;
        }

        if let Some(repeat_count) = job.repeat_count {
            if let Err(e) = self.repo.increment_repeat(&job.id).await {
                error!(job_id = %job.id, error = %e, "cron: failed to increment repeat");
            }
            if job.repeat_completed + 1 >= repeat_count {
                let _ = self.repo.set_state(&job.id, CronJobState::Completed).await;
                let _ = self.repo.delete(&job.id).await;
                info!(job_id = %job.id, "cron: repeat count reached, completed and removed");
            }
        }
    }
}
