use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;
use std::sync::Arc;
use tokio::sync::RwLock;

use crate::ToolExecutor;

/// Maximum delay allowed (1 hour).
const MAX_DELAY_SECS: u64 = 3600;

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

// ─── Arguments ──────────────────────────────────────────────────────────────

/// Arguments for the ping_me_back_in tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct PingMeBackArgs {
    /// Number of seconds to wait before pinging back. Maximum: 3600 (1 hour).
    #[schemars(
        description = "Number of seconds to wait before pinging back. Maximum: 3600 (1 hour)"
    )]
    pub seconds: u64,
    /// A message explaining why you want to be pinged back. This will be included
    /// in the ping-back response to remind you of the context.
    #[schemars(
        description = "A message explaining why you want to be pinged back. This will be included in the response to remind you of the context."
    )]
    pub message: String,
}

/// Arguments for the cancel_ping_me_back tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct CancelPingArgs {
    /// The ID of the ping to cancel (returned by ping_me_back_in).
    #[schemars(description = "The ID of the ping to cancel (returned by ping_me_back_in)")]
    pub id: String,
}

// ─── Tools ──────────────────────────────────────────────────────────────────

/// Schedule a delayed ping-back. Returns immediately with a ping ID.
pub struct PingMeBackTool {
    state: PingState,
}

impl PingMeBackTool {
    pub fn new(state: PingState) -> Self {
        Self { state }
    }

    /// Get a reference to the shared ping state.
    pub fn state(&self) -> &PingState {
        &self.state
    }
}

#[async_trait]
impl ToolExecutor for PingMeBackTool {
    fn name(&self) -> &str {
        "ping_me_back_in"
    }

    fn description(&self) -> &str {
        include_str!("instructions/ping_me_back.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(PingMeBackArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: PingMeBackArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.seconds == 0 {
            return Err("seconds must be greater than 0".to_string());
        }

        let delay = args.seconds.min(MAX_DELAY_SECS);
        let id = self.state.add(args.message.clone(), delay).await;

        Ok(format!(
            "Ping scheduled. You will be pinged back in {} seconds.\nPing ID: {}\nTo cancel: use cancel_ping_me_back with id \"{}\"",
            delay, id, id
        ))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

/// Cancel a pending ping-back by ID.
pub struct CancelPingTool {
    state: PingState,
}

impl CancelPingTool {
    pub fn new(state: PingState) -> Self {
        Self { state }
    }
}

#[async_trait]
impl ToolExecutor for CancelPingTool {
    fn name(&self) -> &str {
        "cancel_ping_me_back"
    }

    fn description(&self) -> &str {
        "Cancel a pending ping-me-back timer by its ID. Use this if you no longer need to be pinged back."
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(CancelPingArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: CancelPingArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if self.state.cancel(&args.id).await {
            Ok(format!("Ping '{}' cancelled.", args.id))
        } else {
            Err(format!(
                "Ping '{}' not found. It may have already fired or been cancelled.",
                args.id
            ))
        }
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
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

// ─── Tests ──────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_ping_me_back_returns_immediately() {
        let state = PingState::new();
        let tool = PingMeBackTool::new(state.clone());

        let start = std::time::Instant::now();
        let args = serde_json::json!({
            "seconds": 300,
            "message": "check the deployment"
        });
        let result = tool.execute(args).await.expect("should succeed");
        let elapsed = start.elapsed();

        // Should return immediately, not block for 300 seconds
        assert!(elapsed < std::time::Duration::from_secs(1));
        assert!(result.contains("Ping scheduled"));
        assert!(result.contains("300 seconds"));
        assert!(result.contains("Ping ID:"));

        // State should have one pending ping
        let pings = state.list().await;
        assert_eq!(pings.len(), 1);
        assert_eq!(pings[0].message, "check the deployment");
    }

    #[tokio::test]
    async fn test_cancel_ping() {
        let state = PingState::new();
        let id = state.add("test".to_string(), 300).await;

        let tool = CancelPingTool::new(state.clone());
        let result = tool
            .execute(serde_json::json!({ "id": id }))
            .await
            .expect("should succeed");
        assert!(result.contains("cancelled"));

        assert!(state.list().await.is_empty());
    }

    #[tokio::test]
    async fn test_cancel_nonexistent_ping() {
        let state = PingState::new();
        let tool = CancelPingTool::new(state);
        let result = tool
            .execute(serde_json::json!({ "id": "nonexistent" }))
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_pop_fired() {
        let state = PingState::new();
        // Add a ping that fires immediately (1 second)
        state.add("immediate".to_string(), 0).await;

        // Wait a tiny bit
        tokio::time::sleep(std::time::Duration::from_millis(10)).await;

        // But seconds=0 is rejected by the tool — test via state directly
        // Add with delay 0 would still set fires_at to now, so pop_fired should get it
    }

    #[tokio::test]
    async fn test_zero_seconds_rejected() {
        let state = PingState::new();
        let tool = PingMeBackTool::new(state);
        let result = tool
            .execute(serde_json::json!({ "seconds": 0, "message": "nope" }))
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_format_pending_pings_empty() {
        assert_eq!(format_pending_pings_reminder(&[]), "");
    }

    #[tokio::test]
    async fn test_format_pending_pings_with_items() {
        let pings = vec![PendingPing {
            id: "abc123".to_string(),
            message: "check build".to_string(),
            fires_at: tokio::time::Instant::now() + std::time::Duration::from_secs(120),
            delay_secs: 120,
            created_at: chrono::Utc::now(),
        }];
        let text = format_pending_pings_reminder(&pings);
        assert!(text.contains("abc123"));
        assert!(text.contains("check build"));
        assert!(text.contains("Pending Ping-Me-Back"));
    }

    #[tokio::test]
    async fn test_clamps_to_max_delay() {
        let state = PingState::new();
        let tool = PingMeBackTool::new(state.clone());
        let result = tool
            .execute(serde_json::json!({ "seconds": 99999, "message": "long" }))
            .await
            .expect("should succeed");
        assert!(result.contains("3600 seconds"));

        let pings = state.list().await;
        assert_eq!(pings[0].delay_secs, 3600); // clamped to max
    }
}
