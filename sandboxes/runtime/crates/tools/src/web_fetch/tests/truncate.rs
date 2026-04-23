//! Truncation unit tests and end-to-end fetch smoke tests.

use super::super::parser::truncate_content;
use super::super::{FetchFormat, WebFetchTool};
use crate::ToolExecutor;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

#[test]
fn test_truncation_at_max_length() {
    // Multi-line input well above the cap should keep the first lines
    // and append the shared truncation marker pointing at the spill file.
    let lines: Vec<String> = (0..500).map(|i| format!("para {i:04}")).collect();
    let long_text = lines.join("\n");
    let truncated = truncate_content(&long_text, 500);
    assert!(
        truncated.contains("para 0000"),
        "head of the content should survive"
    );
    assert!(
        truncated.contains("truncated."),
        "should include shared truncation marker"
    );
    assert!(
        truncated.contains("RipGrep"),
        "should point the agent at the RipGrep tool"
    );
}

#[test]
fn test_truncation_not_applied_when_under_limit() {
    let text = "Hello, world!";
    let result = truncate_content(text, 1000);
    assert_eq!(result, text);
}

#[test]
fn test_truncation_exact_boundary() {
    let text = "abcde";
    let result = truncate_content(text, 5);
    assert_eq!(result, "abcde");
}

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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
            &format!("{}/long", server.uri()),
            100,
            &FetchFormat::Markdown,
        )
        .await
        .expect("fetch should succeed");

    // The content should be truncated by the shared truncator, which
    // appends a "truncated." marker pointing at the spill file.
    assert!(
        result.content.contains("truncated."),
        "content should include shared truncation marker"
    );
}
