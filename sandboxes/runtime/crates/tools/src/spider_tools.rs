use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::time::Duration;

use crate::registry::ToolExecutor;
use crate::truncation;

/// Maximum output characters for crawl results.
const MAX_OUTPUT_CHARS: usize = 50_000;

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
    async fn post(&self, path: &str, body: &serde_json::Value) -> Result<String, String> {
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

// ─── Response types ────────────────────────────────────────────────────

#[derive(Debug, Deserialize)]
struct CrawlPage {
    url: String,
    #[serde(default)]
    content: String,
    #[serde(default)]
    error: Option<String>,
}

#[derive(Debug, Deserialize)]
struct SearchResponse {
    #[serde(default)]
    content: Vec<SearchItem>,
}

#[derive(Debug, Deserialize)]
struct SearchItem {
    url: String,
    #[serde(default)]
    title: String,
    #[serde(default)]
    description: String,
}

#[derive(Debug, Deserialize)]
struct LinkPage {
    url: String,
}

#[derive(Debug, Deserialize)]
struct TransformResponse {
    #[serde(default)]
    content: Vec<String>,
}

// ═══════════════════════════════════════════════════════════════════════
// 1. WebCrawlTool
// ═══════════════════════════════════════════════════════════════════════

#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebCrawlArgs {
    /// The URL to start crawling from.
    pub url: String,
    /// Maximum number of pages to crawl. Defaults to 1. Set higher to crawl multiple pages.
    #[serde(default)]
    pub limit: Option<u32>,
    /// Maximum crawl depth from the starting URL. 0 means no limit.
    #[serde(default)]
    pub depth: Option<u32>,
    /// Output format: "markdown" (default), "raw", "text", "html2text".
    #[serde(default)]
    pub return_format: Option<String>,
    /// Rendering mode: "http" (fast), "chrome" (JavaScript), "smart" (auto-detect).
    #[serde(default)]
    pub request: Option<String>,
    /// Extract clean readable content, removing navigation and boilerplate.
    #[serde(default)]
    pub readability: Option<bool>,
}

pub struct WebCrawlTool {
    spider: Arc<SpiderClient>,
}

impl WebCrawlTool {
    pub fn new(spider: Arc<SpiderClient>) -> Self {
        Self { spider }
    }
}

#[async_trait]
impl ToolExecutor for WebCrawlTool {
    fn name(&self) -> &str {
        "web_crawl"
    }

    fn description(&self) -> &str {
        include_str!("instructions/web_crawl.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebCrawlArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebCrawlArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.url.trim().is_empty() {
            return Err("url is required".to_string());
        }

        let body = serde_json::json!({
            "url": args.url,
            "limit": args.limit.unwrap_or(1),
            "depth": args.depth.unwrap_or(0),
            "return_format": args.return_format.as_deref().unwrap_or("markdown"),
            "request": args.request.as_deref().unwrap_or("smart"),
            "readability": args.readability.unwrap_or(true),
            "storageless": true,
            "cache": false
        });

        let response_text = self.spider.post("/spider/crawl", &body).await?;
        let pages: Vec<CrawlPage> = serde_json::from_str(&response_text)
            .map_err(|e| format!("Failed to parse crawl response: {e}"))?;

        if pages.is_empty() {
            return Ok("No pages found.".to_string());
        }

        let mut output = String::new();
        for page in &pages {
            if let Some(ref error) = page.error {
                output.push_str(&format!("## {} [ERROR]\n\n{}\n\n---\n\n", page.url, error));
                continue;
            }
            if page.content.trim().is_empty() {
                output.push_str(&format!("## {} [EMPTY]\n\n---\n\n", page.url));
                continue;
            }
            output.push_str(&format!("## {}\n\n{}\n\n---\n\n", page.url, page.content));
        }

        let truncated =
            truncation::truncate_output(&output, truncation::MAX_LINES, MAX_OUTPUT_CHARS);
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

// ═══════════════════════════════════════════════════════════════════════
// 2. WebSearchTool
// ═══════════════════════════════════════════════════════════════════════

#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebSearchArgs {
    /// The search query.
    pub search: String,
    /// Number of search results to return. Defaults to 5.
    #[serde(default)]
    pub search_limit: Option<u32>,
    /// If true, crawl each result URL and include full page content.
    #[serde(default)]
    pub fetch_page_content: Option<bool>,
    /// Output format when fetch_page_content is true: "markdown" (default), "raw", "text".
    #[serde(default)]
    pub return_format: Option<String>,
}

pub struct WebSearchTool {
    spider: Arc<SpiderClient>,
    description: String,
}

impl WebSearchTool {
    pub fn new(spider: Arc<SpiderClient>) -> Self {
        let year = chrono::Utc::now().format("%Y").to_string();
        let description =
            include_str!("instructions/web_search_spider.txt").replace("{{year}}", &year);
        Self {
            spider,
            description,
        }
    }
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

