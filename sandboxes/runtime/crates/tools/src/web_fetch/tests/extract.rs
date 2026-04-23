//! Article extraction, fallback conversion, description, script/style
//! stripping, and title extraction tests.

use super::super::parser::{extract_article, fallback_convert};
use super::super::{FetchFormat, WebFetchTool};
use crate::ToolExecutor;
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

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
    let html = include_str!("../../../../../fixtures/html/article.html");
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
    let html = include_str!("../../../../../fixtures/html/nonarticle.html");
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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
