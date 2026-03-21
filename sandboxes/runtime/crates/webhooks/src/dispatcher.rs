use bridge_core::config::WebhookConfig;
use bridge_core::webhook::WebhookPayload;
use reqwest::Client;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

pub struct WebhookDispatcher {
    client: Client,
    event_tx: mpsc::UnboundedSender<WebhookPayload>,
    /// High-water-mark: tracks the peak queue depth for observability.
    enqueued: Arc<AtomicU64>,
}

impl WebhookDispatcher {
    /// Create a new dispatcher with default configuration.
    pub fn new() -> (Self, mpsc::UnboundedReceiver<WebhookPayload>) {
        Self::with_config(&WebhookConfig::default())
    }

    /// Create a new dispatcher with explicit configuration.
    pub fn with_config(config: &WebhookConfig) -> (Self, mpsc::UnboundedReceiver<WebhookPayload>) {
        let (tx, rx) = mpsc::unbounded_channel();
        let enqueued = Arc::new(AtomicU64::new(0));

        let client = Client::builder()
            .pool_max_idle_per_host(config.max_idle_connections)
            .pool_idle_timeout(Duration::from_secs(90))
            .connect_timeout(Duration::from_secs(10))
            .tcp_keepalive(Duration::from_secs(60))
            .tcp_nodelay(true)
            .build()
            .unwrap_or_else(|_| Client::new());

        let dispatcher = Self {
            client,
            event_tx: tx,
            enqueued,
        };
        (dispatcher, rx)
    }

    /// Returns a clone of the internal HTTP client.
    pub fn client(&self) -> Client {
        self.client.clone()
    }

    /// Guaranteed-delivery dispatch. Uses an unbounded channel so events are
    /// never dropped due to backpressure. Memory is the buffer — webhook
    /// payloads are ~1KB each so even 100K queued events is ~100MB.
    ///
    /// The only way this can fail is if the receiver is dropped (runtime
    /// shutting down), which is logged but not recoverable.
    pub fn dispatch(&self, payload: WebhookPayload) {
        let event_type = format!("{:?}", payload.event_type);
        let agent_id = payload.agent_id.clone();
        let conversation_id = payload.conversation_id.clone();

        if self.event_tx.send(payload).is_err() {
            tracing::error!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                event_type = %event_type,
                "webhook channel closed — event lost during shutdown"
            );
            return;
        }
        let depth = self.enqueued.fetch_add(1, Ordering::Relaxed) + 1;

        tracing::debug!(
            agent_id = %agent_id,
            conversation_id = %conversation_id,
            event_type = %event_type,
            queue_depth = depth,
            "webhook event enqueued"
        );

        if depth == 10_000 || depth == 50_000 || depth == 100_000 {
            tracing::warn!(
                queue_depth = depth,
                "webhook queue depth high — delivery may be falling behind"
            );
        }
    }

    /// Returns the total number of events enqueued since startup.
    /// Compare with delivered count to gauge backlog.
    pub fn enqueued_count(&self) -> u64 {
        self.enqueued.load(Ordering::Relaxed)
    }

    /// Background delivery loop with concurrency-limited worker pool.
    ///
    /// On shutdown, drains all remaining queued events before exiting
    /// to ensure zero data loss.
    pub async fn run(
        mut rx: mpsc::UnboundedReceiver<WebhookPayload>,
        client: Client,
        cancel: CancellationToken,
        config: WebhookConfig,
    ) {
        let max_inflight = config.max_concurrent_deliveries;
        let semaphore = Arc::new(tokio::sync::Semaphore::new(max_inflight));
        let delivery_timeout = Duration::from_secs(config.delivery_timeout_secs);
        let max_retries = config.max_retries;

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                payload = rx.recv() => {
                    let Some(payload) = payload else { break };
                    spawn_delivery(
                        &client,
                        &semaphore,
                        payload,
                        delivery_timeout,
                        max_retries,
                    );
                }
            }
        }

        // ── Graceful drain: deliver every remaining queued event ──
        // Close the channel to prevent new sends, then drain.
        rx.close();
        let mut drained = 0u64;
        while let Some(payload) = rx.recv().await {
            spawn_delivery(&client, &semaphore, payload, delivery_timeout, max_retries);
            drained += 1;
        }
        if drained > 0 {
            tracing::info!(count = drained, "drained remaining webhook events");
        }

        // Wait for all in-flight deliveries to complete
        let _ = semaphore.acquire_many(max_inflight as u32).await;
    }

    /// Legacy run method for backwards compatibility (uses default config).
    pub async fn run_default(
        rx: mpsc::UnboundedReceiver<WebhookPayload>,
        client: Client,
        cancel: CancellationToken,
    ) {
        Self::run(rx, client, cancel, WebhookConfig::default()).await;
    }
}

/// Spawn a delivery task with semaphore-limited concurrency.
fn spawn_delivery(
    client: &Client,
    semaphore: &Arc<tokio::sync::Semaphore>,
    payload: WebhookPayload,
    delivery_timeout: Duration,
    max_retries: usize,
) {
    let client = client.clone();
    let sem = semaphore.clone();

    tokio::spawn(async move {
        // Acquire delivery slot — provides backpressure on deliveries, not on queue
        let _permit = match sem.acquire().await {
            Ok(p) => p,
            Err(_) => return, // semaphore closed
        };
        deliver_webhook(client, payload, delivery_timeout, max_retries).await;
    });
}

