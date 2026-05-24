use serde::{Deserialize, Serialize};

/// Webhook delivery configuration for tuning throughput and resilience.
///
/// The internal queue is unbounded (zero data loss guarantee), so there is
/// no channel capacity setting. Memory is the buffer — webhook payloads are
/// ~1KB each so even 100K queued events is only ~100MB.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookConfig {
    /// Max concurrent HTTP deliveries. Default: 50.
    #[serde(default = "default_webhook_max_concurrent")]
    pub max_concurrent_deliveries: usize,
    /// Max idle HTTP connections per host. Default: 20.
    #[serde(default = "default_webhook_max_idle")]
    pub max_idle_connections: usize,
    /// Delivery timeout in seconds. Default: 10.
    #[serde(default = "default_webhook_delivery_timeout")]
    pub delivery_timeout_secs: u64,
    /// Max retry attempts. Default: 5.
    #[serde(default = "default_webhook_max_retries")]
    pub max_retries: usize,
    /// How long a per-conversation delivery worker stays alive with no events,
    /// in seconds. Default: 300 (5 minutes).
    #[serde(default = "default_webhook_worker_idle_timeout")]
    pub worker_idle_timeout_secs: u64,
}

impl Default for WebhookConfig {
    fn default() -> Self {
        Self {
            max_concurrent_deliveries: default_webhook_max_concurrent(),
            max_idle_connections: default_webhook_max_idle(),
            delivery_timeout_secs: default_webhook_delivery_timeout(),
            max_retries: default_webhook_max_retries(),
            worker_idle_timeout_secs: default_webhook_worker_idle_timeout(),
        }
    }
}

fn default_webhook_max_concurrent() -> usize {
    50
}
fn default_webhook_max_idle() -> usize {
    20
}
fn default_webhook_delivery_timeout() -> u64 {
    10
}
fn default_webhook_max_retries() -> usize {
    5
}
fn default_webhook_worker_idle_timeout() -> u64 {
    300
}
