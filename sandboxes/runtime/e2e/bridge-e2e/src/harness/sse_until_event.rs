use anyhow::{anyhow, Context, Result};
use std::time::{Duration, Instant};

use super::types::{unwrap_bridge_event, SseEvent};
use super::TestHarness;

impl TestHarness {
    /// Stream SSE events collecting them until a specific event type is seen,
    /// then return all events collected so far (including the target event).
    ///
    /// Useful for waiting until `tool_approval_required` fires before interacting
    /// with the approval API.
    pub async fn stream_sse_until_event(
        &self,
        conv_id: &str,
        target_event_type: &str,
        timeout: Duration,
    ) -> Result<Vec<SseEvent>> {
        use futures::StreamExt;

        let stream_client = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .build()
            .context("failed to build stream client")?;

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                self.bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("GET stream request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!("stream endpoint returned {}: {}", status, body));
        }

        let mut events = Vec::new();
        let mut current_event_type = String::new();
        let deadline = Instant::now() + timeout;
        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();

        loop {
            let remaining = deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                eprintln!(
                    "[harness] SSE stream timed out waiting for '{}'",
                    target_event_type
                );
                break;
            }

            match tokio::time::timeout(remaining, stream.next()).await {
                Ok(Some(Ok(chunk))) => {
                    buffer.push_str(&String::from_utf8_lossy(&chunk));
                }
                Ok(Some(Err(e))) => {
                    eprintln!("[harness] SSE stream chunk error: {}", e);
                    break;
                }
                Ok(None) => break,
                Err(_) => {
                    eprintln!(
                        "[harness] SSE stream timed out waiting for '{}'",
                        target_event_type
                    );
                    break;
                }
            }

            while let Some(newline_pos) = buffer.find('\n') {
                let line = buffer[..newline_pos].trim_end().to_string();
                buffer = buffer[newline_pos + 1..].to_string();

                if line.is_empty() {
                    continue;
                }

                if let Some(event_name) = line.strip_prefix("event:") {
                    current_event_type = event_name.trim().to_string();
                } else if let Some(data_str) = line.strip_prefix("data:") {
                    let data_str = data_str.trim();
                    if data_str.is_empty() {
                        continue;
                    }

                    let raw: serde_json::Value = serde_json::from_str(data_str)
                        .unwrap_or_else(|_| serde_json::Value::String(data_str.to_string()));

                    let (bridge_event_type, data) = unwrap_bridge_event(&raw);

                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = bridge_event_type {
                        t
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

                    self.log_sse_event(conv_id, &event_type, &data);

                    let event = SseEvent {
                        event_type: event_type.clone(),
                        data,
                    };
                    events.push(event);

                    if event_type == target_event_type || event_type == "done" {
                        return Ok(events);
                    }

                    current_event_type.clear();
                }
            }
        }

        Ok(events)
    }
}
