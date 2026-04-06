//! Webhook HTTP delivery pipeline for `BridgeEvent`.
//!
//! Receives events from the EventBus via an unbounded channel, routes them
//! to per-conversation workers for ordered delivery, and batches events
//! when multiple queue up.

use bridge_core::config::WebhookConfig;
use bridge_core::event::BridgeEvent;
use reqwest::Client;
use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::Duration;
use storage::StorageHandle;
use tokio::sync::mpsc;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

/// Per-worker delivery settings.
#[derive(Clone, Copy)]
struct WorkerConfig {
    delivery_timeout: Duration,
    max_retries: usize,
    idle_timeout: Duration,
}

/// Run the webhook HTTP delivery loop.
///
/// Receives `BridgeEvent` from the EventBus, routes to per-conversation
/// workers for ordered delivery, and signs payloads using the provided
/// webhook URL and secret.
pub async fn run_delivery(
    mut rx: mpsc::UnboundedReceiver<BridgeEvent>,
    client: Client,
    cancel: CancellationToken,
    config: WebhookConfig,
    webhook_url: String,
    webhook_secret: String,
    storage: Option<StorageHandle>,
) {
    let max_inflight = config.max_concurrent_deliveries;
    let semaphore = Arc::new(tokio::sync::Semaphore::new(max_inflight));
    let worker_config = WorkerConfig {
        delivery_timeout: Duration::from_secs(config.delivery_timeout_secs),
        max_retries: config.max_retries,
        idle_timeout: Duration::from_secs(config.worker_idle_timeout_secs),
    };

    let mut workers: HashMap<String, mpsc::UnboundedSender<BridgeEvent>> = HashMap::new();
    let mut worker_handles: JoinSet<String> = JoinSet::new();
    let delivered = Arc::new(AtomicU64::new(0));

    loop {
        tokio::select! {
            _ = cancel.cancelled() => break,

            Some(result) = worker_handles.join_next() => {
                if let Ok(conv_id) = result {
                    let is_stale = workers
                        .get(&conv_id)
                        .is_none_or(|tx| tx.is_closed());
                    if is_stale {
                        workers.remove(&conv_id);
                    }
                }
            }

            event = rx.recv() => {
                let Some(event) = event else { break };
                route_event(
                    event,
                    &mut workers,
                    &mut worker_handles,
                    &client,
                    &semaphore,
                    &worker_config,
                    &webhook_url,
                    &webhook_secret,
                    storage.clone(),
                    &delivered,
                );
            }
        }
    }

    // Graceful drain
    rx.close();
    let mut drained = 0u64;
    while let Some(event) = rx.recv().await {
        route_event(
            event,
            &mut workers,
            &mut worker_handles,
            &client,
            &semaphore,
            &worker_config,
            &webhook_url,
            &webhook_secret,
            storage.clone(),
            &delivered,
        );
        drained += 1;
    }
    if drained > 0 {
        tracing::info!(count = drained, "drained remaining events for delivery");
    }

    workers.clear();
    while worker_handles.join_next().await.is_some() {}
}

#[allow(clippy::too_many_arguments)]
fn route_event(
    event: BridgeEvent,
    workers: &mut HashMap<String, mpsc::UnboundedSender<BridgeEvent>>,
    worker_handles: &mut JoinSet<String>,
    client: &Client,
    semaphore: &Arc<tokio::sync::Semaphore>,
    config: &WorkerConfig,
    webhook_url: &str,
    webhook_secret: &str,
    storage: Option<StorageHandle>,
    delivered: &Arc<AtomicU64>,
) {
    let conv_id = event.conversation_id.clone();

    let event = match workers.get(&conv_id) {
        Some(tx) => match tx.send(event) {
            Ok(()) => return,
            Err(e) => e.0,
        },
        None => event,
    };

    let (tx, worker_rx) = mpsc::unbounded_channel();
    tx.send(event).expect("fresh channel cannot be closed");
    workers.insert(conv_id.clone(), tx);

    let worker_client = client.clone();
    let worker_sem = semaphore.clone();
    let worker_config = *config;
    let url = webhook_url.to_string();
    let secret = webhook_secret.to_string();
    let delivered = delivered.clone();

    worker_handles.spawn(async move {
        conversation_worker(
            &conv_id,
            worker_rx,
            worker_client,
            worker_sem,
            worker_config,
            &url,
            &secret,
            storage,
            &delivered,
        )
        .await;
        conv_id
    });
}

