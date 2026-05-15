use serde::{Deserialize, Serialize};

/// Outbound reply payload. `Rich` is per-platform (Slack Block Kit, Discord
/// Components, Teams Adaptive Cards, ...) — adapters render it themselves.
#[derive(Debug, Clone)]
pub enum Reply {
    Text(String),
    Rich(serde_json::Value),
}

/// Handle returned by `ChannelGateway::reply`, used to edit the message later
/// (progressive streaming, status updates, ...).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "openapi", derive(utoipa::ToSchema))]
pub struct MessageHandle {
    pub channel: String,
    pub ts: String,
}
