use std::collections::HashMap;
use std::sync::Arc;

use domain::{OutboundChannelKind, OutboundChannelSpec};
use tracing::{info, warn};

use crate::{OutboundChannel, OutboundError, OutboundRegistry, Result, WebhookChannel};

pub fn build_registry(specs: &[OutboundChannelSpec]) -> Result<OutboundRegistry> {
    build_registry_with_env(specs, &HashMap::new())
}

pub fn build_registry_with_env(
    specs: &[OutboundChannelSpec],
    runtime_env: &HashMap<String, String>,
) -> Result<OutboundRegistry> {
    let mut registry = OutboundRegistry::new();

    for spec in specs {
        match build_channel_from_spec(spec, runtime_env) {
            Ok(channel) => {
                info!(name = %spec.name, kind = %channel.kind(), "registered outbound channel");
                registry.add(channel);
            }
            Err(error) => {
                warn!(name = %spec.name, %error, "skipping outbound channel");
            }
        }
    }
    Ok(registry)
}

fn build_channel_from_spec(
    spec: &OutboundChannelSpec,
    runtime_env: &HashMap<String, String>,
) -> Result<Arc<dyn OutboundChannel>> {
    match &spec.kind {
        OutboundChannelKind::Webhook {
            url,
            secret_env,
            extra_headers,
        } => {
            let secret = runtime_env.get(secret_env).cloned().ok_or_else(|| {
                OutboundError::Other(anyhow::anyhow!(
                    "env var `{secret_env}` not set for webhook `{}`",
                    spec.name
                ))
            })?;
            let channel = WebhookChannel::new(
                spec.name.clone(),
                url.clone(),
                secret,
                extra_headers.clone(),
                spec.event_filter.clone(),
            )?;
            Ok(Arc::new(channel))
        }
    }
}
