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
mod tests;
