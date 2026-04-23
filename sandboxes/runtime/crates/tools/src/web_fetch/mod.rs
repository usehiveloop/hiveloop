//! WebFetch tool: fetch a URL and extract readable content.
//!
//! Uses a two-tier fetch strategy:
//! 1. External fallback service (configurable via `BRIDGE_WEB_FETCH_URL`)
//! 2. `reqwest` + `dom_smoothie` Readability + `htmd` as last resort

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::ToolExecutor;

mod client;
mod parser;

#[cfg(test)]
mod tests;

use client::build_default_client;

/// Output format for web fetch results.
#[derive(Debug, Deserialize, JsonSchema, Clone, Default)]
#[serde(rename_all = "snake_case")]
pub enum FetchFormat {
    /// Default — convert HTML to Markdown using readability extraction.
    #[default]
    Markdown,
    /// Strip all HTML tags, return plain text.
    Text,
    /// Return raw HTML as-is.
    Html,
}

/// Default maximum content length in bytes. Matches the shared tool-result
/// cap (~2KB); anything larger is spilled to disk and the agent is told to
/// use the RipGrep tool to locate specific content.
const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

/// Maximum response body size (5MB).
pub(super) const MAX_RESPONSE_SIZE: usize = 5 * 1024 * 1024;

/// Result returned by the WebFetch tool.
#[derive(Debug, Serialize, Deserialize)]
pub struct FetchResult {
    /// The title extracted from the page, if available.
    pub title: Option<String>,
    /// The extracted content in Markdown format.
    pub content: String,
    /// The final URL (after any redirects).
    pub url: String,
}

/// Arguments for the WebFetch tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebFetchArgs {
    /// The URL to fetch content from. Must be a fully-formed valid URL.
    #[schemars(
        description = "The URL to fetch content from. Must be a fully-formed valid URL. HTTP is upgraded to HTTPS"
    )]
    pub url: String,
    /// Maximum content length in bytes. Capped at the shared tool-result
    /// limit (~2KB); larger results are spilled to disk. Default matches the cap.
    #[schemars(
        description = "Maximum content length in bytes. Capped at ~2KB; larger results are spilled to a temp file and the agent should call RipGrep on that path to find specific content."
    )]
    pub max_length: Option<usize>,
    /// Output format: 'markdown' (default, HTML→Markdown), 'text' (plain text), or 'html' (raw HTML).
    #[schemars(
        description = "Output format: 'markdown' (default, HTML→Markdown), 'text' (plain text, tags stripped), or 'html' (raw HTML)"
    )]
    #[serde(default)]
    pub format: FetchFormat,
}

/// Web fetch tool that retrieves a URL and extracts readable content as Markdown.
///
/// Uses a two-tier fetch strategy:
/// 1. External fallback service (configurable via `BRIDGE_WEB_FETCH_URL`)
/// 2. `reqwest` + `dom_smoothie` Readability + `htmd` as last resort
pub struct WebFetchTool {
    pub(super) client: reqwest::Client,
    pub(super) fallback_url: Option<String>,
}

impl WebFetchTool {
    /// Create a new `WebFetchTool` with a pre-configured HTTP client.
    pub fn new(client: reqwest::Client) -> Self {
        Self {
            client,
            fallback_url: None,
        }
    }

    /// Create a new `WebFetchTool` with default client settings.
    pub fn with_defaults() -> Self {
        let client = build_default_client();
        Self {
            client,
            fallback_url: None,
        }
    }

    /// Create a new `WebFetchTool` with a fallback fetch service URL.
    pub fn with_fallback(fallback_url: String) -> Self {
        let client = build_default_client();
        Self {
            client,
            fallback_url: Some(fallback_url),
        }
    }

    /// Fetch a URL using the two-tier strategy:
    /// 1. Fallback service (if configured)
    /// 2. Reqwest + readability (last resort)
    pub async fn fetch(
        &self,
        url: &str,
        max_length: usize,
        format: &FetchFormat,
    ) -> Result<FetchResult, String> {
        // Tier 1: Fallback service (if configured)
        if self.fallback_url.is_some() {
            match self.fetch_with_fallback(url, max_length, format).await {
                Ok(Some(result)) => {
                    tracing::info!(url = url, tier = "fallback", "web_fetch succeeded");
                    return Ok(result);
                }
                Ok(None) => {
                    tracing::debug!(url = url, "fallback service returned empty, trying reqwest");
                }
                Err(e) => {
                    tracing::debug!(url = url, error = %e, "fallback service failed, trying reqwest");
                }
            }
        }

        // Tier 2: Reqwest + readability (last resort)
        tracing::info!(
            url = url,
            tier = "reqwest",
            "falling back to reqwest + readability"
        );
        self.fetch_with_reqwest(url, max_length, format).await
    }
}

#[async_trait]
impl ToolExecutor for WebFetchTool {
    fn name(&self) -> &str {
        "web_fetch"
    }

    fn description(&self) -> &str {
        include_str!("../instructions/web_fetch.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebFetchArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebFetchArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.url.trim().is_empty() {
            return Err("URL must not be empty".to_string());
        }

        let max_length = args.max_length.unwrap_or(DEFAULT_MAX_LENGTH);

        let result = self.fetch(&args.url, max_length, &args.format).await?;

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
