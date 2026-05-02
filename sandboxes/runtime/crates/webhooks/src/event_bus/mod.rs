use bridge_core::event::BridgeEvent;
use dashmap::DashMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use storage::StorageHandle;
use tokio::sync::{broadcast, mpsc};

/// Default broadcast buffer size for WebSocket fan-out.
/// Slow consumers that fall behind will receive a `Lagged` error.
const WS_BUFFER_SIZE: usize = 10_000;

/// Default per-conversation SSE broadcast buffer.
const SSE_BUFFER_SIZE: usize = 256;

/// Central event bus that is the single entry point for all events.
///
/// Every event emitted by the bridge runtime flows through the EventBus.
/// The bus stamps a globally monotonic `sequence_number`, then fans the
/// event out to all delivery channels simultaneously:
///
/// 1. **DB** — persisted to `webhook_outbox` for durability
/// 2. **WebSocket** — broadcast to all connected WS clients
/// 3. **SSE** — broadcast to all subscribers of a per-conversation channel
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
    /// Per-conversation SSE broadcast senders. Multiple subscribers per
    /// conversation are supported — every `subscribe_sse` call returns a
    /// fresh receiver attached to the same sender.
    sse_streams: Arc<DashMap<String, broadcast::Sender<BridgeEvent>>>,
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

        // 4. Fan out to SSE subscribers of this conversation
        if let Some(sse_tx) = self.sse_streams.get(event.conversation_id.as_str()) {
            let _ = sse_tx.send(event.clone());
        }

        // 5. Queue for webhook HTTP delivery
        if let Some(ref webhook_tx) = self.webhook_tx {
            let _ = webhook_tx.send(event);
        }

        self.emitted.fetch_add(1, Ordering::Relaxed);
    }

    /// Replay a persisted event into the webhook delivery queue only.
    ///
    /// Used at startup to retry pending webhook deliveries that were
    /// in-flight when the previous bridge process exited. The event is
    /// **not** re-stamped (its original `sequence_number` is preserved)
    /// and is **not** fanned out to live SSE/WS subscribers — those
    /// channels are for fresh-after-startup activity, not historical
    /// catch-up.
    pub fn emit_replayed(&self, event: BridgeEvent) {
        if let Some(ref webhook_tx) = self.webhook_tx {
            let _ = webhook_tx.send(event);
        }
    }

    /// Idempotently ensure an SSE broadcast sender exists for `conversation_id`.
    /// Safe to call multiple times for the same id; existing subscribers are
    /// unaffected. Call this at conversation create/restore time so the first
    /// event isn't dropped while waiting for a client to subscribe.
    pub fn register_sse_stream(&self, conversation_id: String) {
        self.sse_streams
            .entry(conversation_id)
            .or_insert_with(|| broadcast::channel(SSE_BUFFER_SIZE).0);
    }

    /// Subscribe to a conversation's SSE broadcast. Auto-creates the sender
    /// if the conversation hasn't been registered yet, so subscriptions don't
    /// race against `register_sse_stream`. Multiple concurrent subscribers
    /// are supported — each gets an independent receiver.
    pub fn subscribe_sse(&self, conversation_id: &str) -> broadcast::Receiver<BridgeEvent> {
        let entry = self
            .sse_streams
            .entry(conversation_id.to_string())
            .or_insert_with(|| broadcast::channel(SSE_BUFFER_SIZE).0);
        entry.subscribe()
    }

    /// Remove an SSE stream for a conversation (e.g. when the conversation
    /// ends). Live subscribers will see `RecvError::Closed` on their next read.
    pub fn remove_sse_stream(&self, conversation_id: &str) {
        self.sse_streams.remove(conversation_id);
    }

    /// Subscribe to the WebSocket broadcast stream.
    pub fn subscribe_ws(&self) -> broadcast::Receiver<BridgeEvent> {
        self.ws_tx.subscribe()
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

    /// Number of registered SSE conversation channels (not subscribers).
    pub fn sse_stream_count(&self) -> usize {
        self.sse_streams.len()
    }

    /// Number of live SSE subscribers across all conversations.
    pub fn sse_subscriber_count(&self) -> usize {
        self.sse_streams.iter().map(|e| e.receiver_count()).sum()
    }
}

#[cfg(test)]
mod tests;
