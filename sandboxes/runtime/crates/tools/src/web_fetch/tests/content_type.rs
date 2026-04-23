//! Content-type edge-case tests (missing, xhtml, pdf, image, svg, size limits).

use super::super::{FetchFormat, WebFetchTool};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

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
        .fetch_with_reqwest(
            &format!("{}/data.json", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .unwrap_err();

    assert!(err.contains("Non-HTML content type"));
}

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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
            ResponseTemplate::new(200).set_body_raw(b"fake pdf bytes" as &[u8], "application/pdf"),
        )
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let err = tool
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
        .fetch_with_reqwest(
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
            ResponseTemplate::new(200).set_body_raw(large_body.as_bytes().to_vec(), "text/html"),
        )
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let err = tool
        .fetch_with_reqwest(
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
