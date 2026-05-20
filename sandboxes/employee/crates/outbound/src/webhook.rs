use std::collections::HashMap;
use std::time::Duration;

use async_trait::async_trait;
use domain::OutboundEvent;
use hmac::{Hmac, Mac};
use reqwest::Client as HttpClient;
use sha2::Sha256;
use tracing::warn;

use crate::{OutboundChannel, OutboundError, Result};

const HTTP_TIMEOUT_SECONDS: u64 = 15;
const SIGNATURE_HEADER: &str = "X-Hivy-Signature";
const EVENT_TYPE_HEADER: &str = "X-Hivy-Event-Type";

pub struct WebhookChannel {
    name: String,
    url: String,
    secret: String,
    extra_headers: HashMap<String, String>,
    event_filter: Option<Vec<String>>,
    http: HttpClient,
}

impl WebhookChannel {
    pub fn new(
        name: impl Into<String>,
        url: impl Into<String>,
        secret: impl Into<String>,
        extra_headers: HashMap<String, String>,
        event_filter: Option<Vec<String>>,
    ) -> Result<Self> {
        let http = HttpClient::builder()
            .timeout(Duration::from_secs(HTTP_TIMEOUT_SECONDS))
            .build()
            .map_err(|e| OutboundError::Delivery(format!("http client: {e}")))?;
        Ok(Self {
            name: name.into(),
            url: url.into(),
            secret: secret.into(),
            extra_headers,
            event_filter,
            http,
        })
    }
}

#[async_trait]
impl OutboundChannel for WebhookChannel {
    fn name(&self) -> &str {
        &self.name
    }

    fn kind(&self) -> &'static str {
        "webhook"
    }

    fn accepts(&self, event_type: &str) -> bool {
        match self.event_filter.as_ref() {
            None => true,
            Some(filters) if filters.is_empty() => true,
            Some(filters) => filters.iter().any(|f| filter_matches(f, event_type)),
        }
    }

    async fn deliver(&self, event: &OutboundEvent) -> Result<()> {
        let body = serde_json::to_vec(event)
            .map_err(|e| OutboundError::Delivery(format!("serialize event: {e}")))?;
        let signature = compute_signature(&self.secret, &body);

        let mut request = self
            .http
            .post(&self.url)
            .header(SIGNATURE_HEADER, format!("sha256={signature}"))
            .header(EVENT_TYPE_HEADER, &event.event_type)
            .header(reqwest::header::CONTENT_TYPE, "application/json");
        for (header_name, header_value) in &self.extra_headers {
            request = request.header(header_name.as_str(), header_value.as_str());
        }

        let response = request
            .body(body)
            .send()
            .await
            .map_err(|e| OutboundError::Delivery(format!("send {}: {e}", self.url)))?;
        let status = response.status();
        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            warn!(channel = %self.name, %status, body = %body, "webhook non-2xx");
            return Err(OutboundError::Delivery(format!(
                "{} returned {status}",
                self.url
            )));
        }
        Ok(())
    }
}

pub(crate) fn compute_signature(secret: &str, body: &[u8]) -> String {
    let mut mac = Hmac::<Sha256>::new_from_slice(secret.as_bytes()).expect("hmac key any length");
    mac.update(body);
    hex::encode(mac.finalize().into_bytes())
}

pub(crate) fn filter_matches(pattern: &str, event_type: &str) -> bool {
    if pattern == event_type {
        return true;
    }
    if let Some(prefix) = pattern.strip_suffix(".*") {
        return event_type.starts_with(prefix) && event_type.len() > prefix.len();
    }
    if pattern == "*" {
        return true;
    }
    false
}
