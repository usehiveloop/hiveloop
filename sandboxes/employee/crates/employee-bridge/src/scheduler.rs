use std::sync::Arc;
use std::time::Duration;

use domain::{CronJob, InboundEvent, MessageHandle, SessionId};
use storage::CronJobRepo;
use tokio::sync::mpsc;
use tracing::{error, info};

const POLL_INTERVAL_SECONDS: u64 = 5;

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
                self.dispatch_job(job).await;
            }
        }
    }

    async fn dispatch_job(&self, job: CronJob) {
        let session_id = SessionId::from(format!("{}-cron-{}", job.channel, job.id));
        let envelope_id = format!("cron-{}", chrono::Utc::now().timestamp_millis());

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
            return;
        }

        if let Some(interval_secs) = job.interval_seconds {
            if interval_secs > 0 {
                let next = chrono::Utc::now() + chrono::Duration::seconds(interval_secs as i64);
                if let Err(e) = self.repo.update_next_run(&job.id, next).await {
                    error!(job_id = %job.id, error = %e, "cron: failed to update next_run");
                }
            } else {
                let _ = self.repo.delete(&job.id).await;
                info!(job_id = %job.id, "cron: one-shot job completed and removed");
            }
        }
    }
}
