use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

use crate::ToolExecutor;

/// Default maximum content length in characters.
const DEFAULT_MAX_LENGTH: usize = 50_000;

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
    /// The URL to fetch content from.
    pub url: String,
    /// Maximum content length in characters. Defaults to 50000.
    pub max_length: Option<usize>,
}

/// Web fetch tool that retrieves a URL and extracts readable content as Markdown.
///
/// Uses a two-stage extraction pipeline:
/// 1. `dom_smoothie::Readability` for article extraction (Mozilla Readability algorithm)
/// 2. Fallback: `htmd::convert()` on the raw HTML
pub struct WebFetchTool {
    client: reqwest::Client,
}

impl WebFetchTool {
    /// Create a new `WebFetchTool` with a pre-configured HTTP client.
    pub fn new(client: reqwest::Client) -> Self {
        Self { client }
    }

    /// Create a new `WebFetchTool` with default client settings.
    pub fn with_defaults() -> Self {
        let client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .redirect(reqwest::redirect::Policy::limited(10))
            .build()
            .expect("Failed to build reqwest client");
        Self { client }
    }

    /// Fetch a URL and extract its content as Markdown.
    pub async fn fetch(&self, url: &str, max_length: usize) -> Result<FetchResult, String> {
        // 1. HTTP GET with timeout and redirect following
        let response = self
            .client
            .get(url)
            .timeout(Duration::from_secs(30))
            .send()
            .await
            .map_err(|e| {
                if e.is_timeout() {
                    format!("Request timed out: {e}")
                } else if e.is_redirect() {
                    format!("Too many redirects: {e}")
                } else {
                    format!("Request failed: {e}")
                }
            })?;

        let status = response.status();
        let final_url = response.url().to_string();

        // Check for HTTP errors
        if !status.is_success() {
            return Err(format!("HTTP error {status} for URL: {final_url}"));
        }

        // Check content type - only process HTML
        let content_type = response
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("")
            .to_lowercase();

        if !content_type.is_empty()
            && !content_type.contains("text/html")
            && !content_type.contains("application/xhtml")
        {
            return Err(format!(
                "Non-HTML content type: {content_type}. Only HTML pages are supported."
            ));
        }

        let html = response
            .text()
            .await
            .map_err(|e| format!("Failed to read response body: {e}"))?;

        if html.trim().is_empty() {
            return Ok(FetchResult {
                title: None,
                content: String::new(),
                url: final_url,
            });
        }

        // 2. Try dom_smoothie readability extraction first
        if let Some(article) = extract_article(&html, &final_url) {
            let title = if article.title.is_empty() {
                None
            } else {
                Some(article.title)
            };
            let content = truncate_to_char_limit(&article.text_content, max_length);
            return Ok(FetchResult {
                title,
                content,
                url: final_url,
            });
        }

        // 3. Fallback: convert full HTML to markdown with htmd
        let markdown = fallback_convert(&html);
        let content = truncate_to_char_limit(&markdown, max_length);

        Ok(FetchResult {
            title: None,
            content,
            url: final_url,
        })
    }
}

/// Try to extract article content using dom_smoothie's Readability algorithm.
/// Returns `None` if extraction fails (e.g., non-article pages).
fn extract_article(html: &str, url: &str) -> Option<dom_smoothie::Article> {
    let config = dom_smoothie::Config {
        text_mode: dom_smoothie::TextMode::Markdown,
        ..Default::default()
    };

    let mut readability = dom_smoothie::Readability::new(html, Some(url), Some(config)).ok()?;
    readability.parse().ok()
}

/// Fallback conversion: convert raw HTML to Markdown using htmd.
/// htmd handles script/style removal internally.
fn fallback_convert(html: &str) -> String {
    htmd::convert(html).unwrap_or_default()
}

/// Truncate a string to the given character limit, breaking at a character boundary.
fn truncate_to_char_limit(s: &str, max_chars: usize) -> String {
    if s.chars().count() <= max_chars {
        return s.to_string();
    }

    let mut result: String = s.chars().take(max_chars).collect();
    result.push_str("\n\n[Content truncated...]");
    result
}

#[async_trait]
impl ToolExecutor for WebFetchTool {
    fn name(&self) -> &str {
        "web_fetch"
    }