#[allow(clippy::too_many_arguments)]
async fn conversation_worker(
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

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::event::BridgeEventType;

    fn make_event(conv_id: &str) -> BridgeEvent {
        BridgeEvent::new(
            BridgeEventType::ConversationCreated,
            "agent-1",
            conv_id,
            serde_json::json!({}),
        )
    }

    #[tokio::test]
    async fn test_delivery_exits_on_cancel() {
        let (_tx, rx) = mpsc::unbounded_channel();
        let client = Client::new();
        let cancel = CancellationToken::new();
        let config = WebhookConfig::default();

        let cancel_clone = cancel.clone();
        let handle = tokio::spawn(async move {
            run_delivery(
                rx,
                client,
                cancel_clone,
                config,
                "https://example.com".to_string(),
                "secret".to_string(),
                None,
            )
            .await;
        });

        tokio::time::sleep(Duration::from_millis(10)).await;
        cancel.cancel();

        tokio::time::timeout(Duration::from_secs(2), handle)
            .await
            .expect("should exit on cancel")
            .expect("should not panic");
    }

    #[tokio::test]
    async fn test_delivery_body_is_json_array() {
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

        let (tx, rx) = mpsc::unbounded_channel();
        let client = Client::new();
        let cancel = CancellationToken::new();

        tx.send(make_event("conv-1")).unwrap();

        let cancel_clone = cancel.clone();
        let url = mock_server.uri();
        let handle = tokio::spawn(async move {
            run_delivery(
                rx,
                client,
                cancel_clone,
                config,
                url,
                "secret".to_string(),
                None,
            )
            .await;
        });

        tokio::time::sleep(Duration::from_millis(500)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 1);

        let body: serde_json::Value =
            serde_json::from_slice(&requests[0].body).expect("valid JSON");
        assert!(body.is_array(), "body must be a JSON array");
        assert_eq!(body.as_array().unwrap().len(), 1);

        // Verify no secrets in the delivered payload
        let event = &body[0];
        assert!(event.get("webhook_url").is_none());
        assert!(event.get("webhook_secret").is_none());
        assert!(event["event_id"].is_string());
        assert!(event["sequence_number"].is_number());
    }

    #[tokio::test]
    async fn test_delivery_sequence_numbers_preserved() {
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

        let (tx, rx) = mpsc::unbounded_channel();
        let client = Client::new();
        let cancel = CancellationToken::new();

        // Send events with pre-stamped sequence numbers (as EventBus would)
        for i in 1..=5 {
            let mut event = make_event("conv-1");
            event.sequence_number = i;
            tx.send(event).unwrap();
        }

        let cancel_clone = cancel.clone();
        let url = mock_server.uri();
        let handle = tokio::spawn(async move {
            run_delivery(
                rx,
                client,
                cancel_clone,
                config,
                url,
                "secret".to_string(),
                None,
            )
            .await;
        });

        tokio::time::sleep(Duration::from_secs(1)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();
        let mut all_seq: Vec<u64> = Vec::new();
        for req in &requests {
            let batch: Vec<serde_json::Value> =
                serde_json::from_slice(&req.body).expect("valid JSON array");
            for event in &batch {
                all_seq.push(event["sequence_number"].as_u64().unwrap());
            }
        }

        assert_eq!(all_seq, vec![1, 2, 3, 4, 5]);
    }

    #[tokio::test]
    async fn test_cross_conversation_delivery() {
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

        let (tx, rx) = mpsc::unbounded_channel();
        let client = Client::new();
        let cancel = CancellationToken::new();

        for i in 0..3 {
            let mut e_a = make_event("conv-A");
            e_a.sequence_number = (i * 2 + 1) as u64;
            tx.send(e_a).unwrap();

            let mut e_b = make_event("conv-B");
            e_b.sequence_number = (i * 2 + 2) as u64;
            tx.send(e_b).unwrap();
        }

        let cancel_clone = cancel.clone();
        let url = mock_server.uri();
        let handle = tokio::spawn(async move {
            run_delivery(
                rx,
                client,
                cancel_clone,
                config,
                url,
                "secret".to_string(),
                None,
            )
            .await;
        });

        tokio::time::sleep(Duration::from_secs(1)).await;
        cancel.cancel();
        handle.await.unwrap();

        let requests = mock_server.received_requests().await.unwrap();
        let mut conv_a: Vec<u64> = Vec::new();
        let mut conv_b: Vec<u64> = Vec::new();
        for req in &requests {
            let batch: Vec<serde_json::Value> =
                serde_json::from_slice(&req.body).expect("valid JSON array");
            for event in &batch {
                let conv = event["conversation_id"].as_str().unwrap();
                let seq = event["sequence_number"].as_u64().unwrap();
                match conv {
                    "conv-A" => conv_a.push(seq),
                    "conv-B" => conv_b.push(seq),
                    _ => panic!("unexpected conversation"),
                }
            }
        }

        assert_eq!(conv_a.len(), 3);
        assert_eq!(conv_b.len(), 3);
        // Each conversation's events are in order
        assert_eq!(conv_a, vec![1, 3, 5]);
        assert_eq!(conv_b, vec![2, 4, 6]);
    }
}
