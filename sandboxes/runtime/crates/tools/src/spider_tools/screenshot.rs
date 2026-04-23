//! web_screenshot Spider-backed tool.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use super::{CrawlPage, SpiderClient};
use crate::registry::ToolExecutor;

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
        include_str!("../instructions/web_screenshot.txt")
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

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

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

        let spider = Arc::new(SpiderClient::new(server.uri()));
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

        let spider = Arc::new(SpiderClient::new(server.uri()));
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
}
