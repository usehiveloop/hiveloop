use anyhow::{anyhow, Context, Result};
use std::time::{Duration, Instant};

use super::types::{ReceivedWebhookRaw, WebhookEntry, WebhookLog};
use super::TestHarness;

impl TestHarness {
    /// GET /webhooks/log on the mock control plane — retrieve received webhooks
    /// as raw JSON values (kept for backwards compatibility).
    pub async fn get_webhook_log_raw(&self) -> Result<Vec<serde_json::Value>> {
        let resp = self
            .client
            .get(format!("{}/webhooks/log", self.cp_base_url))
            .send()
            .await
            .context("GET webhook log failed")?;

        let body = resp
            .json()
            .await
            .context("failed to parse webhook log body")?;
        Ok(body)
    }

    /// GET /webhooks/log on the mock control plane — retrieve received webhooks
    /// as typed [`WebhookLog`] with query helpers.
    ///
    /// Webhooks are delivered as batched JSON arrays, so each received POST may
    /// contain multiple events. This method flattens them into individual entries.
    pub async fn get_webhook_log(&self) -> Result<WebhookLog> {
        let raw = self.get_webhook_log_raw().await?;
        let mut entries = Vec::new();
        for v in raw {
            let received: ReceivedWebhookRaw =
                serde_json::from_value(v).expect("failed to deserialize ReceivedWebhookRaw");
            match received.body {
                // Batched format: body is an array of event objects
                serde_json::Value::Array(events) => {
                    for event in events {
                        entries.push(WebhookEntry {
                            timestamp: received.timestamp.clone(),
                            headers: received.headers.clone(),
                            body: event,
                        });
                    }
                }
                // Legacy/single format: body is a single event object
                other => {
                    entries.push(WebhookEntry {
                        timestamp: received.timestamp.clone(),
                        headers: received.headers.clone(),
                        body: other,
                    });
                }
            }
        }
        Ok(WebhookLog { entries })
    }

    /// Poll the webhook log until at least `min_count` entries are present, or
    /// until `timeout` elapses. Returns the final [`WebhookLog`].
    ///
    /// Useful because webhook dispatch is fire-and-forget with retries, so
    /// there is a brief delivery delay.
    pub async fn wait_for_webhooks(
        &self,
        min_count: usize,
        timeout: Duration,
    ) -> Result<WebhookLog> {
        let deadline = Instant::now() + timeout;
        loop {
            let log = self.get_webhook_log().await?;
            if log.len() >= min_count {
                return Ok(log);
            }
            if Instant::now() >= deadline {
                return Ok(log);
            }
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
    }

    /// Poll the webhook log until a specific event type is present, or until
    /// `timeout` elapses.
    pub async fn wait_for_webhook_type(
        &self,
        event_type: &str,
        timeout: Duration,
    ) -> Result<WebhookLog> {
        let deadline = Instant::now() + timeout;
        loop {
            let log = self.get_webhook_log().await?;
            if log.has_type(event_type) {
                return Ok(log);
            }
            if Instant::now() >= deadline {
                return Ok(log);
            }
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
    }

    /// DELETE /webhooks/log on the mock control plane — clear the webhook log.
    pub async fn clear_webhook_log(&self) -> Result<()> {
        let resp = self
            .client
            .delete(format!("{}/webhooks/log", self.cp_base_url))
            .send()
            .await
            .context("DELETE webhook log failed")?;

        if !resp.status().is_success() {
            return Err(anyhow!("failed to clear webhook log"));
        }

        Ok(())
    }
}