        if args.search.trim().is_empty() {
            return Err("search query is required".to_string());
        }

        let body = serde_json::json!({
            "search": args.search,
            "search_limit": args.search_limit.unwrap_or(5),
            "fetch_page_content": args.fetch_page_content.unwrap_or(false),
            "return_format": args.return_format.as_deref().unwrap_or("markdown"),
            "storageless": true,
            "cache": false
        });

        let response_text = self.spider.post("/spider/search", &body).await?;
        let response: SearchResponse = serde_json::from_str(&response_text)
            .map_err(|e| format!("Failed to parse search response: {e}"))?;

        if response.content.is_empty() {
            return Ok("No search results found.".to_string());
        }

        let mut output = String::new();
        for (i, item) in response.content.iter().enumerate() {
            let title = if item.title.is_empty() {
                &item.url
            } else {
                &item.title
            };
            output.push_str(&format!("{}. **{}**\n", i + 1, title));
            output.push_str(&format!("   {}\n", item.url));
            if !item.description.is_empty() {
                output.push_str(&format!("   {}\n", item.description));
            }
            output.push('\n');
        }

        let truncated =
            truncation::truncate_output(&output, truncation::MAX_LINES, MAX_OUTPUT_CHARS);
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

// ═══════════════════════════════════════════════════════════════════════
// 3. WebGetLinksTool
// ═══════════════════════════════════════════════════════════════════════

#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebGetLinksArgs {
    /// The URL to extract links from.
    pub url: String,
    /// Maximum number of pages to process. Defaults to 1.
    #[serde(default)]
    pub limit: Option<u32>,
    /// Rendering mode: "http" (fast), "chrome" (JavaScript), "smart" (auto-detect).
    #[serde(default)]
    pub request: Option<String>,
}

pub struct WebGetLinksTool {
    spider: Arc<SpiderClient>,
}

impl WebGetLinksTool {
    pub fn new(spider: Arc<SpiderClient>) -> Self {
        Self { spider }
    }
}

#[async_trait]
impl ToolExecutor for WebGetLinksTool {
    fn name(&self) -> &str {
        "web_get_links"
    }

