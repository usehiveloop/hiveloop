//! web_search Spider-backed tool.

use async_trait::async_trait;
use schemars::JsonSchema;
use serde::Deserialize;
use std::sync::Arc;

use super::SpiderClient;
use crate::registry::ToolExecutor;
use crate::truncation;

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

pub struct WebSearchTool {
    spider: Arc<SpiderClient>,
    description: String,
}

impl WebSearchTool {
    pub fn new(spider: Arc<SpiderClient>) -> Self {
        let year = chrono::Utc::now().format("%Y").to_string();
        let description =
            include_str!("../instructions/web_search_spider.txt").replace("{{year}}", &year);
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
}
