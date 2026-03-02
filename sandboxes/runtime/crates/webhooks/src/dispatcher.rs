use bridge_core::webhook::WebhookPayload;
use reqwest::Client;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

pub struct WebhookDispatcher {
    client: Client,
    event_tx: mpsc::Sender<WebhookPayload>,
}

impl WebhookDispatcher {
    pub fn new() -> (Self, mpsc::Receiver<WebhookPayload>) {
        let (tx, rx) = mpsc::channel(1000);
        let dispatcher = Self {
            client: Client::new(),
            event_tx: tx,
        };
        (dispatcher, rx)
    }

    /// Returns a clone of the internal HTTP client.
    pub fn client(&self) -> Client {
        self.client.clone()
    }

    /// Fire-and-forget dispatch.
    pub fn dispatch(&self, payload: WebhookPayload) {
        let _ = self.event_tx.try_send(payload);
    }

    /// Background delivery loop with retry.
    pub async fn run(
        mut rx: mpsc::Receiver<WebhookPayload>,
        client: Client,
        cancel: CancellationToken,
    ) {
        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                payload = rx.recv() => {
                    let Some(payload) = payload else { break };
                    let client = client.clone();
                    tokio::spawn(async move {
                        deliver_webhook(client, payload).await;
                    });
                }
            }
        }
    }
}

async fn deliver_webhook(client: Client, payload: WebhookPayload) {
    use backon::{ExponentialBuilder, Retryable};

    use crate::signer::sign_webhook;

    let url = payload.webhook_url.clone();
    let secret = payload.webhook_secret.clone();
    let body = match serde_json::to_vec(&payload) {
        Ok(b) => b,
        Err(e) => {
            tracing::error!("Failed to serialize webhook payload: {}", e);
            return;
        }
    };

    let result = (|| {
        let client = client.clone();
        let url = url.clone();
        let secret = secret.clone();
        let body = body.clone();
        async move {
            let timestamp = chrono::Utc::now().timestamp();
            let signature = sign_webhook(&body, &secret, timestamp);
            let response = client
                .post(&url)
                .header("Content-Type", "application/json")
                .header("X-Webhook-Signature", &signature)
                .header("X-Webhook-Timestamp", timestamp.to_string())
                .body(body)
                .timeout(std::time::Duration::from_secs(10))
                .send()
                .await?;

            let status = response.status();
            if status.is_server_error() {
                return Err(response.error_for_status().unwrap_err());
            }
            Ok(())
        }
    })
    .retry(
        ExponentialBuilder::default()
            .with_max_times(5)
            .with_jitter(),
    )
    .sleep(tokio::time::sleep)
    .await;

    if let Err(e) = result {
        tracing::error!("Webhook delivery failed after retries to {}: {}", url, e);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::webhook::{WebhookEventType, WebhookPayload};

    #[test]
    fn test_dispatch_does_not_block() {
        let (dispatcher, _rx) = WebhookDispatcher::new();

        let payload = WebhookPayload {
            event_type: WebhookEventType::ConversationCreated,
            agent_id: "agent-1".to_string(),
            conversation_id: "conv-1".to_string(),
            timestamp: chrono::Utc::now(),
            data: serde_json::json!({}),
            webhook_url: "https://example.com/webhook".to_string(),
            webhook_secret: "secret".to_string(),
        };

        // dispatch uses try_send which should not block even without an active receiver
        dispatcher.dispatch(payload);
    }
}
