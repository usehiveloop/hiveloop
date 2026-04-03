use bridge_core::webhook::WebhookPayload;
use std::sync::atomic::{AtomicU64, Ordering};
use tokio::sync::broadcast;

/// Broadcast buffer size. Slow consumers that fall more than this many events
/// behind will receive a `Lagged` error with the number of missed events.
const DEFAULT_BUFFER_SIZE: usize = 10_000;

/// Fan-out broadcaster for WebSocket event delivery.
///
/// Every event dispatched through the webhook system is also sent here.
/// Connected WebSocket clients subscribe via `subscribe()` and receive
/// a copy of every event across all agents and conversations.
///
/// Uses `tokio::sync::broadcast` internally — multiple consumers each
/// get their own view of the stream, and slow consumers are notified
/// when they fall behind (rather than blocking producers).
pub struct WsBroadcaster {
    tx: broadcast::Sender<WebhookPayload>,
    /// Global monotonically increasing sequence number stamped on every
    /// event before broadcast. Clients use this for gap detection on
    /// reconnection.
    sequence: AtomicU64,
}

impl WsBroadcaster {
    /// Create a new broadcaster with the default buffer size.
    pub fn new() -> Self {
        Self::with_capacity(DEFAULT_BUFFER_SIZE)
    }

    /// Create a new broadcaster with a custom buffer capacity.
    pub fn with_capacity(capacity: usize) -> Self {
        let (tx, _) = broadcast::channel(capacity);
        Self {
            tx,
            sequence: AtomicU64::new(0),
        }
    }

    /// Broadcast an event to all connected WebSocket clients.
    ///
    /// Stamps a global `ws_sequence` on the payload before sending.
    /// Returns the assigned sequence number.
    ///
    /// If there are no active subscribers the event is silently dropped
    /// (this is expected — WebSocket connections are optional).
    pub fn broadcast(&self, mut payload: WebhookPayload) -> u64 {
        let seq = self.sequence.fetch_add(1, Ordering::Relaxed) + 1;
        payload.sequence_number = seq;
        // broadcast::send returns Err only when there are zero receivers,
        // which is fine — no connected clients means nobody to deliver to.
        let _ = self.tx.send(payload);
        seq
    }

    /// Subscribe to the event stream. Returns a receiver that will get
    /// a copy of every subsequently broadcast event.
    pub fn subscribe(&self) -> broadcast::Receiver<WebhookPayload> {
        self.tx.subscribe()
    }

    /// The number of events broadcast so far (since creation).
    pub fn broadcast_count(&self) -> u64 {
        self.sequence.load(Ordering::Relaxed)
    }

    /// The number of currently active subscribers.
    pub fn subscriber_count(&self) -> usize {
        self.tx.receiver_count()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use bridge_core::webhook::{WebhookEventType, WebhookPayload};

    fn make_payload() -> WebhookPayload {
        WebhookPayload {
            event_id: "evt-1".to_string(),
            event_type: WebhookEventType::ConversationCreated,
            agent_id: "agent-1".to_string(),
            conversation_id: "conv-1".to_string(),
            timestamp: chrono::Utc::now(),
            sequence_number: 0,
            data: serde_json::json!({"test": true}),
            webhook_url: "https://example.com/webhook".to_string(),
            webhook_secret: "secret".to_string(),
        }
    }

    #[test]
    fn test_broadcast_send_receive() {
        let broadcaster = WsBroadcaster::new();
        let mut rx = broadcaster.subscribe();

        broadcaster.broadcast(make_payload());

        let received = rx.try_recv().expect("should receive event");
        assert_eq!(received.agent_id, "agent-1");
        assert_eq!(received.conversation_id, "conv-1");
        assert_eq!(received.sequence_number, 1);
    }

    #[test]
    fn test_broadcast_multiple_subscribers() {
        let broadcaster = WsBroadcaster::new();
        let mut rx1 = broadcaster.subscribe();
        let mut rx2 = broadcaster.subscribe();

        broadcaster.broadcast(make_payload());

        let e1 = rx1.try_recv().expect("subscriber 1 should receive");
        let e2 = rx2.try_recv().expect("subscriber 2 should receive");
        assert_eq!(e1.sequence_number, e2.sequence_number);
        assert_eq!(e1.agent_id, e2.agent_id);
    }

    #[test]
    fn test_broadcast_sequence_numbers_monotonic() {
        let broadcaster = WsBroadcaster::new();
        let mut rx = broadcaster.subscribe();

        for _ in 0..5 {
            broadcaster.broadcast(make_payload());
        }

        let mut prev_seq = 0;
        for _ in 0..5 {
            let event = rx.try_recv().expect("should receive event");
            assert!(
                event.sequence_number > prev_seq,
                "sequence numbers must be strictly increasing"
            );
            prev_seq = event.sequence_number;
        }
        assert_eq!(prev_seq, 5);
    }

    #[test]
    fn test_late_subscriber_does_not_get_old_events() {
        let broadcaster = WsBroadcaster::new();

        // Broadcast before subscribing
        broadcaster.broadcast(make_payload());

        let mut rx = broadcaster.subscribe();

        // Should not receive the event sent before subscription
        assert!(rx.try_recv().is_err());

        // But should receive new ones
        broadcaster.broadcast(make_payload());
        let event = rx.try_recv().expect("should receive new event");
        assert_eq!(event.sequence_number, 2);
    }

    #[test]
    fn test_broadcast_lagged_subscriber() {
        // Create a tiny buffer so we can easily trigger lag
        let broadcaster = WsBroadcaster::with_capacity(2);
        let mut rx = broadcaster.subscribe();

        // Send 5 events into a buffer of 2 — subscriber will lag
        for _ in 0..5 {
            broadcaster.broadcast(make_payload());
        }

        match rx.try_recv() {
            Err(broadcast::error::TryRecvError::Lagged(n)) => {
                assert!(n > 0, "should report missed events");
            }
            other => panic!("expected Lagged, got {:?}", other),
        }
    }

    #[test]
    fn test_broadcast_no_subscribers_does_not_panic() {
        let broadcaster = WsBroadcaster::new();
        // No subscribers — should not panic
        let seq = broadcaster.broadcast(make_payload());
        assert_eq!(seq, 1);
        assert_eq!(broadcaster.broadcast_count(), 1);
    }

    #[test]
    fn test_subscriber_count() {
        let broadcaster = WsBroadcaster::new();
        assert_eq!(broadcaster.subscriber_count(), 0);

        let _rx1 = broadcaster.subscribe();
        assert_eq!(broadcaster.subscriber_count(), 1);

        let _rx2 = broadcaster.subscribe();
        assert_eq!(broadcaster.subscriber_count(), 2);

        drop(_rx1);
        assert_eq!(broadcaster.subscriber_count(), 1);
    }

    #[test]
    fn test_dropped_broadcaster_closes_receivers() {
        let broadcaster = WsBroadcaster::new();
        let mut rx = broadcaster.subscribe();

        drop(broadcaster);

        match rx.try_recv() {
            Err(broadcast::error::TryRecvError::Closed) => {}
            other => panic!("expected Closed after drop, got {:?}", other),
        }
    }
}