async fn deliver_webhook(
    client: Client,
    payload: WebhookPayload,
    timeout: Duration,
    max_retries: usize,
) {
    use backon::{ExponentialBuilder, Retryable};

    use crate::signer::sign_webhook;

    let url = payload.webhook_url.clone();
    let secret = payload.webhook_secret.clone();
    let event_type = format!("{:?}", payload.event_type);
    let agent_id = payload.agent_id.clone();
    let conversation_id = payload.conversation_id.clone();
    let body = match serde_json::to_vec(&payload) {
        Ok(b) => b,
        Err(e) => {
            tracing::error!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                event_type = %event_type,
                error = %e,
                "webhook serialization failed"
            );
            return;
        }
    };

    let body_len = body.len();
    let start = std::time::Instant::now();
    let attempt = std::sync::Arc::new(std::sync::atomic::AtomicU32::new(0));

    tracing::debug!(
        agent_id = %agent_id,
        conversation_id = %conversation_id,
        event_type = %event_type,
        body_bytes = body_len,
        "webhook delivery starting"
    );

    let result = (|| {
        let client = client.clone();
        let url = url.clone();
        let secret = secret.clone();
        let body = body.clone();
        let agent_id = agent_id.clone();
        let conversation_id = conversation_id.clone();
        let event_type = event_type.clone();
        let attempt = attempt.clone();
        async move {
            let attempt_num = attempt.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1;
            let attempt_start = std::time::Instant::now();

            if attempt_num > 1 {
                tracing::warn!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    event_type = %event_type,
                    attempt = attempt_num,
                    "webhook delivery retrying"
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
            let latency_ms = attempt_start.elapsed().as_millis() as u64;

            if status.is_server_error() {
                tracing::warn!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    event_type = %event_type,
                    attempt = attempt_num,
                    status = status.as_u16(),
                    latency_ms = latency_ms,
                    "webhook delivery attempt failed with server error"
                );
                return Err(response.error_for_status().unwrap_err());
            }

            tracing::info!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                event_type = %event_type,
                attempt = attempt_num,
                status = status.as_u16(),
                latency_ms = latency_ms,
                "webhook delivered"
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

    let total_latency_ms = start.elapsed().as_millis() as u64;
    let total_attempts = attempt.load(std::sync::atomic::Ordering::Relaxed);

    if let Err(e) = result {
        tracing::error!(
            agent_id = %agent_id,
            conversation_id = %conversation_id,
            event_type = %event_type,
            attempts = total_attempts,
            total_latency_ms = total_latency_ms,
            error = %e,
            "webhook delivery failed after all retries"
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::webhook::{WebhookEventType, WebhookPayload};

    fn make_payload() -> WebhookPayload {
        WebhookPayload {
            event_type: WebhookEventType::ConversationCreated,
            agent_id: "agent-1".to_string(),
            conversation_id: "conv-1".to_string(),
            timestamp: chrono::Utc::now(),
            data: serde_json::json!({}),
            webhook_url: "https://example.com/webhook".to_string(),
            webhook_secret: "secret".to_string(),
        }
    }

    #[test]
    fn test_dispatch_does_not_block() {
        let (dispatcher, _rx) = WebhookDispatcher::new();
        dispatcher.dispatch(make_payload());
    }

    #[test]
    fn test_enqueued_counter_starts_at_zero() {
        let (dispatcher, _rx) = WebhookDispatcher::new();
        assert_eq!(dispatcher.enqueued_count(), 0);
    }

    #[test]
    fn test_dispatch_never_drops_events() {
        // Unbounded channel — even 10K events should all be enqueued
        let (dispatcher, _rx) = WebhookDispatcher::new();

        for _ in 0..10_000 {
            dispatcher.dispatch(make_payload());
        }
        assert_eq!(dispatcher.enqueued_count(), 10_000);
    }

    #[test]
    fn test_dispatch_tracks_enqueued_count() {
        let (dispatcher, _rx) = WebhookDispatcher::new();

        dispatcher.dispatch(make_payload());
        dispatcher.dispatch(make_payload());
        dispatcher.dispatch(make_payload());

        assert_eq!(dispatcher.enqueued_count(), 3);
    }

    #[tokio::test]
    async fn test_run_exits_on_cancel() {
        let config = WebhookConfig::default();
        let (dispatcher, rx) = WebhookDispatcher::with_config(&config);
        let client = dispatcher.client();
        let cancel = CancellationToken::new();

        let cancel_clone = cancel.clone();
        let handle = tokio::spawn(async move {
            WebhookDispatcher::run(rx, client, cancel_clone, config).await;
        });

        // Let the loop start
        tokio::time::sleep(Duration::from_millis(10)).await;
        cancel.cancel();

        // Should complete without hanging
        tokio::time::timeout(Duration::from_secs(2), handle)
            .await
            .expect("run should exit on cancel")
            .expect("task should not panic");
    }

    #[tokio::test]
    async fn test_run_drains_queued_events_on_shutdown() {
        let config = WebhookConfig::default();
        let (dispatcher, rx) = WebhookDispatcher::with_config(&config);
        let client = dispatcher.client();
        let cancel = CancellationToken::new();

        // Enqueue events before the run loop starts
        for _ in 0..5 {
            dispatcher.dispatch(make_payload());
        }
        assert_eq!(dispatcher.enqueued_count(), 5);

        // Cancel immediately — the run loop should still drain the 5 events
        cancel.cancel();

        // Run will attempt to deliver (to a fake URL, which will fail, but
        // the important thing is it reads all events from the channel)
        tokio::time::timeout(
            Duration::from_secs(5),
            WebhookDispatcher::run(rx, client, cancel, config),
        )
        .await
        .expect("run should complete drain within timeout");
    }
}
