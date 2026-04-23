use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::time::{Duration, Instant};

/// A WebSocket event received from the `/ws/events` endpoint.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WsEvent {
    /// Raw JSON value of the event.
    pub data: serde_json::Value,
}

impl WsEvent {
    /// The event type (e.g. "conversation_created", "response_started").
    pub fn event_type(&self) -> Option<&str> {
        // Could be a normal event or a "lagged" warning
        self.data
            .get("event_type")
            .and_then(|v| v.as_str())
            .or_else(|| self.data.get("type").and_then(|v| v.as_str()))
    }

    /// The agent ID from the event payload.
    pub fn agent_id(&self) -> Option<&str> {
        self.data.get("agent_id").and_then(|v| v.as_str())
    }

    /// The conversation ID from the event payload.
    pub fn conversation_id(&self) -> Option<&str> {
        self.data.get("conversation_id").and_then(|v| v.as_str())
    }

    /// The sequence number from the event payload.
    pub fn sequence_number(&self) -> Option<u64> {
        self.data.get("sequence_number").and_then(|v| v.as_u64())
    }

    /// Whether this is a "lagged" warning (client fell behind).
    pub fn is_lagged(&self) -> bool {
        self.data.get("type").and_then(|v| v.as_str()) == Some("lagged")
    }

    /// Number of missed events (only present on "lagged" events).
    pub fn missed_events(&self) -> Option<u64> {
        self.data.get("missed_events").and_then(|v| v.as_u64())
    }
}

/// A long-lived WebSocket event stream reader that collects events in a
/// background task. Connects to the bridge `/ws/events` endpoint and
/// receives ALL events from ALL agents and conversations.
pub struct WsEventStream {
    events: Arc<std::sync::Mutex<Vec<WsEvent>>>,
    _handle: tokio::task::JoinHandle<()>,
}

impl WsEventStream {
    /// Connect to the WebSocket event stream on the bridge.
    pub async fn connect(bridge_base_url: &str, token: &str) -> Result<Self> {
        use futures::StreamExt;

        let ws_url = bridge_base_url
            .replace("http://", "ws://")
            .replace("https://", "wss://");
        let url = format!("{}/ws/events?token={}", ws_url, token);

        let (ws_stream, _) = tokio_tungstenite::connect_async(&url)
            .await
            .context("failed to connect to WebSocket endpoint")?;

        let events: Arc<std::sync::Mutex<Vec<WsEvent>>> =
            Arc::new(std::sync::Mutex::new(Vec::new()));
        let events_clone = events.clone();

        let handle = tokio::spawn(async move {
            let (_, mut read) = ws_stream.split();

            while let Some(msg) = read.next().await {
                match msg {
                    Ok(tokio_tungstenite::tungstenite::Message::Text(text)) => {
                        if let Ok(data) = serde_json::from_str::<serde_json::Value>(&text) {
                            let event = WsEvent { data };
                            let event_type = event.event_type().unwrap_or("unknown").to_string();
                            eprintln!("[ws] received event: {}", event_type);
                            events_clone.lock().unwrap().push(event);
                        }
                    }
                    Ok(tokio_tungstenite::tungstenite::Message::Close(_)) => break,
                    Err(e) => {
                        eprintln!("[ws] error: {}", e);
                        break;
                    }
                    _ => {} // ignore ping/pong/binary
                }
            }
        });

        Ok(Self {
            events,
            _handle: handle,
        })
    }

    /// Wait until at least `min_count` events have been received, or timeout.
    pub async fn wait_for_event_count(&self, min_count: usize, timeout: Duration) -> Vec<WsEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                if events.len() >= min_count {
                    return events.clone();
                }
            }
            if Instant::now() >= deadline {
                return self.events.lock().unwrap().clone();
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Wait until an event of the given type appears, or timeout.
    pub async fn wait_for_event_type(
        &self,
        event_type: &str,
        timeout: Duration,
    ) -> Option<WsEvent> {
        let deadline = Instant::now() + timeout;
        loop {
            {
                let events = self.events.lock().unwrap();
                if let Some(e) = events.iter().find(|e| e.event_type() == Some(event_type)) {
                    return Some(e.clone());
                }
            }
            if Instant::now() >= deadline {
                return None;
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    /// Get a snapshot of all events collected so far.
    pub fn events(&self) -> Vec<WsEvent> {
        self.events.lock().unwrap().clone()
    }

    /// Get events filtered by event type.
    pub fn events_by_type(&self, event_type: &str) -> Vec<WsEvent> {
        self.events
            .lock()
            .unwrap()
            .iter()
            .filter(|e| e.event_type() == Some(event_type))
            .cloned()
            .collect()
    }

    /// Get events filtered by conversation ID.
    pub fn events_for_conversation(&self, conv_id: &str) -> Vec<WsEvent> {
        self.events
            .lock()
            .unwrap()
            .iter()
            .filter(|e| e.conversation_id() == Some(conv_id))
            .cloned()
            .collect()
    }

    /// Get events filtered by agent ID.
    pub fn events_for_agent(&self, agent_id: &str) -> Vec<WsEvent> {
        self.events
            .lock()
            .unwrap()
            .iter()
            .filter(|e| e.agent_id() == Some(agent_id))
            .cloned()
            .collect()
    }
}
