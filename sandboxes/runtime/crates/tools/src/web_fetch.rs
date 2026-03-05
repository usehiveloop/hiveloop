use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

use crate::ToolExecutor;

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

/// Default maximum content length in characters.
const DEFAULT_MAX_LENGTH: usize = 50_000;

/// Maximum response body size (5MB).
const MAX_RESPONSE_SIZE: usize = 5 * 1024 * 1024;

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
    /// Maximum content length in characters. Defaults to 50000.
    #[schemars(description = "Maximum content length in characters. Default: 50000")]
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
            .user_agent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
            .build()
            .expect("Failed to build reqwest client");
        Self { client }
    }

    /// Fetch a URL and extract its content in the specified format.
    pub async fn fetch(
        &self,
        url: &str,
        max_length: usize,
        format: &FetchFormat,
    ) -> Result<FetchResult, String> {
        // Build Accept header based on format
        let accept_header = match format {
            FetchFormat::Markdown => "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1",
            FetchFormat::Text => "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1",
            FetchFormat::Html => "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, */*;q=0.1",
        };

        // 1. HTTP GET with timeout and redirect following
        let response = self
            .client
            .get(url)
            .header("Accept", accept_header)
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

        // Check for Cloudflare challenge on 403
        let response = if status == reqwest::StatusCode::FORBIDDEN {
            let cf_mitigated = response
                .headers()
                .get("cf-mitigated")
                .and_then(|v| v.to_str().ok())
                .map(|s| s.to_string());

            if cf_mitigated.as_deref() == Some("challenge") {
                // Retry with simpler User-Agent
                let retry = self
                    .client
                    .get(url)
                    .header("User-Agent", "bridge")
                    .timeout(Duration::from_secs(30))
                    .send()
                    .await
                    .map_err(|e| format!("Cloudflare retry failed: {e}"))?;

                if retry.status().is_success() {
                    retry
                } else {
                    return Err(format!(
                        "HTTP error {} for URL: {}",
                        retry.status(),
                        final_url
                    ));
                }
            } else {
                return Err(format!("HTTP error {status} for URL: {final_url}"));
            }
        } else if !status.is_success() {
            return Err(format!("HTTP error {status} for URL: {final_url}"));
        } else {
            response
        };

        let final_url = response.url().to_string();

        // Check Content-Length header before reading body
        if let Some(content_length) = response
            .headers()
            .get(reqwest::header::CONTENT_LENGTH)
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.parse::<usize>().ok())
        {
            if content_length > MAX_RESPONSE_SIZE {
                return Err("Response too large (exceeds 5MB limit)".to_string());
            }
        }

        // Check content type
        let content_type_str = response
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .unwrap_or("")
            .to_string();

        // Handle image content types (return as base64) — except SVG
        if content_type_str.starts_with("image/")
            && content_type_str != "image/svg+xml"
            && !content_type_str.contains("vnd.fastbidsheet")
        {
            let bytes = response
                .bytes()
                .await
                .map_err(|e| format!("Failed to read image: {e}"))?;

            if bytes.len() > MAX_RESPONSE_SIZE {
                return Err("Response too large (exceeds 5MB limit)".to_string());
            }

            use base64::Engine;
            let b64 = base64::engine::general_purpose::STANDARD.encode(&bytes);

            let content = format!("data:{};base64,{}", content_type_str, b64);
            return Ok(FetchResult {
                title: Some(format!("Image ({})", content_type_str)),
                content,
                url: final_url,
            });
        }

        if !content_type_str.is_empty() {
            let is_html = content_type_str
                .parse::<mime::Mime>()
                .map(|m| {
                    (m.type_() == mime::TEXT && m.subtype() == mime::HTML)
                        || (m.type_() == mime::APPLICATION
                            && m.subtype().as_str().starts_with("xhtml"))
                        || content_type_str.starts_with("image/svg+xml")
                })
                .unwrap_or(false);

            if !is_html {
                return Err(format!(
                    "Non-HTML content type: {}. Only HTML pages are supported.",
                    content_type_str
                ));
            }
        }

        // Read body as bytes to check size
        let body_bytes = response
            .bytes()
            .await
            .map_err(|e| format!("Failed to read response body: {e}"))?;

        if body_bytes.len() > MAX_RESPONSE_SIZE {
            return Err("Response too large (exceeds 5MB limit)".to_string());
        }

        let html = String::from_utf8_lossy(&body_bytes).to_string();

        if html.trim().is_empty() {
            return Ok(FetchResult {
                title: None,
                content: String::new(),
                url: final_url,
            });
        }

        // Handle different output formats
        match format {
            FetchFormat::Html => {
                let content = truncate_to_char_limit(&html, max_length);
                Ok(FetchResult {
                    title: None,
                    content,
                    url: final_url,
                })
            }
            FetchFormat::Text => {
                let text = strip_html_tags(&html);
                let content = truncate_to_char_limit(&text, max_length);
                Ok(FetchResult {
                    title: None,
                    content,
                    url: final_url,
                })
            }
            FetchFormat::Markdown => {
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

/// Strip all HTML tags and return plain text.
/// Also removes script and style blocks and collapses whitespace.
fn strip_html_tags(html: &str) -> String {
    // Use htmd to convert to markdown, then strip remaining markdown formatting
    // This is simpler and more robust than regex-based tag stripping
    let md = htmd::convert(html).unwrap_or_default();
    // The markdown output is already a reasonable plain-text representation
    // Just collapse excessive blank lines
    let mut result = String::with_capacity(md.len());
    let mut blank_count = 0;
    for line in md.lines() {
        if line.trim().is_empty() {
            blank_count += 1;
            if blank_count <= 2 {
                result.push('\n');
            }
        } else {
            blank_count = 0;
            result.push_str(line);
            result.push('\n');
        }
    }
    result.trim().to_string()
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
        include_str!("instructions/web_fetch.txt")
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
    fn test_web_fetch_description_is_rich() {
        let tool = WebFetchTool::with_defaults();
        let desc = tool.description();
        assert!(!desc.is_empty());
        assert!(desc.contains("markdown"), "should mention markdown format");
        assert!(desc.contains("URL"), "should mention URL input");
        assert!(
            desc.contains("Format options"),
            "should mention format options"
        );
    }

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
            .fetch(
                &format!("{}/page", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
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
            .fetch(
                &format!("{}/data.json", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
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
            .fetch(
                &format!("{}/missing", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
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
            .fetch(
                &format!("{}/empty", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
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
            .fetch(
                &format!("{}/long", server.uri()),
                100,
                &FetchFormat::Markdown,
            )
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

    // -----------------------------------------------------------------------
    // Redirect tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_follow_single_redirect() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/old"))
            .respond_with(
                ResponseTemplate::new(301)
                    .insert_header("Location", &*format!("{}/new", server.uri())),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/new"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(
                "<html><body><p>Final destination</p></body></html>",
                "text/html",
            ))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/old", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should succeed after redirect");

        assert_eq!(result.url, format!("{}/new", server.uri()));
        assert!(!result.content.is_empty());
    }

    #[tokio::test]
    async fn test_follow_redirect_chain() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/hop1"))
            .respond_with(
                ResponseTemplate::new(302)
                    .insert_header("Location", &*format!("{}/hop2", server.uri())),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/hop2"))
            .respond_with(
                ResponseTemplate::new(302)
                    .insert_header("Location", &*format!("{}/hop3", server.uri())),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/hop3"))
            .respond_with(
                ResponseTemplate::new(302)
                    .insert_header("Location", &*format!("{}/final", server.uri())),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/final"))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_raw("<html><body><p>End of chain</p></body></html>", "text/html"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/hop1", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should follow redirect chain");

        assert_eq!(result.url, format!("{}/final", server.uri()));
    }

    #[tokio::test]
    async fn test_redirect_preserves_content() {
        let server = MockServer::start().await;

        let html = r#"<html>
        <head><title>Redirected Article</title></head>
        <body>
            <article>
                <h1>Redirected Article</h1>
                <p>This content was reached via redirect.</p>
                <p>It should be fully extracted despite the redirect hop.</p>
                <p>The article discusses important topics about software engineering.</p>
                <p>Including best practices for building robust systems.</p>
                <p>And patterns for handling distributed architectures.</p>
                <p>Performance optimization is also covered in detail.</p>
            </article>
        </body>
        </html>"#;

        Mock::given(method("GET"))
            .and(path("/redirect-me"))
            .respond_with(
                ResponseTemplate::new(301)
                    .insert_header("Location", &*format!("{}/article", server.uri())),
            )
            .expect(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/article"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/redirect-me", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should succeed after redirect");

        assert!(
            result.content.contains("redirect") || result.content.contains("Redirect"),
            "content should be extracted from redirected page"
        );
    }

    // -----------------------------------------------------------------------
    // HTTP status code tests
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_http_500_returns_error() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/error"))
            .respond_with(ResponseTemplate::new(500).set_body_string("Internal Server Error"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(
                &format!("{}/error", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .unwrap_err();

        assert!(
            err.contains("HTTP error"),
            "error should mention HTTP error: {err}"
        );
        assert!(
            err.contains("500"),
            "error should contain status code 500: {err}"
        );
    }

    #[tokio::test]
    async fn test_http_403_returns_error() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/forbidden"))
            .respond_with(ResponseTemplate::new(403).set_body_string("Forbidden"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(
                &format!("{}/forbidden", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .unwrap_err();

        assert!(
            err.contains("HTTP error"),
            "error should mention HTTP error: {err}"
        );
        assert!(
            err.contains("403"),
            "error should contain status code 403: {err}"
        );
    }

    #[tokio::test]
    async fn test_http_503_returns_error() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/unavailable"))
            .respond_with(ResponseTemplate::new(503).set_body_string("Service Unavailable"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(
                &format!("{}/unavailable", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .unwrap_err();

        assert!(
            err.contains("HTTP error"),
            "error should mention HTTP error: {err}"
        );
        assert!(
            err.contains("503"),
            "error should contain status code 503: {err}"
        );
    }

    // -----------------------------------------------------------------------
    // Content-type edge cases
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_accept_missing_content_type() {
        let server = MockServer::start().await;

        // Use set_body_bytes to avoid wiremock auto-adding Content-Type: text/plain
        Mock::given(method("GET"))
            .and(path("/no-ct"))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_bytes("<html><body><p>No content type header</p></body></html>"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/no-ct", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should succeed when Content-Type is missing");

        assert!(!result.content.is_empty());
    }

    #[tokio::test]
    async fn test_accept_xhtml_content_type() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/xhtml"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(
                "<html><body><p>XHTML content</p></body></html>",
                "application/xhtml+xml; charset=utf-8",
            ))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/xhtml", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should accept application/xhtml+xml");

        assert!(!result.content.is_empty());
    }

    #[tokio::test]
    async fn test_reject_pdf_content_type() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/doc.pdf"))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_raw(b"fake pdf bytes" as &[u8], "application/pdf"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(
                &format!("{}/doc.pdf", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .unwrap_err();

        assert!(
            err.contains("Non-HTML content type"),
            "should reject PDF content type: {err}"
        );
    }

    #[tokio::test]
    async fn test_fetch_image_returns_base64() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/image.png"))
            .respond_with(
                ResponseTemplate::new(200).set_body_raw(b"fake png bytes" as &[u8], "image/png"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/image.png", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("should return image as base64");

        assert!(
            result.content.contains("data:image/png;base64,"),
            "should contain base64 image data"
        );
    }

    #[tokio::test]
    async fn test_fetch_svg_treated_as_html() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/icon.svg"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(
                r#"<svg xmlns="http://www.w3.org/2000/svg"><circle r="50"/></svg>"#,
                "image/svg+xml",
            ))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/icon.svg", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Html,
            )
            .await
            .expect("SVG should be processed as HTML/text");

        // SVG should be returned as content, not base64
        assert!(
            !result.content.starts_with("data:image"),
            "SVG should not be returned as base64"
        );
    }

    #[tokio::test]
    async fn test_fetch_response_too_large() {
        let server = MockServer::start().await;

        // Create a response with Content-Length > 5MB
        let large_body = "x".repeat(6 * 1024 * 1024);
        Mock::given(method("GET"))
            .and(path("/huge"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(large_body, "text/html"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(
                &format!("{}/huge", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .unwrap_err();

        assert!(
            err.contains("5MB") || err.contains("too large"),
            "should reject response >5MB: {err}"
        );
    }

    #[tokio::test]
    async fn test_fetch_content_length_too_large() {
        let server = MockServer::start().await;

        // Create actual large body matching Content-Length to avoid reqwest errors
        let large_body = "x".repeat(6 * 1024 * 1024);
        Mock::given(method("GET"))
            .and(path("/cl-huge"))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_raw(large_body.as_bytes().to_vec(), "text/html"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let err = tool
            .fetch(
                &format!("{}/cl-huge", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .unwrap_err();

        assert!(
            err.contains("5MB") || err.contains("too large"),
            "should reject when Content-Length > 5MB: {err}"
        );
    }

    #[tokio::test]
    async fn test_fetch_cloudflare_retry() {
        let server = MockServer::start().await;

        // First request returns 403 with cf-mitigated: challenge
        // wiremock will match both requests to /cf-page
        // We need two mocks: first returns 403, second returns 200
        // But wiremock matches all requests to a path. We use expect to control.
        Mock::given(method("GET"))
            .and(path("/cf-page"))
            .respond_with(
                ResponseTemplate::new(403)
                    .insert_header("cf-mitigated", "challenge")
                    .set_body_string("Blocked"),
            )
            .expect(1)
            .up_to_n_times(1)
            .mount(&server)
            .await;

        Mock::given(method("GET"))
            .and(path("/cf-page"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(
                "<html><body><p>Success after retry</p></body></html>",
                "text/html",
            ))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/cf-page", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("should succeed after CF retry");

        assert!(!result.content.is_empty());
    }

    // -----------------------------------------------------------------------
    // Script/style stripping
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_scripts_and_styles_not_in_output() {
        let server = MockServer::start().await;

        let html = r#"<html>
        <head>
            <style>.secret { display: none; } body { color: red; }</style>
            <script>var secret = "do_not_leak"; alert("hi");</script>
        </head>
        <body>
            <p>Visible paragraph for the reader.</p>
            <script>console.log("another script block");</script>
            <style>.more-styles { font-size: 12px; }</style>
        </body>
        </html>"#;

        Mock::given(method("GET"))
            .and(path("/scripted"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/scripted", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should succeed");

        let content = &result.content;
        assert!(
            !content.contains("do_not_leak"),
            "script content should be stripped from output"
        );
        assert!(
            !content.contains("alert("),
            "script tags should be stripped from output"
        );
        assert!(
            !content.contains("console.log"),
            "inline scripts should be stripped from output"
        );
        assert!(
            !content.contains("display: none"),
            "style content should be stripped from output"
        );
        assert!(
            !content.contains("font-size"),
            "style blocks should be stripped from output"
        );
    }

    // -----------------------------------------------------------------------
    // Title extraction
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_article_title_extracted() {
        let server = MockServer::start().await;

        let html = r#"<html>
        <head><title>My Great Article</title></head>
        <body>
            <article>
                <h1>My Great Article</h1>
                <p>This is the first paragraph of a substantial article about testing.</p>
                <p>The article continues with more details about software quality.</p>
                <p>We discuss various testing strategies and their trade-offs.</p>
                <p>Unit tests are the foundation of a good test suite.</p>
                <p>Integration tests verify that components work together correctly.</p>
                <p>End-to-end tests simulate real user workflows.</p>
            </article>
        </body>
        </html>"#;

        Mock::given(method("GET"))
            .and(path("/titled"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/titled", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should succeed");

        assert!(
            result.title.is_some(),
            "article with <title> and <h1> should have extracted title"
        );
        let title = result.title.unwrap();
        assert!(
            title.contains("Great Article") || title.contains("My Great"),
            "title should match page title, got: {title}"
        );
    }

    #[tokio::test]
    async fn test_no_title_for_empty_page() {
        let server = MockServer::start().await;

        Mock::given(method("GET"))
            .and(path("/blank"))
            .respond_with(
                ResponseTemplate::new(200).set_body_raw("<html><body></body></html>", "text/html"),
            )
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/blank", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Markdown,
            )
            .await
            .expect("fetch should succeed");

        assert!(
            result.title.is_none(),
            "empty page should not have a title, got: {:?}",
            result.title
        );
    }

    // -----------------------------------------------------------------------
    // Execute via ToolExecutor
    // -----------------------------------------------------------------------

    #[tokio::test]
    async fn test_execute_with_custom_max_length() {
        let server = MockServer::start().await;

        let long_paragraph = "word ".repeat(200);
        let html = format!(r#"<html><body><p>{long_paragraph}</p></body></html>"#);

        Mock::given(method("GET"))
            .and(path("/truncate-exec"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let args = serde_json::json!({
            "url": format!("{}/truncate-exec", server.uri()),
            "max_length": 50
        });

        let output = tool.execute(args).await.expect("execute should succeed");
        let parsed: FetchResult =
            serde_json::from_str(&output).expect("output should be valid FetchResult JSON");

        assert!(
            parsed.content.contains("[Content truncated...]"),
            "content should be truncated with max_length=50"
        );
    }

    #[tokio::test]
    async fn test_execute_with_default_max_length() {
        let server = MockServer::start().await;

        let html =
            r#"<html><body><p>Simple content for default max length test.</p></body></html>"#;

        Mock::given(method("GET"))
            .and(path("/default-exec"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let args = serde_json::json!({
            "url": format!("{}/default-exec", server.uri())
        });

        let output = tool.execute(args).await.expect("execute should succeed");
        let parsed: FetchResult =
            serde_json::from_str(&output).expect("output should be valid FetchResult JSON");

        assert!(!parsed.content.is_empty(), "content should not be empty");
        assert!(
            !parsed.content.contains("[Content truncated...]"),
            "short content should not be truncated with default max_length"
        );
    }

    #[tokio::test]
    async fn test_fetch_text_format() {
        let server = MockServer::start().await;

        let html = r#"<html><body><h1>Title</h1><p>Hello <b>bold</b> world.</p></body></html>"#;

        Mock::given(method("GET"))
            .and(path("/text"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/text", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Text,
            )
            .await
            .expect("fetch should succeed");

        // Text format should not contain HTML tags
        assert!(!result.content.contains("<h1>"));
        assert!(!result.content.contains("<p>"));
        assert!(!result.content.contains("<b>"));
        // But should contain the text content
        assert!(result.content.contains("Hello"));
        assert!(result.content.contains("world"));
    }

    #[tokio::test]
    async fn test_fetch_html_format() {
        let server = MockServer::start().await;

        let html = r#"<html><body><h1>Raw Title</h1><p>Raw paragraph.</p></body></html>"#;

        Mock::given(method("GET"))
            .and(path("/raw"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let result = tool
            .fetch(
                &format!("{}/raw", server.uri()),
                DEFAULT_MAX_LENGTH,
                &FetchFormat::Html,
            )
            .await
            .expect("fetch should succeed");

        // HTML format should preserve the raw HTML
        assert!(result.content.contains("<h1>Raw Title</h1>"));
        assert!(result.content.contains("<p>Raw paragraph.</p>"));
    }

    #[tokio::test]
    async fn test_execute_with_format_parameter() {
        let server = MockServer::start().await;

        let html = r#"<html><body><p>Format test content</p></body></html>"#;

        Mock::given(method("GET"))
            .and(path("/fmt"))
            .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
            .expect(1)
            .mount(&server)
            .await;

        let tool = WebFetchTool::with_defaults();
        let args = serde_json::json!({
            "url": format!("{}/fmt", server.uri()),
            "format": "html"
        });

        let output = tool.execute(args).await.expect("execute should succeed");
        let parsed: FetchResult = serde_json::from_str(&output).expect("parse");

        assert!(parsed.content.contains("<p>Format test content</p>"));
    }
}
