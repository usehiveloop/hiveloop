use bridge_core::event::BridgeEvent;
use reqwest::Client;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;
use storage::StorageHandle;
use tokio::sync::mpsc;

/// Per-worker delivery settings.
#[derive(Clone, Copy)]
pub(super) struct WorkerConfig {
    pub(super) delivery_timeout: Duration,
    pub(super) max_retries: usize,
    pub(super) idle_timeout: Duration,
}

#[allow(clippy::too_many_arguments)]
pub(super) async fn conversation_worker(
    _conversation_id: &str,
    mut rx: mpsc::UnboundedReceiver<BridgeEvent>,
    client: Client,
    semaphore: Arc<tokio::sync::Semaphore>,
    config: WorkerConfig,
    webhook_url: &str,
    webhook_secret: &str,
    storage: Option<StorageHandle>,
    delivered: &AtomicU64,
) {
    loop {
        let first = match tokio::time::timeout(config.idle_timeout, rx.recv()).await {
            Ok(Some(event)) => event,
            Ok(None) => break,
            Err(_) => break, // idle timeout
        };

        let mut batch = vec![first];
        while let Ok(event) = rx.try_recv() {
            batch.push(event);
        }

        let _permit = match semaphore.acquire().await {
            Ok(p) => p,
            Err(_) => break,
        };

        deliver_batch(
            client.clone(),
            batch,
            config.delivery_timeout,
            config.max_retries,
            webhook_url,
            webhook_secret,
            storage.clone(),
            delivered,
        )
        .await;
    }
}

#[allow(clippy::too_many_arguments)]
async fn deliver_batch(
    client: Client,
    batch: Vec<BridgeEvent>,
    timeout: Duration,
    max_retries: usize,
    webhook_url: &str,
    webhook_secret: &str,
    storage: Option<StorageHandle>,
    delivered: &AtomicU64,
) {
    use crate::signer::sign_webhook;
    use backon::{ExponentialBuilder, Retryable};

    let agent_id = batch[0].agent_id.clone();
    let conversation_id = batch[0].conversation_id.clone();
    let batch_size = batch.len();

    let body = match serde_json::to_vec(&batch) {
        Ok(b) => b,
        Err(e) => {
            tracing::error!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                error = %e,
                "event batch serialization failed"
            );
            return;
        }
    };

    let url = webhook_url.to_string();
    let secret = webhook_secret.to_string();
    let start = std::time::Instant::now();
    let attempt = Arc::new(std::sync::atomic::AtomicU32::new(0));

    let result = (|| {
        let client = client.clone();
        let url = url.clone();
        let secret = secret.clone();
        let body = body.clone();
        let attempt = attempt.clone();
        async move {
            let attempt_num = attempt.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1;

            if attempt_num > 1 {
                tracing::warn!(
                    batch_size = batch_size,
                    attempt = attempt_num,
                    "event delivery retrying"
                );
            }

            let timestamp = chrono::Utc::now().timestamp();
            let signature = sign_webhook(&body, &secret, timestamp);
            let response = client
                .post(&url)
                .header("Content-Type", "application/json")
                .header("X-Webhook-Signature", &signature)
                .header("X-Webhook-Timestamp", timestamp.to_string())
                .body(body)
                .timeout(timeout)
                .send()
                .await?;

            let status = response.status();
            if status.is_server_error() {
                return Err(response.error_for_status().unwrap_err());
            }

            tracing::info!(
                batch_size = batch_size,
                status = status.as_u16(),
                latency_ms = start.elapsed().as_millis() as u64,
                "event batch delivered"
            );
            Ok(())
        }
    })
    .retry(
        ExponentialBuilder::default()
            .with_max_times(max_retries)
            .with_jitter(),
    )
    .sleep(tokio::time::sleep)
    .await;

    if let Err(e) = result {
        tracing::error!(
            batch_size = batch_size,
            error = %e,
            "event delivery failed after all retries"
        );
    } else if let Some(storage) = storage {
        for event in &batch {
            storage.mark_webhook_delivered(event.event_id.clone());
        }
        delivered.fetch_add(batch_size as u64, Ordering::Relaxed);
    }
}
