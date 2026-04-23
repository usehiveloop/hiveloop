//! Spider API-backed web tools (crawl, search, links, screenshot, transform).

use serde::Deserialize;
use std::time::Duration;

mod crawl;
mod links;
mod screenshot;
mod search;
mod transform;

pub use crawl::{WebCrawlArgs, WebCrawlTool};
pub use links::{WebGetLinksArgs, WebGetLinksTool};
pub use screenshot::{WaitForSelector, WebScreenshotArgs, WebScreenshotTool};
pub use search::{WebSearchArgs, WebSearchTool};
pub use transform::{TransformItem, WebTransformArgs, WebTransformTool};

// ─── Shared Spider API Client ──────────────────────────────────────────

/// HTTP client for Spider's hosted API. Shared across all Spider-backed tools.
pub struct SpiderClient {
    client: reqwest::Client,
    base_url: String,
}

impl SpiderClient {
    pub fn new(base_url: String) -> Self {
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(60))
            .build()
            .expect("failed to build Spider HTTP client");
        Self { client, base_url }
    }

    /// POST JSON to a Spider API endpoint and return the response body.
    pub(crate) async fn post(
        &self,
        path: &str,
        body: &serde_json::Value,
    ) -> Result<String, String> {
        let url = format!("{}{}", self.base_url, path);

        let response = self
            .client
            .post(&url)
            .header("Content-Type", "application/json")
            .json(body)
            .send()
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    "Spider API request timed out (60s). The site may be slow or unresponsive."
                        .to_string()
                } else {
                    format!("Spider API request failed: {e}")
                }
            })?;

        let status = response.status();
        let text = response
            .text()
            .await
            .map_err(|e| format!("Failed to read Spider API response: {e}"))?;

        if !status.is_success() {
            // Try to extract error message from JSON response
            if let Ok(parsed) = serde_json::from_str::<serde_json::Value>(&text) {
                if let Some(error) = parsed.get("error").and_then(|e| e.as_str()) {
                    return Err(format!("Spider API error ({}): {}", status, error));
                }
            }
            return Err(format!("Spider API error ({}): {}", status, text));
        }

        Ok(text)
    }
}

// ─── Shared response types ─────────────────────────────────────────────

#[derive(Debug, Deserialize)]
pub(crate) struct CrawlPage {
    pub(crate) url: String,
    #[serde(default)]
    pub(crate) content: String,
    #[serde(default)]
    pub(crate) error: Option<String>,
}