    fn description(&self) -> &str {
        include_str!("instructions/web_get_links.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebGetLinksArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebGetLinksArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.url.trim().is_empty() {
            return Err("url is required".to_string());
        }

        let body = serde_json::json!({
            "url": args.url,
            "limit": args.limit.unwrap_or(1),
            "request": args.request.as_deref().unwrap_or("http"),
            "storageless": true,
            "cache": false
        });

        let response_text = self.spider.post("/spider/links", &body).await?;
        let pages: Vec<LinkPage> = serde_json::from_str(&response_text)
            .map_err(|e| format!("Failed to parse links response: {e}"))?;

        if pages.is_empty() {
            return Ok("No links found.".to_string());
        }

        let urls: Vec<&str> = pages.iter().map(|p| p.url.as_str()).collect();
        serde_json::to_string_pretty(&urls).map_err(|e| format!("Failed to serialize: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

// ═══════════════════════════════════════════════════════════════════════
// 4. WebScreenshotTool
// ═══════════════════════════════════════════════════════════════════════

#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebScreenshotArgs {
    /// The URL to screenshot.
    pub url: String,
    /// Rendering mode: "http", "chrome" (recommended), "smart".
    #[serde(default)]
    pub request: Option<String>,
    /// Wait for a CSS selector before capturing. Requires "chrome" or "smart" mode.
    #[serde(default)]
    pub wait_for_selector: Option<WaitForSelector>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct WaitForSelector {
    /// CSS selector to wait for.
    pub selector: String,
    /// Maximum wait time in milliseconds.
    #[serde(default)]
    pub timeout: Option<u32>,
}

/// Screenshot result returned to the agent.
#[derive(Debug, Serialize)]
struct ScreenshotResult {
    url: String,
    content: String,
    content_type: String,
}

pub struct WebScreenshotTool {
    spider: Arc<SpiderClient>,
}

impl WebScreenshotTool {
    pub fn new(spider: Arc<SpiderClient>) -> Self {
        Self { spider }
    }
}

#[async_trait]
impl ToolExecutor for WebScreenshotTool {
    fn name(&self) -> &str {
        "web_screenshot"
    }

    fn description(&self) -> &str {
        include_str!("instructions/web_screenshot.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebScreenshotArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebScreenshotArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.url.trim().is_empty() {
            return Err("url is required".to_string());
        }

        let mut body = serde_json::json!({
            "url": args.url,
            "request": args.request.as_deref().unwrap_or("chrome"),
            "storageless": true,
            "cache": false
        });

        if let Some(ref wfs) = args.wait_for_selector {
            body["wait_for_selector"] =
                serde_json::to_value(wfs).map_err(|e| format!("Invalid wait_for_selector: {e}"))?;
        }

        let response_text = self.spider.post("/spider/screenshot", &body).await?;
        let pages: Vec<CrawlPage> = serde_json::from_str(&response_text)
            .map_err(|e| format!("Failed to parse screenshot response: {e}"))?;

        let page = pages
            .first()
            .ok_or_else(|| "No screenshot returned.".to_string())?;

        if page.content.is_empty() {
            return Err("Screenshot returned empty content.".to_string());
        }

        let result = ScreenshotResult {
            url: page.url.clone(),
            content: format!("data:image/png;base64,{}", page.content),
            content_type: "image/png".to_string(),
        };

        serde_json::to_string(&result).map_err(|e| format!("Failed to serialize: {e}"))
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

// ═══════════════════════════════════════════════════════════════════════
// 5. WebTransformTool
// ═══════════════════════════════════════════════════════════════════════

#[derive(Debug, Deserialize, JsonSchema)]
pub struct WebTransformArgs {
    /// Array of HTML content items to transform.
    pub data: Vec<TransformItem>,
    /// Output format: "markdown" (default), "text", "html2text".
    #[serde(default)]
    pub return_format: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, JsonSchema)]
pub struct TransformItem {
    /// The HTML content to transform.
    pub html: String,
    /// Source URL for resolving relative links (optional).
    #[serde(default)]
    pub url: Option<String>,
}

pub struct WebTransformTool {
    spider: Arc<SpiderClient>,
}

impl WebTransformTool {
    pub fn new(spider: Arc<SpiderClient>) -> Self {
        Self { spider }
    }
}

#[async_trait]
impl ToolExecutor for WebTransformTool {
    fn name(&self) -> &str {
        "web_transform"
    }

    fn description(&self) -> &str {
        include_str!("instructions/web_transform.txt")
    }

    fn parameters_schema(&self) -> serde_json::Value {
        serde_json::to_value(schemars::schema_for!(WebTransformArgs))
            .unwrap_or_else(|_| serde_json::json!({}))
    }

    async fn execute(&self, args: serde_json::Value) -> Result<String, String> {
        let args: WebTransformArgs =
            serde_json::from_value(args).map_err(|e| format!("Invalid arguments: {e}"))?;

        if args.data.is_empty() {
            return Err("data array must not be empty".to_string());
        }

        let body = serde_json::json!({
            "data": args.data,
            "return_format": args.return_format.as_deref().unwrap_or("markdown"),
            "readability": true
        });

        let response_text = self.spider.post("/spider/transform", &body).await?;
        let response: TransformResponse = serde_json::from_str(&response_text)
            .map_err(|e| format!("Failed to parse transform response: {e}"))?;

        if response.content.is_empty() {
            return Ok("No content returned from transformation.".to_string());
        }

        if response.content.len() == 1 {
            return Ok(response.content.into_iter().next().unwrap());
        }

        let output = response.content.join("\n\n---\n\n");
        let truncated =
            truncation::truncate_output(&output, truncation::MAX_LINES, MAX_OUTPUT_CHARS);
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

// ─── Tests ─────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    async fn setup_spider(server: &MockServer) -> Arc<SpiderClient> {
        Arc::new(SpiderClient::new(server.uri()))
    }

    // ── web_crawl ──────────────────────────────────────────────────

    #[tokio::test]
    async fn test_web_crawl_single_page() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/crawl"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([
                {"url": "https://example.com/", "content": "# Example\n\nHello world"}
            ])))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebCrawlTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .expect("should succeed");

        assert!(result.contains("# Example"));
        assert!(result.contains("Hello world"));
        assert!(result.contains("https://example.com/"));
    }

