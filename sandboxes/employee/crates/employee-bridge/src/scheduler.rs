use std::sync::Arc;
use std::time::Duration;

use chrono::Utc;
use domain::cron::CronJobState;
use domain::{CronJob, InboundEvent, MessageHandle, SessionId};
use storage::CronJobRepo;
use tokio::sync::mpsc;
use tracing::{error, info};

const POLL_INTERVAL_SECONDS: u64 = 5;
const STALE_GRACE_MULTIPLIER: f64 = 0.5;

pub struct CronScheduler {
    repo: Arc<dyn CronJobRepo>,
    inbound_sink: mpsc::Sender<InboundEvent>,
}

impl CronScheduler {
    pub fn new(repo: Arc<dyn CronJobRepo>, inbound_sink: mpsc::Sender<InboundEvent>) -> Self {
        Self { repo, inbound_sink }
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
            .record_run(&job.id, Utc::now(), "running", None)
            .await
        {
            error!(job_id = %job.id, error = %e, "cron: failed to record run start");
        }

        let inbound = InboundEvent {
            envelope_id: envelope_id.clone(),
            session_id: session_id.clone(),
            user: "cron".to_string(),
            user_display_name: Some("Scheduler".to_string()),
            text: job.task_prompt.clone(),
            attachments: Vec::new(),
            raw: serde_json::json!({"source": "cron", "job_id": job.id}),
            inbound_handle: MessageHandle {
                channel: job.channel.clone(),
                ts: String::new(),
            },
            is_direct_message: false,
            is_directly_addressed: true,
            link_previews: Vec::new(),
        };

        info!(
            job_id = %job.id,
            session = %session_id,
            channel = %job.channel,
            "cron: dispatching scheduled task"
        );

        if let Err(e) = self.inbound_sink.send(inbound).await {
            error!(job_id = %job.id, error = %e, "cron: failed to dispatch");
            let _ = self
                .repo
                .record_run(&job.id, Utc::now(), "error", Some(&e.to_string()))
                .await;
            return;
        }

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
