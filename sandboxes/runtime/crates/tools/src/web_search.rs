use async_trait::async_trait;
use backon::{ExponentialBuilder, Retryable};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

use crate::ToolExecutor;

/// Supported search API providers.
#[derive(Debug, Clone)]
pub enum SearchProvider {
    Brave,
    Tavily,
    SerpApi,
}

/// A single search result.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SearchResult {
    pub title: String,
    pub url: String,
    pub snippet: String,
}

/// Arguments for the WebSearch tool.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebSearchArgs {
    /// The search query string.
    pub query: String,
    /// Maximum number of results to return. Defaults to 10.
    pub max_results: Option<u32>,
}

/// Web search tool that queries a configurable search API provider.
pub struct WebSearchTool {
    client: reqwest::Client,
    provider: SearchProvider,
    api_key: String,
}

impl WebSearchTool {
    /// Create a new `WebSearchTool`.
    pub fn new(client: reqwest::Client, provider: SearchProvider, api_key: String) -> Self {
        Self {
            client,
            provider,
            api_key,
        }
    }

    /// Build the request URL for the given provider and query parameters.
    fn build_request_url(&self, query: &str, max_results: u32) -> String {
        match &self.provider {
            SearchProvider::Brave => {
                format!(
                    "https://api.search.brave.com/res/v1/web/search?q={}&count={}",
                    urlencoded(query),
                    max_results
                )
            }
            SearchProvider::Tavily => {
                // Tavily uses POST with JSON body, but we still construct the endpoint URL.
                "https://api.tavily.com/search".to_string()
            }
            SearchProvider::SerpApi => {
                format!(
                    "https://serpapi.com/search.json?q={}&num={}&api_key={}",
                    urlencoded(query),
                    max_results,
                    urlencoded(&self.api_key)
                )
            }
        }
    }

    /// Execute the search request with retry logic.
    async fn search_with_retry(
        &self,
        query: String,
        max_results: u32,
    ) -> Result<Vec<SearchResult>, String> {
        let client = &self.client;
        let provider = &self.provider;
        let api_key = &self.api_key;

        let url = self.build_request_url(&query, max_results);

        let do_request = || async {
            let response = match provider {
                SearchProvider::Brave => client
                    .get(&url)
                    .header("Accept", "application/json")
                    .header("Accept-Encoding", "gzip")
                    .header("X-Subscription-Token", api_key.as_str())
                    .timeout(Duration::from_secs(15))
                    .send()
                    .await
                    .map_err(|e| format!("Request failed: {e}"))?,
                SearchProvider::Tavily => {
                    let body = serde_json::json!({
                        "api_key": api_key,
                        "query": query,
                        "max_results": max_results,
                    });
                    client
                        .post(&url)
                        .json(&body)
                        .timeout(Duration::from_secs(15))
                        .send()
                        .await
                        .map_err(|e| format!("Request failed: {e}"))?
                }
                SearchProvider::SerpApi => client
                    .get(&url)
                    .timeout(Duration::from_secs(15))
                    .send()
                    .await
                    .map_err(|e| format!("Request failed: {e}"))?,
            };

            let status = response.status();
            let body = response
                .text()
                .await
                .map_err(|e| format!("Failed to read response body: {e}"))?;

            if status.is_server_error() {
                return Err(format!("Server error {status}: {body}"));
            }

            if !status.is_success() {
                return Err(format!("HTTP {status}: {body}"));
            }

            parse_response(provider, &body)
        };

        do_request
            .retry(
                ExponentialBuilder::default()
                    .with_min_delay(Duration::from_millis(500))
                    .with_max_delay(Duration::from_secs(5))
                    .with_max_times(3),
            )
            .when(|e: &String| {
                // Retry only on server errors (transient)
                e.starts_with("Server error") || e.starts_with("Request failed")
            })
            .await
    }
}

/// URL-encode a string for query parameters.
fn urlencoded(s: &str) -> String {
    // Minimal percent-encoding for URL query values.
    let mut result = String::with_capacity(s.len());
    for byte in s.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                result.push(byte as char)
            }
            b' ' => result.push('+'),
            _ => {
                result.push('%');
                result.push_str(&format!("{byte:02X}"));
            }
        }
    }
    result
}

/// Parse the JSON response from the search provider into a Vec<SearchResult>.
fn parse_response(provider: &SearchProvider, body: &str) -> Result<Vec<SearchResult>, String> {
    let json: serde_json::Value =
        serde_json::from_str(body).map_err(|e| format!("Failed to parse JSON response: {e}"))?;

    match provider {
        SearchProvider::Brave => parse_brave_response(&json),
        SearchProvider::Tavily => parse_tavily_response(&json),
        SearchProvider::SerpApi => parse_serpapi_response(&json),
    }
}

fn parse_brave_response(json: &serde_json::Value) -> Result<Vec<SearchResult>, String> {
    let results = json
        .get("web")
        .and_then(|w| w.get("results"))
        .and_then(|r| r.as_array())
        .unwrap_or(&Vec::new())
        .iter()
        .filter_map(|item| {
            let title = item.get("title")?.as_str()?.to_string();
            let url = item.get("url")?.as_str()?.to_string();
            let snippet = item
                .get("description")
                .and_then(|d| d.as_str())
                .unwrap_or("")
                .to_string();
            Some(SearchResult {
                title,
                url,
                snippet,
            })
        })
        .collect();

    Ok(results)
}

