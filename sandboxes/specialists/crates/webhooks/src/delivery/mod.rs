//! Webhook HTTP delivery pipeline for `BridgeEvent`.
//!
//! Receives events from the EventBus via an unbounded channel, routes them
//! to per-conversation workers for ordered delivery, and batches events
//! when multiple queue up.

use bridge_core::config::WebhookConfig;
use bridge_core::event::BridgeEvent;
use reqwest::Client;
use std::collections::HashMap;
use std::sync::atomic::AtomicU64;
use std::sync::Arc;
use std::time::Duration;
use storage::StorageHandle;
use tokio::sync::mpsc;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

mod worker;

use worker::{conversation_worker, WorkerConfig};

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

#[cfg(test)]
mod tests;