    fn description(&self) -> &str {
        "Fetch a URL and extract its content as Markdown. Uses article extraction when possible, with a fallback to full HTML-to-Markdown conversion."
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

        let result = self.fetch(&args.url, max_length).await?;

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize result: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    // -----------------------------------------------------------------------
    // Article extraction from fixture HTML
    // -----------------------------------------------------------------------

    #[test]
    fn test_extract_article_from_fixture() {
        let html = include_str!("../../../fixtures/html/article.html");
        let article = extract_article(html, "https://example.com/article");
        assert!(
            article.is_some(),
            "Should extract article from fixture HTML"
        );

        let article = article.unwrap();
        assert!(!article.title.is_empty(), "Article should have a title");
        // The article content should contain the main body text
        let text = article.text_content.to_string();
        assert!(
            text.contains("quantum computing"),
            "Article text should contain the main content about quantum computing"
        );
    }

    #[test]
    fn test_nonarticle_fallback_to_htmd() {
        let html = include_str!("../../../fixtures/html/nonarticle.html");
        // dom_smoothie may or may not extract from a dashboard page
        // The important thing is that fallback_convert produces output
        let markdown = fallback_convert(html);
        assert!(
            !markdown.is_empty(),
            "Fallback conversion should produce output"
        );
        // Should contain some text from the dashboard page
        assert!(
            markdown.contains("Dashboard") || markdown.contains("dashboard"),
            "Fallback should contain dashboard text"
        );
    }

    // -----------------------------------------------------------------------
    // Truncation
    // -----------------------------------------------------------------------

    #[test]
    fn test_truncation_at_max_length() {
        let long_text = "a".repeat(100);
        let truncated = truncate_to_char_limit(&long_text, 50);
        // Should be 50 chars + "\n\n[Content truncated...]"
        assert!(truncated.starts_with(&"a".repeat(50)));
        assert!(truncated.ends_with("[Content truncated...]"));
        assert!(truncated.len() < long_text.len() + 30);
    }

    #[test]
    fn test_truncation_not_applied_when_under_limit() {
        let text = "Hello, world!";
        let result = truncate_to_char_limit(text, 1000);
        assert_eq!(result, text);
    }

    #[test]
    fn test_truncation_exact_boundary() {
        let text = "abcde";
        let result = truncate_to_char_limit(text, 5);
        assert_eq!(result, "abcde");
    }

    #[test]
    fn test_truncation_with_multibyte_chars() {
        // Each emoji is multiple bytes but 1 char
        let text = "\u{1F600}\u{1F601}\u{1F602}\u{1F603}\u{1F604}";
        let result = truncate_to_char_limit(text, 3);
        assert_eq!(result.chars().take(3).count(), 3);
        assert!(result.ends_with("[Content truncated...]"));
    }

    // -----------------------------------------------------------------------
    // WebFetchTool with mock server
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_fetch_html_page() {
        let server = MockServer::start().await;

        let html = r#"<html>
        <head><title>Test Page</title></head>
        <body>
            <article>
                <h1>Test Article</h1>
                <p>This is a test paragraph with some content about Rust programming.</p>
                <p>Rust is a systems programming language focused on safety and performance.</p>
                <p>It provides memory safety without garbage collection through its ownership system.</p>
                <p>The borrow checker ensures references are always valid and prevents data races.</p>
                <p>Rust's type system and ownership model guarantee memory safety at compile time.</p>
                <p>Developers use Rust to build reliable and efficient software applications.</p>
                <p>The language has a growing ecosystem with many useful libraries and tools.</p>
                <p>Rust was voted most loved programming language for several consecutive years.</p>
            </article>
        </body>
        </html>"#;

        Mock::given(method("GET"))
            .and(path("/page"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(&format!("{}/page", server.uri()), DEFAULT_MAX_LENGTH)
            .await
            .expect("fetch should succeed");

        assert!(!result.content.is_empty());
        assert_eq!(result.url, format!("{}/page", server.uri()));
    }

    #[tokio::test]
    async fn test_fetch_non_html_content_type() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/data.json"))
            .respond_with(
                ResponseTemplate::new(200).set_body_raw(r#"{"key": "value"}"#, "application/json"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(&format!("{}/data.json", server.uri()), DEFAULT_MAX_LENGTH)
            .await
            .unwrap_err();

        assert!(err.contains("Non-HTML content type"));
    }

    #[tokio::test]
    async fn test_fetch_http_error_status() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/missing"))
            .respond_with(ResponseTemplate::new(404).set_body_string("Not Found"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(&format!("{}/missing", server.uri()), DEFAULT_MAX_LENGTH)
            .await
            .unwrap_err();

        assert!(err.contains("HTTP error"));
        assert!(err.contains("404"));
    }

    #[tokio::test]
    async fn test_fetch_empty_page() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/empty"))
            .respond_with(ResponseTemplate::new(200).set_body_raw("", "text/html"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(&format!("{}/empty", server.uri()), DEFAULT_MAX_LENGTH)
            .await
            .expect("fetch should succeed");

        assert!(result.content.is_empty());
    }

    #[tokio::test]
    async fn test_execute_empty_url_returns_error() {
        let tool = WebFetchTool::with_defaults();
        let args = serde_json::json!({ "url": "  " });
        let err = tool.execute(args).await.unwrap_err();
        assert!(err.contains("must not be empty"));
    }

    #[tokio::test]
    async fn test_fetch_truncates_long_content() {
        let server = MockServer::start().await;

        // Generate a long HTML page
        let long_paragraph = "x".repeat(1000);
        let html = format!(
            r#"<html><body><article>
            <h1>Long Article</h1>
            <p>{long_paragraph}</p>
            <p>{long_paragraph}</p>
            <p>{long_paragraph}</p>
            </article></body></html>"#
        );

        Mock::given(method("GET"))
            .and(path("/long"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(&format!("{}/long", server.uri()), 100)
            .await
            .expect("fetch should succeed");

        // The content should be truncated to approximately max_length
        // The first 100 chars + truncation marker
        assert!(result.content.contains("[Content truncated...]"));
    }

    // -----------------------------------------------------------------------
    // Fallback conversion
    // -----------------------------------------------------------------------

    #[test]
    fn test_fallback_convert_basic_html() {
        let html = "<html><body><h1>Title</h1><p>Paragraph text.</p></body></html>";
        let md = fallback_convert(html);
        assert!(md.contains("Title"));
        assert!(md.contains("Paragraph text."));
    }

    #[test]
    fn test_fallback_convert_empty_html() {
        let md = fallback_convert("");
        assert!(md.is_empty() || md.trim().is_empty());
    }
}