fn parse_tavily_response(json: &serde_json::Value) -> Result<Vec<SearchResult>, String> {
    let results = json
        .get("results")
        .and_then(|r| r.as_array())
        .unwrap_or(&Vec::new())
        .iter()
        .filter_map(|item| {
            let title = item.get("title")?.as_str()?.to_string();
            let url = item.get("url")?.as_str()?.to_string();
            let snippet = item
                .get("content")
                .and_then(|c| c.as_str())
                .unwrap_or("")
                .to_string();
            Some(SearchResult {
                title,
                url,
                snippet,
            })
        })
        .collect();

    Ok(results)
}

fn parse_serpapi_response(json: &serde_json::Value) -> Result<Vec<SearchResult>, String> {
    let results = json
        .get("organic_results")
        .and_then(|r| r.as_array())
        .unwrap_or(&Vec::new())
        .iter()
        .filter_map(|item| {
            let title = item.get("title")?.as_str()?.to_string();
            let url = item.get("link")?.as_str()?.to_string();
            let snippet = item
                .get("snippet")
                .and_then(|s| s.as_str())
                .unwrap_or("")
                .to_string();
            Some(SearchResult {
                title,
                url,
                snippet,
            })
        })
        .collect();

    Ok(results)
}

#[async_trait]
impl ToolExecutor for WebSearchTool {
    fn name(&self) -> &str {
        "web_search"
    }

    fn description(&self) -> &str {
        "Search the web using a configurable search API provider. Returns structured results with title, URL, and snippet."
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebSearchArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebSearchArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        let max_results = args.max_results.unwrap_or(10);

        if args.query.trim().is_empty() {
            return Err("Search query must not be empty".to_string());
        }

        let results = self.search_with_retry(args.query, max_results).await?;

        serde_json::to_string(&results).map_err(|e| format!("Failed to serialize results: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{header, method, path, query_param};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    // -----------------------------------------------------------------------
    // Brave provider tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_brave_request_url_and_response_parsing() {
        let server = MockServer::start().await;

        let brave_response = serde_json::json!({
            "web": {
                "results": [
                    {
                        "title": "Rust Programming Language",
                        "url": "https://www.rust-lang.org/",
                        "description": "A language empowering everyone to build reliable software."
                    },
                    {
                        "title": "Rust Wikipedia",
                        "url": "https://en.wikipedia.org/wiki/Rust_(programming_language)",
                        "description": "Rust is a multi-paradigm programming language."
                    }
                ]
            }
        });

        Mock::given(method("GET"))
            .and(path("/res/v1/web/search"))
            .and(query_param("q", "rust programming"))
            .and(query_param("count", "5"))
            .and(header("X-Subscription-Token", "brave-key-123"))
            .respond_with(ResponseTemplate::new(200).set_body_json(&brave_response))
            .expect(1)
            .mount(&server)
            .await;

        // Override the URL by creating a tool that uses the mock server.
        let client = reqwest::Client::new();
        let tool = WebSearchTool {
            client,
            provider: SearchProvider::Brave,
            api_key: "brave-key-123".to_string(),
        };

        // We need to manually construct and call the mock server URL.
        let url = format!(
            "{}/res/v1/web/search?q={}&count={}",
            server.uri(),
            urlencoded("rust programming"),
            5
        );
        let response = tool
            .client
            .get(&url)
            .header("Accept", "application/json")
            .header("Accept-Encoding", "gzip")
            .header("X-Subscription-Token", &tool.api_key)
            .send()
            .await
            .expect("request");

        let body = response.text().await.expect("body");
        let results = parse_response(&SearchProvider::Brave, &body).expect("parse");

        assert_eq!(results.len(), 2);
        assert_eq!(results[0].title, "Rust Programming Language");
        assert_eq!(results[0].url, "https://www.rust-lang.org/");
        assert!(results[0].snippet.contains("empowering everyone"));
        assert_eq!(results[1].title, "Rust Wikipedia");
    }

    // -----------------------------------------------------------------------
    // Tavily provider tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_tavily_request_and_response_parsing() {
        let server = MockServer::start().await;

        let tavily_response = serde_json::json!({
            "results": [
                {
                    "title": "Learn Rust",
                    "url": "https://doc.rust-lang.org/book/",
                    "content": "The Rust Programming Language book."
                }
            ]
        });

        Mock::given(method("POST"))
            .and(path("/search"))
            .respond_with(ResponseTemplate::new(200).set_body_json(&tavily_response))
            .expect(1)
            .mount(&server)
            .await;

        let client = reqwest::Client::new();
        let body = serde_json::json!({
            "api_key": "<search-provider-api-key>",
            "query": "rust book",
            "max_results": 5,
        });

        let response = client
            .post(format!("{}/search", server.uri()))
            .json(&body)
            .send()
            .await
            .expect("request");

        let response_body = response.text().await.expect("body");
        let results = parse_response(&SearchProvider::Tavily, &response_body).expect("parse");

        assert_eq!(results.len(), 1);
        assert_eq!(results[0].title, "Learn Rust");
        assert_eq!(results[0].url, "https://doc.rust-lang.org/book/");
        assert_eq!(results[0].snippet, "The Rust Programming Language book.");
    }

    // -----------------------------------------------------------------------
    // SerpApi provider tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_serpapi_request_url_and_response_parsing() {
        let server = MockServer::start().await;

        let serpapi_response = serde_json::json!({
            "organic_results": [
                {
                    "title": "Cargo - The Rust Package Manager",
                    "link": "https://doc.rust-lang.org/cargo/",
                    "snippet": "Cargo is the Rust package manager."
                },
                {
                    "title": "crates.io",
                    "link": "https://crates.io/",
                    "snippet": "The Rust community's crate registry."
                }
            ]
        });

        Mock::given(method("GET"))
            .and(path("/search.json"))
            .and(query_param("q", "rust cargo"))
            .and(query_param("num", "10"))
            .and(query_param("api_key", "serp-key-789"))
            .respond_with(ResponseTemplate::new(200).set_body_json(&serpapi_response))
            .expect(1)
            .mount(&server)
            .await;

        let client = reqwest::Client::new();
        let url = format!(
            "{}/search.json?q={}&num={}&api_key={}",
            server.uri(),
            urlencoded("rust cargo"),
            10,
            urlencoded("serp-key-789")
        );

        let response = client.get(&url).send().await.expect("request");

        let body = response.text().await.expect("body");
        let results = parse_response(&SearchProvider::SerpApi, &body).expect("parse");

        assert_eq!(results.len(), 2);
        assert_eq!(results[0].title, "Cargo - The Rust Package Manager");
        assert_eq!(results[0].url, "https://doc.rust-lang.org/cargo/");
        assert_eq!(results[0].snippet, "Cargo is the Rust package manager.");
        assert_eq!(results[1].title, "crates.io");
    }

