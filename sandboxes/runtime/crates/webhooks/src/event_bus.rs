use bridge_core::event::BridgeEvent;
use dashmap::DashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use storage::StorageHandle;
use tokio::sync::{broadcast, mpsc};

/// Default broadcast buffer size for WebSocket fan-out.
/// Slow consumers that fall behind will receive a `Lagged` error.
const WS_BUFFER_SIZE: usize = 10_000;

/// Central event bus that is the single entry point for all events.
///
/// Every event emitted by the bridge runtime flows through the EventBus.
/// The bus stamps a globally monotonic `sequence_number`, then fans the
/// event out to all delivery channels simultaneously:
///
/// 1. **DB** — persisted to `webhook_outbox` for durability
/// 2. **WebSocket** — broadcast to all connected WS clients
/// 3. **SSE** — routed to the per-conversation SSE stream
/// 4. **Webhook HTTP** — queued for batched HTTP delivery to the control plane
///
/// Every channel receives the exact same `BridgeEvent` with the same
/// `sequence_number`, `event_id`, `timestamp`, and `data`.
pub struct EventBus {
    /// Mutex that serialises sequence assignment + broadcast send so that
    /// concurrent emitters cannot reorder events in the WS broadcast channel.
    emit_lock: Mutex<()>,
    /// Global monotonically increasing sequence counter.
    sequence: AtomicU64,
    /// Optional persistence handle for storing events.
    storage: Option<StorageHandle>,
    /// Broadcast sender for WebSocket fan-out.
    ws_tx: broadcast::Sender<BridgeEvent>,
    /// Per-conversation SSE streams.
    sse_streams: Arc<DashMap<String, mpsc::Sender<BridgeEvent>>>,
    /// Channel for webhook HTTP delivery pipeline.
    webhook_tx: Option<mpsc::UnboundedSender<BridgeEvent>>,
    /// Webhook URL for HTTP delivery.
    webhook_url: String,
    /// Webhook secret for HMAC signing during HTTP delivery.
    webhook_secret: String,
    /// High-water-mark: total events emitted since startup.
    emitted: AtomicU64,
}

impl EventBus {
    /// Create a new EventBus.
    ///
    /// - `webhook_tx`: channel for the HTTP delivery pipeline (None disables HTTP webhooks)
    /// - `storage`: optional persistence handle
    /// - `webhook_url`/`webhook_secret`: delivery config for HTTP webhooks
    pub fn new(
        webhook_tx: Option<mpsc::UnboundedSender<BridgeEvent>>,
        storage: Option<StorageHandle>,
        webhook_url: String,
        webhook_secret: String,
    ) -> Self {
        let (ws_tx, _) = broadcast::channel(WS_BUFFER_SIZE);
        Self {
            emit_lock: Mutex::new(()),
            sequence: AtomicU64::new(0),
            storage,
            ws_tx,
            sse_streams: Arc::new(DashMap::new()),
            webhook_tx,
            webhook_url,
            webhook_secret,
            emitted: AtomicU64::new(0),
        }
    }

    /// Emit an event to all delivery channels.
    ///
    /// Stamps a globally monotonic `sequence_number` on the event, then
    /// fans out to DB, WebSocket, SSE, and webhook HTTP delivery.
    pub fn emit(&self, mut event: BridgeEvent) {
        // Hold the lock across sequence assignment + all channel sends
        // to guarantee that events appear in sequence order in every channel.
        let _guard = self.emit_lock.lock().unwrap_or_else(|e| e.into_inner());

        // 1. Stamp global sequence number
        let seq = self.sequence.fetch_add(1, Ordering::Relaxed) + 1;
        event.sequence_number = seq;

        // 2. Persist to DB
        if let Some(ref storage) = self.storage {
            storage.enqueue_event(event.clone());
        }

        // 3. Broadcast to WebSocket clients
        let _ = self.ws_tx.send(event.clone());

        // 4. Route to per-conversation SSE stream
        if let Some(sse_tx) = self.sse_streams.get(event.conversation_id.as_str()) {
            let _ = sse_tx.try_send(event.clone());
        }

        // 5. Queue for webhook HTTP delivery
        if let Some(ref webhook_tx) = self.webhook_tx {
            let _ = webhook_tx.send(event);
        }

        self.emitted.fetch_add(1, Ordering::Relaxed);
    }

    /// Emit a replayed event (already persisted in DB). Skips DB persistence
    /// but fans out to WS, SSE, and webhook HTTP delivery.
    pub fn emit_replayed(&self, mut event: BridgeEvent) {
        let _guard = self.emit_lock.lock().unwrap_or_else(|e| e.into_inner());

        let seq = self.sequence.fetch_add(1, Ordering::Relaxed) + 1;
        event.sequence_number = seq;

        let _ = self.ws_tx.send(event.clone());

        if let Some(sse_tx) = self.sse_streams.get(event.conversation_id.as_str()) {
            let _ = sse_tx.try_send(event.clone());
        }

        if let Some(ref webhook_tx) = self.webhook_tx {
            let _ = webhook_tx.send(event);
        }

        self.emitted.fetch_add(1, Ordering::Relaxed);
    }

