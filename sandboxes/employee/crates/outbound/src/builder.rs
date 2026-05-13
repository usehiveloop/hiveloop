use std::sync::Arc;

use domain::{OutboundChannelKind, OutboundChannelSpec};
use sqlx::SqlitePool;
use storage::SharedWriteNotifier;
use tracing::{info, warn};

use crate::{
    DatabaseChannel, OutboundChannel, OutboundError, OutboundRegistry, Result, WebhookChannel,
};

pub fn build_registry(
    sqlite_pool: Arc<SqlitePool>,
    specs: &[OutboundChannelSpec],
) -> Result<OutboundRegistry> {
    build_registry_with_write_notifier(sqlite_pool, specs, None)
}

pub fn build_registry_with_write_notifier(
    sqlite_pool: Arc<SqlitePool>,
    specs: &[OutboundChannelSpec],
    write_notifier: Option<SharedWriteNotifier>,
) -> Result<OutboundRegistry> {
    let mut registry = OutboundRegistry::new();
    let database_channel: Arc<dyn OutboundChannel> = match write_notifier {
        Some(write_notifier) => Arc::new(DatabaseChannel::with_write_notifier(
            sqlite_pool,
            write_notifier,
        )),
        None => Arc::new(DatabaseChannel::new(sqlite_pool)),
    };
    registry.add(database_channel);

    for spec in specs {
        match build_channel_from_spec(spec) {
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

fn build_channel_from_spec(spec: &OutboundChannelSpec) -> Result<Arc<dyn OutboundChannel>> {
    match &spec.kind {
        OutboundChannelKind::Webhook {
            url,
            secret_env,
            extra_headers,
        } => {
            let secret = std::env::var(secret_env).map_err(|_| {
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
