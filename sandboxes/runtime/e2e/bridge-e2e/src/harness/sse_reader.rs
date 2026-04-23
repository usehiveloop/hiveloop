use anyhow::{anyhow, Context, Result};
use std::time::{Duration, Instant};

use super::types::{unwrap_bridge_event, SseEvent};
use super::TestHarness;

impl TestHarness {
    /// Connect to SSE stream and collect events until Done or timeout.
    pub async fn stream_sse_until_done(
        &self,
        conv_id: &str,
        timeout: Duration,
    ) -> Result<(Vec<SseEvent>, String)> {
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
        let mut response_text = String::new();
        let mut current_event_type = String::new();

        let deadline = Instant::now() + timeout;

        let mut stream = resp.bytes_stream();

        let mut buffer = String::new();

        loop {
            let remaining = deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                eprintln!("[harness] SSE stream timed out after {:?}", timeout);
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
                Ok(None) => {
                    // Stream ended
                    break;
                }
                Err(_) => {
                    eprintln!("[harness] SSE stream timed out after {:?}", timeout);
                    break;
                }
            }

            // Process complete lines
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

                    // Unwrap BridgeEvent: extract inner data and event_type
                    let (bridge_event_type, data) = unwrap_bridge_event(&raw);

                    // Determine event type from event: line, BridgeEvent, or data.type
                    let event_type = if !current_event_type.is_empty() {
                        current_event_type.clone()
                    } else if let Some(t) = bridge_event_type {
                        t
                    } else if let Some(t) = data.get("type").and_then(|v| v.as_str()) {
                        t.to_string()
                    } else {
                        "message".to_string()
                    };

                    // Collect content deltas into response text
                    if event_type == "content_delta" || event_type == "response_chunk" {
                        if let Some(delta) = data.get("delta").and_then(|d| d.as_str()) {
                            response_text.push_str(delta);
                        }
                    }

                    let event = SseEvent {
                        event_type: event_type.clone(),
                        data,
                    };
                    events.push(event);

                    // Log the SSE event
                    let last = events.last().unwrap();
                    self.log_sse_event(conv_id, &last.event_type, &last.data);

                    // Stop when we get a Done event
                    if event_type == "done" {
                        return Ok((events, response_text));
                    }

                    current_event_type.clear();
                }
            }
        }

        // If we have events but no response text, try to extract from error events
        if response_text.is_empty() && !events.is_empty() {
            eprintln!(
                "[harness] Warning: no content_delta events found. Events received: {:?}",
                events
                    .iter()
                    .map(|e| format!(
                        "{}:{}",
                        e.event_type,
                        &e.data.to_string()[..e.data.to_string().len().min(100)]
                    ))
                    .collect::<Vec<_>>()
            );
        }

        Ok((events, response_text))
    }

    /// Connect to SSE stream and collect events across multiple turns.
    /// Keeps reading past "done" events until `done_count` "done" events
    /// have been received, or the timeout expires.
    pub async fn stream_sse_until_done_count(
        &self,
        conv_id: &str,
        done_count: usize,
        timeout: Duration,
    ) -> Result<(Vec<SseEvent>, String)> {
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
        let mut response_text = String::new();
        let mut current_event_type = String::new();
        let mut done_seen = 0usize;

        let deadline = Instant::now() + timeout;

        let mut stream = resp.bytes_stream();
        let mut buffer = String::new();

        loop {
            let remaining = deadline.saturating_duration_since(Instant::now());
            if remaining.is_zero() {
                eprintln!(
                    "[harness] SSE stream timed out after {:?} (done_seen={}/{})",
                    timeout, done_seen, done_count
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
                Ok(None) => {
                    // Stream ended
                    break;
                }
                Err(_) => {
                    eprintln!(
                        "[harness] SSE stream timed out after {:?} (done_seen={}/{})",
                        timeout, done_seen, done_count
                    );
                    break;
                }
            }

            // Process complete lines
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

                    if event_type == "content_delta" || event_type == "response_chunk" {
                        if let Some(delta) = data.get("delta").and_then(|d| d.as_str()) {
                            response_text.push_str(delta);
                        }
                    }

                    let event = SseEvent {
                        event_type: event_type.clone(),
                        data,
                    };
                    events.push(event);

                    // Log the SSE event
                    let last = events.last().unwrap();
                    self.log_sse_event(conv_id, &last.event_type, &last.data);

                    if event_type == "done" {
                        done_seen += 1;
                        if done_seen >= done_count {
                            return Ok((events, response_text));
                        }
                    }

                    current_event_type.clear();
                }
            }
        }

        Ok((events, response_text))
    }
}