    /// Register an SSE stream for a conversation. Returns the receiver end.
    pub fn register_sse_stream(
        &self,
        conversation_id: String,
        buffer_size: usize,
    ) -> mpsc::Receiver<BridgeEvent> {
        let (tx, rx) = mpsc::channel(buffer_size);
        self.sse_streams.insert(conversation_id, tx);
        rx
    }

    /// Remove an SSE stream for a conversation (e.g. when the client disconnects
    /// or the conversation ends).
    pub fn remove_sse_stream(&self, conversation_id: &str) {
        self.sse_streams.remove(conversation_id);
    }

    /// Subscribe to the WebSocket broadcast stream.
    pub fn subscribe_ws(&self) -> broadcast::Receiver<BridgeEvent> {
        self.ws_tx.subscribe()
    }

    /// Returns a reference to the SSE streams map (for external inspection or
    /// migration during hydration).
    pub fn sse_streams(&self) -> &Arc<DashMap<String, mpsc::Sender<BridgeEvent>>> {
        &self.sse_streams
    }

    /// Returns the webhook URL for HTTP delivery.
    pub fn webhook_url(&self) -> &str {
        &self.webhook_url
    }

    /// Returns the webhook secret for HMAC signing.
    pub fn webhook_secret(&self) -> &str {
        &self.webhook_secret
    }

    /// Total events emitted since startup.
    pub fn emitted_count(&self) -> u64 {
        self.emitted.load(Ordering::Relaxed)
    }

    /// Current global sequence number (last assigned).
    pub fn current_sequence(&self) -> u64 {
        self.sequence.load(Ordering::Relaxed)
    }

    /// Number of currently active WebSocket subscribers.
    pub fn ws_subscriber_count(&self) -> usize {
        self.ws_tx.receiver_count()
    }

    /// Number of active SSE streams.
    pub fn sse_stream_count(&self) -> usize {
        self.sse_streams.len()
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

    #[test]
    fn test_emit_stamps_monotonic_sequence_numbers() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        let mut ws_rx = bus.subscribe_ws();

        bus.emit(make_event("conv-1"));
        bus.emit(make_event("conv-2"));
        bus.emit(make_event("conv-1"));

        let e1 = ws_rx.try_recv().unwrap();
        let e2 = ws_rx.try_recv().unwrap();
        let e3 = ws_rx.try_recv().unwrap();

        assert_eq!(e1.sequence_number, 1);
        assert_eq!(e2.sequence_number, 2);
        assert_eq!(e3.sequence_number, 3);
    }

    #[test]
    fn test_ws_and_sse_receive_same_event() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        let mut ws_rx = bus.subscribe_ws();
        let mut sse_rx = bus.register_sse_stream("conv-1".to_string(), 16);

        bus.emit(make_event("conv-1"));

        let ws_event = ws_rx.try_recv().unwrap();
        let sse_event = sse_rx.try_recv().unwrap();

