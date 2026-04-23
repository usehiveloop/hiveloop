use std::sync::Arc;
use tokio::sync::RwLock;

/// Maximum delay allowed (1 hour).
pub(super) const MAX_DELAY_SECS: u64 = 3600;

/// A pending ping-back timer.
#[derive(Debug, Clone)]
pub struct PendingPing {
    /// Unique identifier for this ping.
    pub id: String,
    /// The message to return when the timer fires.
    pub message: String,
    /// When this ping should fire.
    pub fires_at: tokio::time::Instant,
    /// Requested delay in seconds (for display).
    pub delay_secs: u64,
    /// When this ping was created (for display).
    pub created_at: chrono::DateTime<chrono::Utc>,
}

/// Shared state for pending pings, accessible by tools and the conversation loop.
#[derive(Clone, Default)]
pub struct PingState {
    inner: Arc<RwLock<Vec<PendingPing>>>,
}

impl PingState {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(RwLock::new(Vec::new())),
        }
    }

    /// Add a new pending ping. Returns the ping ID.
    pub async fn add(&self, message: String, delay_secs: u64) -> String {
        let id = uuid::Uuid::new_v4().to_string()[..8].to_string();
        let ping = PendingPing {
            id: id.clone(),
            message,
            fires_at: tokio::time::Instant::now()
                + std::time::Duration::from_secs(delay_secs.min(MAX_DELAY_SECS)),
            delay_secs,
            created_at: chrono::Utc::now(),
        };
        self.inner.write().await.push(ping);
        id
    }

    /// Cancel a pending ping by ID. Returns true if found and removed.
    pub async fn cancel(&self, id: &str) -> bool {
        let mut pings = self.inner.write().await;
        let len_before = pings.len();
        pings.retain(|p| p.id != id);
        pings.len() < len_before
    }

    /// Return the next ping that should fire, or None if no pings are pending.
    /// This does NOT remove the ping — call `pop_fired` after it fires.
    pub async fn next_fire_time(&self) -> Option<tokio::time::Instant> {
        let pings = self.inner.read().await;
        pings.iter().map(|p| p.fires_at).min()
    }

    /// Remove and return all pings that have fired (fires_at <= now).
    pub async fn pop_fired(&self) -> Vec<PendingPing> {
        let now = tokio::time::Instant::now();
        let mut pings = self.inner.write().await;
        let (fired, remaining): (Vec<_>, Vec<_>) = pings.drain(..).partition(|p| p.fires_at <= now);
        *pings = remaining;
        fired
    }

    /// Get a snapshot of all pending pings (for system reminders).
    pub async fn list(&self) -> Vec<PendingPing> {
        self.inner.read().await.clone()
    }
}

/// Format pending pings as a system reminder section.
pub fn format_pending_pings_reminder(pings: &[PendingPing]) -> String {
    if pings.is_empty() {
        return String::new();
    }

    let mut lines = Vec::new();
    lines.push("## Pending Ping-Me-Back Timers\n".to_string());
    for ping in pings {
        let remaining = ping
            .fires_at
            .saturating_duration_since(tokio::time::Instant::now());
        let remaining_secs = remaining.as_secs();
        lines.push(format!(
            "- **{}** — fires in ~{}s: {}",
            ping.id, remaining_secs, ping.message
        ));
    }
    lines.push("\nUse `cancel_ping_me_back` to cancel any pending ping.".to_string());
    lines.join("\n")
}
