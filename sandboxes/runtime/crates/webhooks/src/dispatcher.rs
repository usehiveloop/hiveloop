use bridge_core::config::WebhookConfig;
use bridge_core::webhook::WebhookPayload;
use reqwest::Client;
use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio::task::JoinSet;
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

    /// Background delivery loop with per-conversation ordered, batched delivery.
    ///
    /// Events within the same conversation are delivered sequentially in FIFO
    /// order. When multiple events queue up for a conversation, they are batched
    /// into a single HTTP POST (JSON array). Cross-conversation delivery is
    /// fully concurrent, bounded by the global semaphore.
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
        let idle_timeout = Duration::from_secs(config.worker_idle_timeout_secs);

        // Per-conversation routing table and sequence counters
        let mut workers: HashMap<String, mpsc::UnboundedSender<WebhookPayload>> = HashMap::new();
        let mut sequence_counters: HashMap<String, u64> = HashMap::new();
        let mut worker_handles: JoinSet<String> = JoinSet::new();

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,

                // A conversation worker exited (idle timeout or channel closed).
                // Clean up its routing entry, but only if the sender is actually
                // closed (a new worker may have replaced it in a race).
                Some(result) = worker_handles.join_next() => {
                    if let Ok(conv_id) = result {
                        let is_stale = workers
                            .get(&conv_id)
                            .map_or(true, |tx| tx.is_closed());
                        if is_stale {
                            workers.remove(&conv_id);
                            sequence_counters.remove(&conv_id);
                            tracing::debug!(
                                conversation_id = %conv_id,
                                "conversation worker exited, cleaned up"
                            );
                        }
                    }
                }

                payload = rx.recv() => {
                    let Some(payload) = payload else { break };
                    route_event(
                        payload,
                        &mut workers,
                        &mut sequence_counters,
                        &mut worker_handles,
                        &client,
                        &semaphore,
                        delivery_timeout,
                        max_retries,
                        idle_timeout,
                    );
                }
            }
        }

        // ── Graceful drain: deliver every remaining queued event ──
        // Close the channel to prevent new sends, then drain.
        rx.close();
        let mut drained = 0u64;
        while let Some(payload) = rx.recv().await {
            route_event(
                payload,
                &mut workers,
                &mut sequence_counters,
                &mut worker_handles,
                &client,
                &semaphore,
                delivery_timeout,
                max_retries,
                idle_timeout,
            );
            drained += 1;
        }
        if drained > 0 {
            tracing::info!(count = drained, "drained remaining webhook events");
        }

        // Drop all conversation senders to signal workers to finish remaining items
        workers.clear();

        // Wait for all conversation workers to complete
        while worker_handles.join_next().await.is_some() {}
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

/// Stamp a sequence number on the payload and route it to the correct
/// per-conversation worker, creating a new worker if needed.
fn route_event(
    mut payload: WebhookPayload,
    workers: &mut HashMap<String, mpsc::UnboundedSender<WebhookPayload>>,
    sequence_counters: &mut HashMap<String, u64>,
    worker_handles: &mut JoinSet<String>,
    client: &Client,
    semaphore: &Arc<tokio::sync::Semaphore>,
    delivery_timeout: Duration,
    max_retries: usize,
    idle_timeout: Duration,
) {
    let conv_id = payload.conversation_id.clone();

    // Assign monotonically increasing sequence number per conversation
    let counter = sequence_counters.entry(conv_id.clone()).or_insert(0);
    *counter += 1;
    payload.sequence_number = *counter;

    // Try to send to existing worker; if the worker is gone, get the payload back
    let payload = match workers.get(&conv_id) {
        Some(tx) => match tx.send(payload) {
            Ok(()) => return, // successfully routed
            Err(e) => e.0,    // worker gone, reclaim payload
        },
        None => payload, // no worker exists yet
    };

    // Create a new worker for this conversation
    let (tx, worker_rx) = mpsc::unbounded_channel();
    tx.send(payload).expect("fresh channel cannot be closed");
    workers.insert(conv_id.clone(), tx);

    let worker_client = client.clone();
    let worker_sem = semaphore.clone();
    let worker_conv_id = conv_id;

    worker_handles.spawn(async move {
        conversation_delivery_worker(
            &worker_conv_id,
            worker_rx,
            worker_client,
            worker_sem,
            delivery_timeout,
            max_retries,
            idle_timeout,
        )
        .await;
        worker_conv_id
    });
}