    // -----------------------------------------------------------------------
    // URL encoding
    // -----------------------------------------------------------------------

    #[test]
    fn test_urlencoded() {
        assert_eq!(urlencoded("hello world"), "hello+world");
        assert_eq!(urlencoded("a&b=c"), "a%26b%3Dc");
        assert_eq!(urlencoded("simple"), "simple");
        assert_eq!(urlencoded("foo bar+baz"), "foo+bar%2Bbaz");
    }

    // -----------------------------------------------------------------------
    // Build request URL format
    // -----------------------------------------------------------------------

    #[test]
    fn test_brave_build_request_url() {
        let tool = WebSearchTool::new(
            reqwest::Client::new(),
            SearchProvider::Brave,
            "test-key".to_string(),
        );
        let url = tool.build_request_url("rust lang", 5);
        assert_eq!(
            url,
            "https://api.search.brave.com/res/v1/web/search?q=rust+lang&count=5"
        );
    }

    #[test]
    fn test_tavily_build_request_url() {
        let tool = WebSearchTool::new(
            reqwest::Client::new(),
            SearchProvider::Tavily,
            "test-key".to_string(),
        );
        let url = tool.build_request_url("rust lang", 5);
        assert_eq!(url, "https://api.tavily.com/search");
    }

    #[test]
    fn test_serpapi_build_request_url() {
        let tool = WebSearchTool::new(
            reqwest::Client::new(),
            SearchProvider::SerpApi,
            "test-key".to_string(),
        );
        let url = tool.build_request_url("rust lang", 5);
        assert_eq!(
            url,
            "https://serpapi.com/search.json?q=rust+lang&num=5&api_key=test-key"
        );
    }

    // -----------------------------------------------------------------------
    // ToolExecutor integration (empty query)
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_execute_empty_query_returns_error() {
        let tool = WebSearchTool::new(
            reqwest::Client::new(),
            SearchProvider::Brave,
            "key".to_string(),
        );
        let args = serde_json::json!({ "query": "  " });
        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("must not be empty"));
    }

    // -----------------------------------------------------------------------
    // Parse empty / missing fields
    // -----------------------------------------------------------------------

    #[test]
    fn test_parse_brave_empty_results() {
        let json = serde_json::json!({ "web": { "results": [] } });
        let results = parse_brave_response(&json).expect("parse");
        assert!(results.is_empty());
    }

    #[test]
    fn test_parse_brave_missing_web_key() {
        let json = serde_json::json!({});
        let results = parse_brave_response(&json).expect("parse");
        assert!(results.is_empty());
    }

    #[test]
    fn test_parse_tavily_empty_results() {
        let json = serde_json::json!({ "results": [] });
        let results = parse_tavily_response(&json).expect("parse");
        assert!(results.is_empty());
    }

    #[test]
    fn test_parse_serpapi_empty_results() {
        let json = serde_json::json!({ "organic_results": [] });
        let results = parse_serpapi_response(&json).expect("parse");
        assert!(results.is_empty());
    }
}
