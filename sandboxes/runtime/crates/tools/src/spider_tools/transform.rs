//! web_transform Spider-backed tool.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use super::SpiderClient;
use crate::registry::ToolExecutor;
use crate::truncation;

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

#[derive(Debug, Deserialize)]
struct TransformResponse {
    #[serde(default)]
    content: Vec<String>,
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
        include_str!("../instructions/web_transform.txt")
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

        let spider = Arc::new(SpiderClient::new(server.uri()));
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

        let spider = Arc::new(SpiderClient::new(server.uri()));
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
}
