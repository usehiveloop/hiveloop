use crate::WebhookDispatcher;
use std::sync::Arc;

/// Bundles a dispatcher, URL, and secret into a single clonable handle.
///
/// `Option<WebhookContext>` is `None` when webhooks are disabled.
#[derive(Clone)]
pub struct WebhookContext {
    pub dispatcher: Arc<WebhookDispatcher>,
    pub url: String,
    pub secret: String,
}