    #[tokio::test]
    async fn test_web_crawl_multiple_pages() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/crawl"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([
                {"url": "https://example.com/", "content": "# Home"},
                {"url": "https://example.com/about", "content": "# About Us"},
                {"url": "https://example.com/contact", "content": "# Contact"}
            ])))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebCrawlTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"url": "https://example.com", "limit": 10}))
            .await
            .expect("should succeed");

        assert!(result.contains("# Home"));
        assert!(result.contains("# About Us"));
        assert!(result.contains("# Contact"));
    }

    #[tokio::test]
    async fn test_web_crawl_error_response() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/crawl"))
            .respond_with(
                ResponseTemplate::new(401)
                    .set_body_json(serde_json::json!({"error": "Invalid API key"})),
            )
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebCrawlTool::new(spider);
        let err = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .unwrap_err();

        assert!(err.contains("Invalid API key"));
    }

    #[tokio::test]
    async fn test_web_crawl_empty_url() {
        let server = MockServer::start().await;
        let spider = setup_spider(&server).await;
        let tool = WebCrawlTool::new(spider);
        let err = tool
            .execute(serde_json::json!({"url": ""}))
            .await
            .unwrap_err();
        assert!(err.contains("url is required"));
    }

    // ── web_search_v2 ──────────────────────────────────────────────

    #[tokio::test]
    async fn test_web_search_basic() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/search"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "content": [
                    {"url": "https://rust-lang.org/", "title": "Rust Programming Language", "description": "A systems programming language"},
                    {"url": "https://doc.rust-lang.org/", "title": "Rust Documentation", "description": "Official docs"}
                ]
            })))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebSearchTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"search": "rust programming"}))
            .await
            .expect("should succeed");

        assert!(result.contains("Rust Programming Language"));
        assert!(result.contains("rust-lang.org"));
        assert!(result.contains("Rust Documentation"));
    }

    #[tokio::test]
    async fn test_web_search_empty_results() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/search"))
            .respond_with(
                ResponseTemplate::new(200).set_body_json(serde_json::json!({"content": []})),
            )
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebSearchTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"search": "nonexistent query xyz123"}))
            .await
            .expect("should succeed");

        assert!(result.contains("No search results"));
    }

    // ── web_get_links ──────────────────────────────────────────────

    #[tokio::test]
    async fn test_web_get_links() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/links"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([
                {"url": "https://example.com/"},
                {"url": "https://example.com/about"},
                {"url": "https://example.com/blog"}
            ])))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebGetLinksTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .expect("should succeed");

        let urls: Vec<String> = serde_json::from_str(&result).expect("should be JSON array");
        assert_eq!(urls.len(), 3);
        assert!(urls.contains(&"https://example.com/about".to_string()));
    }

    // ── web_screenshot ─────────────────────────────────────────────

    #[tokio::test]
    async fn test_web_screenshot() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/screenshot"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([
                {"url": "https://example.com/", "content": "iVBORw0KGgoAAAANSUhEUg=="}
            ])))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebScreenshotTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .expect("should succeed");

        let parsed: serde_json::Value = serde_json::from_str(&result).expect("should be JSON");
        assert!(parsed["content"]
            .as_str()
            .unwrap()
            .starts_with("data:image/png;base64,"));
        assert_eq!(parsed["content_type"].as_str().unwrap(), "image/png");
    }

    #[tokio::test]
    async fn test_web_screenshot_with_selector() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/screenshot"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!([
                {"url": "https://example.com/", "content": "base64data=="}
            ])))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebScreenshotTool::new(spider);
        let result = tool
            .execute(serde_json::json!({
                "url": "https://example.com",
                "request": "chrome",
                "wait_for_selector": {"selector": "#app", "timeout": 5000}
            }))
            .await
            .expect("should succeed");

        assert!(result.contains("data:image/png;base64,"));
    }

    // ── web_transform ──────────────────────────────────────────────

    #[tokio::test]
    async fn test_web_transform() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/transform"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "content": ["# Hello World\nTest paragraph."]
            })))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebTransformTool::new(spider);
        let result = tool
            .execute(serde_json::json!({
                "data": [{"html": "<h1>Hello World</h1><p>Test paragraph.</p>"}]
            }))
            .await
            .expect("should succeed");

        assert!(result.contains("# Hello World"));
        assert!(result.contains("Test paragraph"));
    }

    #[tokio::test]
    async fn test_web_transform_multiple_items() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/transform"))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "content": ["# Page One", "# Page Two"]
            })))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebTransformTool::new(spider);
        let result = tool
            .execute(serde_json::json!({
                "data": [
                    {"html": "<h1>Page One</h1>"},
                    {"html": "<h1>Page Two</h1>"}
                ]
            }))
            .await
            .expect("should succeed");

        assert!(result.contains("# Page One"));
        assert!(result.contains("# Page Two"));
        assert!(result.contains("---"));
    }

    // ── error handling ─────────────────────────────────────────────

    #[tokio::test]
    async fn test_spider_client_server_error() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/crawl"))
            .respond_with(ResponseTemplate::new(502).set_body_string("Bad Gateway"))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebCrawlTool::new(spider);
        let err = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .unwrap_err();

        assert!(err.contains("Spider API error (502"));
    }

    #[tokio::test]
    async fn test_spider_client_json_error() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/spider/crawl"))
            .respond_with(ResponseTemplate::new(401).set_body_json(serde_json::json!({
                "error": "Credits or a valid subscription required"
            })))
            .expect(1)
            .mount(&server)
            .await;

        let spider = setup_spider(&server).await;
        let tool = WebCrawlTool::new(spider);
        let err = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .unwrap_err();

        assert!(err.contains("Credits or a valid subscription required"));
    }
}