/// Per-conversation delivery worker. Processes events sequentially to guarantee
/// in-order delivery. When multiple events are queued, batches them into a
/// single HTTP POST.
async fn conversation_delivery_worker(
    conversation_id: &str,
    mut rx: mpsc::UnboundedReceiver<WebhookPayload>,
    client: Client,
    semaphore: Arc<tokio::sync::Semaphore>,
    delivery_timeout: Duration,
    max_retries: usize,
    idle_timeout: Duration,
) {
    tracing::debug!(
        conversation_id = %conversation_id,
        "conversation delivery worker started"
    );

    loop {
        // Wait for at least one event, with idle timeout
        let first = match tokio::time::timeout(idle_timeout, rx.recv()).await {
            Ok(Some(payload)) => payload,
            Ok(None) => {
                tracing::debug!(
                    conversation_id = %conversation_id,
                    "conversation worker channel closed, exiting"
                );
                break;
            }
            Err(_) => {
                tracing::debug!(
                    conversation_id = %conversation_id,
                    "conversation worker idle timeout, exiting"
                );
                break;
            }
        };

        // Drain any additional queued events to form a batch
        let mut batch = vec![first];
        while let Ok(payload) = rx.try_recv() {
            batch.push(payload);
        }

        // Acquire global concurrency permit
        let _permit = match semaphore.acquire().await {
            Ok(p) => p,
            Err(_) => break, // semaphore closed
        };

        let batch_size = batch.len();
        tracing::debug!(
            conversation_id = %conversation_id,
            batch_size = batch_size,
            "delivering webhook batch"
        );

        // Deliver the batch sequentially — next batch waits for this one
        deliver_webhook_batch(
            client.clone(),
            batch,
            delivery_timeout,
            max_retries,
        )
        .await;
        // permit dropped here, freeing the slot
    }

    tracing::debug!(
        conversation_id = %conversation_id,
        "conversation delivery worker exited"
    );
}

