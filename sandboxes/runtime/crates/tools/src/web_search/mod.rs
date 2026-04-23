use async_trait::async_trait;
use backon::{ExponentialBuilder, Retryable};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

use crate::ToolExecutor;

#[cfg(test)]
mod tests;

/// A single search result.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SearchResult {
    pub title: String,
    pub url: String,
    pub snippet: String,
    pub position: Option<u32>,
}

/// Arguments for the WebSearch tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebSearchArgs {
    /// The search query string.
    pub query: String,
}

/// Web search tool that POSTs to a single configurable search endpoint
/// and parses a Serper-format response.
pub struct WebSearchTool {
    client: reqwest::Client,
    endpoint: String,
    description: String,
}

impl WebSearchTool {
    /// Create a new `WebSearchTool` with the given endpoint URL.
    pub fn new(endpoint: String) -> Self {
        let year = chrono::Utc::now().format("%Y").to_string();
        let description = include_str!("../instructions/web_search.txt").replace("{{year}}", &year);
        Self {
            client: reqwest::Client::builder()
                .timeout(Duration::from_secs(15))
                .build()
                .expect("failed to build HTTP client"),
            endpoint,
            description,
        }
    }

    /// Execute the search request with retry logic.
    async fn search_with_retry(&self, query: String) -> Result<Vec<SearchResult>, String> {
        let client = &self.client;
        let endpoint = &self.endpoint;

        let do_request = || async {
            let body = serde_json::json!({ "q": query });

            let response = client
                .post(endpoint)
                .header("Content-Type", "application/json")
                .json(&body)
                .send()
                .await
                .map_err(|e| format!("Request failed: {e}"))?;

            let status = response.status();
            let response_body = response
                .text()
                .await
                .map_err(|e| format!("Failed to read response body: {e}"))?;

            if status.is_server_error() {
                return Err(format!("Server error {status}: {response_body}"));
            }

            if !status.is_success() {
                return Err(format!("HTTP {status}: {response_body}"));
            }

            parse_serper_response(&response_body)
        };

        do_request
            .retry(
                ExponentialBuilder::default()
                    .with_min_delay(Duration::from_millis(500))
                    .with_max_delay(Duration::from_secs(5))
                    .with_max_times(3),
            )
            .when(|e: &String| e.starts_with("Server error") || e.starts_with("Request failed"))
            .await
    }
}

/// Parse a Serper-format JSON response into a `Vec<SearchResult>`.
///
/// Expected shape:
/// ```json
/// {
///   "knowledgeGraph": { "title": "...", "description": "..." },
///   "organic": [{ "title": "...", "link": "...", "snippet": "...", "position": 1 }],
///   "peopleAlsoAsk": [{ "question": "...", "snippet": "..." }]
/// }
/// ```
fn parse_serper_response(body: &str) -> Result<Vec<SearchResult>, String> {
    let json: serde_json::Value =
        serde_json::from_str(body).map_err(|e| format!("Failed to parse JSON response: {e}"))?;

    let mut results = Vec::new();

    // Optionally prepend knowledge graph as first result
    if let Some(kg) = json.get("knowledgeGraph") {
        let title = kg.get("title").and_then(|v| v.as_str()).unwrap_or_default();
        let description = kg
            .get("description")
            .and_then(|v| v.as_str())
            .unwrap_or_default();

        if !title.is_empty() {
            // Build snippet from description + any attributes
            let mut snippet = description.to_string();
            if let Some(attrs) = kg.get("attributes").and_then(|a| a.as_object()) {
                for (key, value) in attrs {
                    if let Some(v) = value.as_str() {
                        snippet.push_str(&format!(" {key}: {v}."));
                    }
                }
            }

            results.push(SearchResult {
                title: title.to_string(),
                url: String::new(),
                snippet,
                position: Some(0),
            });
        }
    }

    // Extract organic results
    if let Some(organic) = json.get("organic").and_then(|v| v.as_array()) {
        for item in organic {
            let title = item
                .get("title")
                .and_then(|v| v.as_str())
                .unwrap_or_default()
                .to_string();
            let url = item
                .get("link")
                .and_then(|v| v.as_str())
                .unwrap_or_default()
                .to_string();
            let snippet = item
                .get("snippet")
                .and_then(|v| v.as_str())
                .unwrap_or_default()
                .to_string();
            let position = item
                .get("position")
                .and_then(|v| v.as_u64())
                .map(|p| p as u32);

            if !title.is_empty() {
                results.push(SearchResult {
                    title,
                    url,
                    snippet,
                    position,
                });
            }
        }
    }

    Ok(results)
}

#[async_trait]
impl ToolExecutor for WebSearchTool {
    fn name(&self) -> &str {
        "web_search"
    }

    fn description(&self) -> &str {
        &self.description
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebSearchArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebSearchArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.query.trim().is_empty() {
            return Err("Search query must not be empty".to_string());
        }

        let results = self.search_with_retry(args.query).await?;

        serde_json::to_string(&results).map_err(|e| format!("Failed to serialize results: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}
