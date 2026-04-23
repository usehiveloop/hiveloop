//! web_crawl Spider-backed tool.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;
use std::sync::Arc;

use super::{CrawlPage, SpiderClient};
use crate::registry::ToolExecutor;
use crate::truncation;

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
        include_str!("../instructions/web_crawl.txt")
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
            truncation::truncate_output(&output, truncation::MAX_LINES, truncation::MAX_BYTES);
        Ok(truncated.content)
    }

    fn as_any(&self) -> &dyn std::any::Any {
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    async fn setup_spider(server: &MockServer) -> Arc<SpiderClient> {
        Arc::new(SpiderClient::new(server.uri()))
    }

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