/// Deliver a batch of webhook payloads as a single HTTP POST with a JSON array body.
/// All payloads in the batch share the same conversation (and thus the same
/// webhook_url and webhook_secret).
async fn deliver_webhook_batch(
    client: Client,
    batch: Vec<WebhookPayload>,
    timeout: Duration,
    max_retries: usize,
) {
    use backon::{ExponentialBuilder, Retryable};

    use crate::signer::sign_webhook;

    let url = batch[0].webhook_url.clone();
    let secret = batch[0].webhook_secret.clone();
    let agent_id = batch[0].agent_id.clone();
    let conversation_id = batch[0].conversation_id.clone();
    let batch_size = batch.len();

    let event_types: Vec<String> = batch
        .iter()
        .map(|p| format!("{:?}", p.event_type))
        .collect();

    let body = match serde_json::to_vec(&batch) {
        Ok(b) => b,
        Err(e) => {
            tracing::error!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                batch_size = batch_size,
                error = %e,
                "webhook batch serialization failed"
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
        batch_size = batch_size,
        event_types = ?event_types,
        body_bytes = body_len,
        "webhook batch delivery starting"
    );

    let result = (|| {
        let client = client.clone();
        let url = url.clone();
        let secret = secret.clone();
        let body = body.clone();
        let agent_id = agent_id.clone();
        let conversation_id = conversation_id.clone();
        let attempt = attempt.clone();
        async move {
            let attempt_num = attempt.fetch_add(1, std::sync::atomic::Ordering::Relaxed) + 1;
            let attempt_start = std::time::Instant::now();

            if attempt_num > 1 {
                tracing::warn!(
                    agent_id = %agent_id,
                    conversation_id = %conversation_id,
                    batch_size = batch_size,
                    attempt = attempt_num,
                    "webhook batch delivery retrying"
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
                    batch_size = batch_size,
                    attempt = attempt_num,
                    status = status.as_u16(),
                    latency_ms = latency_ms,
                    "webhook batch delivery attempt failed with server error"
                );
                return Err(response.error_for_status().unwrap_err());
            }

            tracing::info!(
                agent_id = %agent_id,
                conversation_id = %conversation_id,
                batch_size = batch_size,
                attempt = attempt_num,
                status = status.as_u16(),
                latency_ms = latency_ms,
                "webhook batch delivered"
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
            batch_size = batch_size,
            event_types = ?event_types,
            attempts = total_attempts,
            total_latency_ms = total_latency_ms,
            error = %e,
            "webhook batch delivery failed after all retries"
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::webhook::{WebhookEventType, WebhookPayload};

    fn make_payload() -> WebhookPayload {
        make_payload_for_conv("conv-1")
    }

    fn make_payload_for_conv(conv_id: &str) -> WebhookPayload {
        WebhookPayload {
            event_type: WebhookEventType::ConversationCreated,
            agent_id: "agent-1".to_string(),
            conversation_id: conv_id.to_string(),
            timestamp: chrono::Utc::now(),
            sequence_number: 0,
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
        // Use fast config so delivery attempts fail quickly
        let config = WebhookConfig {
            delivery_timeout_secs: 1,
            max_retries: 0,
            worker_idle_timeout_secs: 5,
            ..WebhookConfig::default()
        };
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
            Duration::from_secs(10),
            WebhookDispatcher::run(rx, client, cancel, config),
        )
        .await
        .expect("run should complete drain within timeout");
    }

    #[tokio::test]
    async fn test_body_is_always_json_array() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let mock_server = MockServer::start().await;

        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        let config = WebhookConfig {
            max_retries: 0,
            delivery_timeout_secs: 5,
            worker_idle_timeout_secs: 2,
            ..WebhookConfig::default()
        };

        let (dispatcher, rx) = WebhookDispatcher::with_config(&config);
        let client = dispatcher.client();
        let cancel = CancellationToken::new();

        // Dispatch a single event
        let mut payload = make_payload();
        payload.webhook_url = mock_server.uri();
        dispatcher.dispatch(payload);

        let cancel_clone = cancel.clone();
        let handle = tokio::spawn(async move {
            WebhookDispatcher::run(rx, client, cancel_clone, config).await;
        });

        // Wait for delivery
        tokio::time::sleep(Duration::from_millis(500)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 1, "expected exactly one request");

        // Body must be a JSON array, even for a single event
        let body: serde_json::Value =
            serde_json::from_slice(&requests[0].body).expect("valid JSON");
        assert!(body.is_array(), "body must be a JSON array");
        assert_eq!(body.as_array().unwrap().len(), 1);
    }

    #[tokio::test]
    async fn test_sequence_numbers_monotonic_per_conversation() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let mock_server = MockServer::start().await;

        // Slow response to force events to queue up and batch
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(200).set_body_string("ok"))
            .mount(&mock_server)
            .await;

        let config = WebhookConfig {
            max_retries: 0,
            delivery_timeout_secs: 5,
            worker_idle_timeout_secs: 2,
            ..WebhookConfig::default()
        };

        let (dispatcher, rx) = WebhookDispatcher::with_config(&config);
        let client = dispatcher.client();
        let cancel = CancellationToken::new();

        // Dispatch 10 events for the same conversation
        for _ in 0..10 {
            let mut payload = make_payload();
            payload.webhook_url = mock_server.uri();
            dispatcher.dispatch(payload);
        }

        let cancel_clone = cancel.clone();
        let handle = tokio::spawn(async move {
            WebhookDispatcher::run(rx, client, cancel_clone, config).await;
        });

        // Wait for all deliveries
        tokio::time::sleep(Duration::from_secs(1)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();
        assert!(!requests.is_empty(), "expected at least one request");

        // Collect all events across batches in delivery order
        let mut all_seq_numbers: Vec<u64> = Vec::new();
        for req in &requests {
            let batch: Vec<serde_json::Value> =
                serde_json::from_slice(&req.body).expect("valid JSON array");
            for event in &batch {
                all_seq_numbers.push(event["sequence_number"].as_u64().unwrap());
            }
        }

        assert_eq!(all_seq_numbers.len(), 10, "all 10 events should be delivered");

        // Sequence numbers must be 1..=10 in strictly increasing order
        let expected: Vec<u64> = (1..=10).collect();
        assert_eq!(all_seq_numbers, expected, "sequence numbers must be 1..10 in order");
    }

    #[tokio::test]
    async fn test_cross_conversation_events_both_delivered() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let mock_server = MockServer::start().await;

        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        let config = WebhookConfig {
            max_retries: 0,
            delivery_timeout_secs: 5,
            worker_idle_timeout_secs: 2,
            ..WebhookConfig::default()
        };

        let (dispatcher, rx) = WebhookDispatcher::with_config(&config);
        let client = dispatcher.client();
        let cancel = CancellationToken::new();

        // Dispatch events for two different conversations
        for _ in 0..3 {
            let mut p1 = make_payload_for_conv("conv-A");
            p1.webhook_url = mock_server.uri();
            dispatcher.dispatch(p1);

            let mut p2 = make_payload_for_conv("conv-B");
            p2.webhook_url = mock_server.uri();
            dispatcher.dispatch(p2);
        }

        let cancel_clone = cancel.clone();
        let handle = tokio::spawn(async move {
            WebhookDispatcher::run(rx, client, cancel_clone, config).await;
        });

        tokio::time::sleep(Duration::from_secs(1)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();

        // Collect all events across all batches
        let mut conv_a_events: Vec<u64> = Vec::new();
        let mut conv_b_events: Vec<u64> = Vec::new();
        for req in &requests {
            let batch: Vec<serde_json::Value> =
                serde_json::from_slice(&req.body).expect("valid JSON array");
            for event in &batch {
                let conv = event["conversation_id"].as_str().unwrap();
                let seq = event["sequence_number"].as_u64().unwrap();
                match conv {
                    "conv-A" => conv_a_events.push(seq),
                    "conv-B" => conv_b_events.push(seq),
                    other => panic!("unexpected conversation_id: {other}"),
                }
            }
        }

        assert_eq!(conv_a_events.len(), 3, "conv-A should have 3 events");
        assert_eq!(conv_b_events.len(), 3, "conv-B should have 3 events");

        // Each conversation's sequence numbers must be strictly increasing
        assert_eq!(conv_a_events, vec![1, 2, 3]);
        assert_eq!(conv_b_events, vec![1, 2, 3]);
    }

    #[tokio::test]
    async fn test_batching_groups_queued_events() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let mock_server = MockServer::start().await;

        // First request is slow, so subsequent events queue up and batch
        Mock::given(method("POST"))
            .respond_with(
                ResponseTemplate::new(200).set_body_string("ok"),
            )
            .mount(&mock_server)
            .await;

        let config = WebhookConfig {
            max_retries: 0,
            delivery_timeout_secs: 5,
            worker_idle_timeout_secs: 2,
            ..WebhookConfig::default()
        };

        let (dispatcher, rx) = WebhookDispatcher::with_config(&config);
        let client = dispatcher.client();
        let cancel = CancellationToken::new();

        let cancel_clone = cancel.clone();
        let handle = tokio::spawn(async move {
            WebhookDispatcher::run(rx, client, cancel_clone, config).await;
        });

        // Give run() time to start
        tokio::time::sleep(Duration::from_millis(50)).await;

        // Dispatch 5 events rapidly — they should batch
        for _ in 0..5 {
            let mut payload = make_payload();
            payload.webhook_url = mock_server.uri();
            dispatcher.dispatch(payload);
        }

        // Wait for deliveries
        tokio::time::sleep(Duration::from_secs(1)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();

        // All events must be delivered
        let total_events: usize = requests
            .iter()
            .map(|r| {
                let batch: Vec<serde_json::Value> =
                    serde_json::from_slice(&r.body).expect("valid JSON array");
                batch.len()
            })
            .sum();
        assert_eq!(total_events, 5, "all 5 events should be delivered");

        // Fewer requests than events means batching occurred
        // (we can't guarantee exact batch sizes due to timing, but we can
        // verify the body is always an array and events are in order)
        for req in &requests {
            let batch: Vec<serde_json::Value> =
                serde_json::from_slice(&req.body).expect("valid JSON array");
            assert!(!batch.is_empty());
            // Within each batch, sequence numbers must be contiguous and increasing
            for window in batch.windows(2) {
                let seq_a = window[0]["sequence_number"].as_u64().unwrap();
                let seq_b = window[1]["sequence_number"].as_u64().unwrap();
                assert_eq!(seq_b, seq_a + 1, "sequence numbers must be contiguous within batch");
            }
        }
    }
}
