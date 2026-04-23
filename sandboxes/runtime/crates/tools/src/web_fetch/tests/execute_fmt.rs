//! End-to-end `execute` tests and format-specific (text/html) fetch tests.

use super::super::{FetchFormat, FetchResult, WebFetchTool};
use crate::ToolExecutor;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

#[tokio::test]
async fn test_execute_with_custom_max_length() {
    let server = MockServer::start().await;

    let long_paragraph = "word ".repeat(200);
    let html = format!(r#"<html><body><p>{long_paragraph}</p></body></html>"#);

    // Allow multiple requests
    Mock::given(method("GET"))
        .and(path("/truncate-exec"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
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
        parsed.content.contains("truncated."),
        "content should be truncated with max_length=50"
    );
}

#[tokio::test]
async fn test_execute_with_default_max_length() {
    let server = MockServer::start().await;

    let html = r#"<html><body><p>Simple content for default max length test.</p></body></html>"#;

    // Allow multiple requests
    Mock::given(method("GET"))
        .and(path("/default-exec"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
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
        !parsed.content.contains("truncated."),
        "short content should not be truncated under the default cap"
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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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

    // Allow multiple requests — spider tier may also hit this endpoint
    Mock::given(method("GET"))
        .and(path("/fmt"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let args = serde_json::json!({
        "url": format!("{}/fmt", server.uri()),
        "format": "html"
    });

    let output = tool.execute(args).await.expect("execute should succeed");
    let parsed: FetchResult = serde_json::from_str(&output).expect("parse");

    // Content should contain the HTML from reqwest
    assert!(
        parsed.content.contains("Format test content"),
        "should contain page content"
    );
}