        assert_eq!(ws_event.event_id, sse_event.event_id);
        assert_eq!(ws_event.sequence_number, sse_event.sequence_number);
        assert_eq!(ws_event.event_type, sse_event.event_type);
        assert_eq!(ws_event.agent_id, sse_event.agent_id);
        assert_eq!(ws_event.conversation_id, sse_event.conversation_id);
        assert_eq!(ws_event.data, sse_event.data);
    }

    #[test]
    fn test_sse_only_receives_matching_conversation() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        let mut sse_rx = bus.register_sse_stream("conv-1".to_string(), 16);

        bus.emit(make_event("conv-2")); // different conversation
        bus.emit(make_event("conv-1")); // matching conversation

        // Should only receive the conv-1 event
        let event = sse_rx.try_recv().unwrap();
        assert_eq!(event.conversation_id, "conv-1");
        assert_eq!(event.sequence_number, 2);

        // No more events
        assert!(sse_rx.try_recv().is_err());
    }

    #[test]
    fn test_webhook_channel_receives_events() {
        let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
        let bus = EventBus::new(
            Some(webhook_tx),
            None,
            "https://example.com".to_string(),
            "secret".to_string(),
        );

        bus.emit(make_event("conv-1"));
        bus.emit(make_event("conv-2"));

        let e1 = webhook_rx.try_recv().unwrap();
        let e2 = webhook_rx.try_recv().unwrap();

        assert_eq!(e1.sequence_number, 1);
        assert_eq!(e2.sequence_number, 2);
        assert_eq!(e1.conversation_id, "conv-1");
        assert_eq!(e2.conversation_id, "conv-2");
    }

    #[test]
    fn test_all_channels_get_identical_data() {
        let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
        let bus = EventBus::new(
            Some(webhook_tx),
            None,
            "https://example.com".to_string(),
            "secret".to_string(),
        );
        let mut ws_rx = bus.subscribe_ws();
        let mut sse_rx = bus.register_sse_stream("conv-1".to_string(), 16);

        let event = BridgeEvent::new(
            BridgeEventType::ResponseChunk,
            "agent-1",
            "conv-1",
            serde_json::json!({"delta": "Hello", "message_id": "msg-1"}),
        );
        bus.emit(event);

        let ws = ws_rx.try_recv().unwrap();
        let sse = sse_rx.try_recv().unwrap();
        let wh = webhook_rx.try_recv().unwrap();

        // All three channels have the same event_id
        assert_eq!(ws.event_id, sse.event_id);
        assert_eq!(sse.event_id, wh.event_id);

        // All three have the same sequence_number
        assert_eq!(ws.sequence_number, 1);
        assert_eq!(sse.sequence_number, 1);
        assert_eq!(wh.sequence_number, 1);

        // All three have the same data
        assert_eq!(ws.data, sse.data);
        assert_eq!(sse.data, wh.data);

        // All three have the same event_type
        assert_eq!(ws.event_type, sse.event_type);
        assert_eq!(sse.event_type, wh.event_type);
    }

    #[test]
    fn test_remove_sse_stream() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        let _sse_rx = bus.register_sse_stream("conv-1".to_string(), 16);
        assert_eq!(bus.sse_stream_count(), 1);

        bus.remove_sse_stream("conv-1");
        assert_eq!(bus.sse_stream_count(), 0);
    }

    #[test]
    fn test_emitted_count() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        assert_eq!(bus.emitted_count(), 0);

        bus.emit(make_event("conv-1"));
        bus.emit(make_event("conv-2"));
        assert_eq!(bus.emitted_count(), 2);
    }

    #[test]
    fn test_emit_without_any_subscribers_does_not_panic() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        // No WS subscribers, no SSE streams, no webhook channel
        bus.emit(make_event("conv-1"));
        assert_eq!(bus.emitted_count(), 1);
        assert_eq!(bus.current_sequence(), 1);
    }

    #[test]
    fn test_emit_replayed_skips_db_but_fans_out() {
        let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
        let bus = EventBus::new(Some(webhook_tx), None, String::new(), String::new());
        let mut ws_rx = bus.subscribe_ws();

        bus.emit_replayed(make_event("conv-1"));

        let ws = ws_rx.try_recv().unwrap();
        let wh = webhook_rx.try_recv().unwrap();
        assert_eq!(ws.sequence_number, 1);
        assert_eq!(wh.sequence_number, 1);
    }

    #[test]
    fn test_multiple_sse_streams_independent() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        let mut sse_a = bus.register_sse_stream("conv-a".to_string(), 16);
        let mut sse_b = bus.register_sse_stream("conv-b".to_string(), 16);

        bus.emit(make_event("conv-a"));
        bus.emit(make_event("conv-b"));
        bus.emit(make_event("conv-a"));

        // conv-a receives 2 events
        let a1 = sse_a.try_recv().unwrap();
        let a2 = sse_a.try_recv().unwrap();
        assert!(sse_a.try_recv().is_err());
        assert_eq!(a1.sequence_number, 1);
        assert_eq!(a2.sequence_number, 3);

        // conv-b receives 1 event
        let b1 = sse_b.try_recv().unwrap();
        assert!(sse_b.try_recv().is_err());
        assert_eq!(b1.sequence_number, 2);
    }

    #[test]
    fn test_no_secrets_on_event() {
        let (webhook_tx, mut webhook_rx) = mpsc::unbounded_channel();
        let bus = EventBus::new(
            Some(webhook_tx),
            None,
            "https://secret-url.com".to_string(),
            "webhook-signing-test-key".to_string(),
        );

        bus.emit(make_event("conv-1"));

        let event = webhook_rx.try_recv().unwrap();
        let json = serde_json::to_value(&event).unwrap();
        let obj = json.as_object().unwrap();

        // BridgeEvent must NOT contain webhook_url or webhook_secret
        assert!(!obj.contains_key("webhook_url"));
        assert!(!obj.contains_key("webhook_secret"));
    }

    #[test]
    fn test_event_json_shape() {
        let bus = EventBus::new(None, None, String::new(), String::new());
        let mut ws_rx = bus.subscribe_ws();

        bus.emit(BridgeEvent::new(
            BridgeEventType::ToolCallStarted,
            "agent-5",
            "conv-99",
            serde_json::json!({"name": "bash", "arguments": {"command": "ls"}}),
        ));

        let event = ws_rx.try_recv().unwrap();
        let json = serde_json::to_value(&event).unwrap();

        assert_eq!(json["event_type"], "tool_call_started");
        assert_eq!(json["agent_id"], "agent-5");
        assert_eq!(json["conversation_id"], "conv-99");
        assert_eq!(json["sequence_number"], 1);
        assert!(json["event_id"].is_string());
        assert!(json["timestamp"].is_string());
        assert_eq!(json["data"]["name"], "bash");
    }
}
