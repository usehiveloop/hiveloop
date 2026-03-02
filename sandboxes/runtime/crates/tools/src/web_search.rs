use async_trait::async_trait;
use backon::{ExponentialBuilder, Retryable};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

use crate::ToolExecutor;

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
}

impl WebSearchTool {
    /// Create a new `WebSearchTool` with the given endpoint URL.
    pub fn new(endpoint: String) -> Self {
        Self {
            client: reqwest::Client::builder()
                .timeout(Duration::from_secs(15))
                .build()
                .expect("failed to build HTTP client"),
            endpoint,
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
            .when(|e: &String| {
                e.starts_with("Server error") || e.starts_with("Request failed")
            })
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
        let title = kg
            .get("title")
            .and_then(|v| v.as_str())
            .unwrap_or_default();
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
            let position = item.get("position").and_then(|v| v.as_u64()).map(|p| p as u32);

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
        "Search the web for information. Returns structured results with title, URL, and snippet."
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
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{body_json, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn serper_response() -> serde_json::Value {
        serde_json::json!({
            "searchParameters": { "q": "rust async", "type": "search", "engine": "google" },
            "knowledgeGraph": {
                "title": "Rust Programming Language",
                "description": "Rust is a multi-paradigm, general-purpose programming language."
            },
            "organic": [
                {
                    "title": "Understanding Async Await in Rust",
                    "link": "https://tokio.rs/tokio/tutorial",
                    "snippet": "The Tokio runtime powers async Rust applications.",
                    "position": 1
                },
                {
                    "title": "Rust by Example - Async/Await",
                    "link": "https://doc.rust-lang.org/rust-by-example/async/await.html",
                    "snippet": "Async functions in Rust return a Future.",
                    "position": 2
                }
            ],
            "peopleAlsoAsk": [
                {
                    "question": "Is Rust good for async programming?",
                    "snippet": "Yes, Rust has first-class async/await support."
                }
            ]
        })
    }

    #[tokio::test]
    async fn test_serper_response_parsing() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/search"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serper_response()))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebSearchTool::new(format!("{}/search", server.uri()));
        let args = serde_json::json!({ "query": "rust async" });
        let result = tool.execute(args).await.expect("execute should succeed");

        let results: Vec<SearchResult> =
            serde_json::from_str(&result).expect("should parse results");

        // Knowledge graph + 2 organic = 3 results
        assert_eq!(results.len(), 3);

        // First result is knowledge graph
        assert_eq!(results[0].title, "Rust Programming Language");
        assert!(results[0]
            .snippet
            .contains("multi-paradigm, general-purpose"));
        assert_eq!(results[0].position, Some(0));

        // Organic results
        assert_eq!(results[1].title, "Understanding Async Await in Rust");
        assert_eq!(results[1].url, "https://tokio.rs/tokio/tutorial");
        assert_eq!(results[1].position, Some(1));

        assert_eq!(results[2].title, "Rust by Example - Async/Await");
        assert_eq!(results[2].position, Some(2));
    }

    #[tokio::test]
    async fn test_post_body_contains_query() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/search"))
            .and(body_json(serde_json::json!({ "q": "hello world" })))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_json(serde_json::json!({ "organic": [] })),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebSearchTool::new(format!("{}/search", server.uri()));
        let args = serde_json::json!({ "query": "hello world" });
        let result = tool.execute(args).await.expect("execute should succeed");

        let results: Vec<SearchResult> =
            serde_json::from_str(&result).expect("should parse results");
        assert!(results.is_empty());
    }

    #[tokio::test]
    async fn test_empty_query_returns_error() {
        let tool = WebSearchTool::new("http://unused".to_string());
        let args = serde_json::json!({ "query": "  " });
        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("must not be empty"));
    }

    #[test]
    fn test_parse_empty_organic() {
        let body = r#"{ "organic": [] }"#;
        let results = parse_serper_response(body).expect("parse");
        assert!(results.is_empty());
    }

    #[test]
    fn test_parse_missing_knowledge_graph() {
        let body = r#"{
            "organic": [{
                "title": "Example",
                "link": "https://example.com",
                "snippet": "An example.",
                "position": 1
            }]
        }"#;
        let results = parse_serper_response(body).expect("parse");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].title, "Example");
        assert_eq!(results[0].url, "https://example.com");
        assert_eq!(results[0].position, Some(1));
    }

    #[test]
    fn test_parse_knowledge_graph_with_attributes() {
        let body = r#"{
            "knowledgeGraph": {
                "title": "Rust",
                "description": "A systems language.",
                "attributes": {
                    "Developer": "Mozilla",
                    "License": "MIT"
                }
            },
            "organic": []
        }"#;
        let results = parse_serper_response(body).expect("parse");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].title, "Rust");
        assert!(results[0].snippet.contains("systems language"));
        // Attributes are appended (order may vary in HashMap, so just check they're present)
        assert!(
            results[0].snippet.contains("Mozilla") || results[0].snippet.contains("MIT"),
            "snippet should contain attribute values"
        );
    }

    #[test]
    fn test_parse_invalid_json() {
        let result = parse_serper_response("not json");
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Failed to parse JSON"));
    }
}
