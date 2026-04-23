use anyhow::{anyhow, Context, Result};
use std::sync::Arc;
use std::time::{Duration, Instant};

use super::types::{format_sse_for_log, unwrap_bridge_event, SseEvent};

/// A long-lived SSE stream reader that collects events in a background task.
///
/// Unlike `stream_sse_until_done`, this keeps the connection alive so the test
/// can interact with the approval API while events continue to arrive.
pub struct SseStream {
    events: Arc<std::sync::Mutex<Vec<SseEvent>>>,
    _handle: tokio::task::JoinHandle<()>,
}

impl SseStream {
    /// Connect to the SSE stream for a conversation and start collecting events
    /// in a background task. Events are logged to the console as they arrive.
    pub async fn connect(bridge_base_url: &str, conv_id: &str) -> Result<Self> {
        let stream_client = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(10))
            .build()
            .context("failed to build stream client")?;

        let resp = stream_client
            .get(format!(
                "{}/conversations/{}/stream",
                bridge_base_url, conv_id
            ))
            .send()
            .await
            .context("GET stream request failed")?;

        let status = resp.status();
        if !status.is_success() {
            let body = resp.text().await.unwrap_or_default();
            return Err(anyhow!("stream endpoint returned {}: {}", status, body));
        }

        let events: Arc<std::sync::Mutex<Vec<SseEvent>>> =
            Arc::new(std::sync::Mutex::new(Vec::new()));
        let events_clone = events.clone();
        let log_conv_id = conv_id.to_string();

        let handle = tokio::spawn(async move {
            use futures::StreamExt;
            let mut stream = resp.bytes_stream();
            let mut buffer = String::new();
            let mut current_event_type = String::new();

            loop {
                match stream.next().await {
                    Some(Ok(chunk)) => {
                        buffer.push_str(&String::from_utf8_lossy(&chunk));
                    }
                    _ => break,
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

                        let event = SseEvent { event_type, data };

                        // Log to console like stream_sse_until_done does
                        let formatted = format_sse_for_log(&event.event_type, &event.data);
                        let short_id = if log_conv_id.len() > 8 {
                            &log_conv_id[..8]
                        } else {
                            &log_conv_id
                        };
                        for line in formatted.lines() {
                            eprintln!("[conv:{}] [SSE:{}] {}", short_id, event.event_type, line);
                        }

                        events_clone.lock().unwrap().push(event);
                        current_event_type.clear();
                    }
                }
            }
        });

        Ok(Self {
            events,
            _handle: handle,
        })
    }

    /// Wait until an event of the given type appears, or timeout.
    pub async fn wait_for_event(&self, event_type: &str, timeout: Duration) -> Option<SseEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                if let Some(e) = events.iter().find(|e| e.event_type == event_type) {
                    return Some(e.clone());
                }
            }
            if Instant::now() >= deadline {
                return None;
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Wait until the "done" event appears, or timeout. Returns all collected events.
    pub async fn wait_for_done(&self, timeout: Duration) -> Vec<SseEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                if events.iter().any(|e| e.event_type == "done") {
                    return events.clone();
                }
            }
            if Instant::now() >= deadline {
                let events = self.events.lock().unwrap();
                return events.clone();
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Wait until N "done" events have appeared, or timeout. Returns all collected events.
    pub async fn wait_for_done_count(&self, count: usize, timeout: Duration) -> Vec<SseEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                let done_count = events.iter().filter(|e| e.event_type == "done").count();
                if done_count >= count {
                    return events.clone();
                }
            }
            if Instant::now() >= deadline {
                let events = self.events.lock().unwrap();
                return events.clone();
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Get a snapshot of all events collected so far.
    pub fn events(&self) -> Vec<SseEvent> {
        self.events.lock().unwrap().clone()
    }
}
