//! web_get_links Spider-backed tool.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;
use std::sync::Arc;

use super::SpiderClient;
use crate::registry::ToolExecutor;

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

#[derive(Debug, Deserialize)]
struct LinkPage {
    url: String,
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
        include_str!("../instructions/web_get_links.txt")
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

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

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

        let spider = Arc::new(SpiderClient::new(server.uri()));
        let tool = WebGetLinksTool::new(spider);
        let result = tool
            .execute(serde_json::json!({"url": "https://example.com"}))
            .await
            .expect("should succeed");

        let urls: Vec<String> = serde_json::from_str(&result).expect("should be JSON array");
        assert_eq!(urls.len(), 3);
        assert!(urls.contains(&"https://example.com/about".to_string()));
    }
}
