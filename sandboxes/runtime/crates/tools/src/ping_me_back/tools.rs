use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;

use super::state::{PingState, MAX_DELAY_SECS};
use crate::ToolExecutor;

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
        include_str!("../instructions/ping_me_back.txt")
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
        include_str!("../instructions/cancel_ping_me_back.txt")
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
