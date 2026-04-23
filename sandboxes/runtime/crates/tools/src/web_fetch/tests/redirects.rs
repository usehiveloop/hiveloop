//! Redirect handling tests.

use super::super::{FetchFormat, WebFetchTool};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

#[tokio::test]
async fn test_follow_single_redirect() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/old"))
        .respond_with(
            ResponseTemplate::new(301).insert_header("Location", &*format!("{}/new", server.uri())),
        )
        .expect(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/new"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            "<html><body><p>Final destination</p></body></html>",
            "text/html",
        ))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let result = tool
        .fetch_with_reqwest(
            &format!("{}/old", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .expect("fetch should succeed after redirect");

    assert_eq!(result.url, format!("{}/new", server.uri()));
    assert!(!result.content.is_empty());
}

#[tokio::test]
async fn test_follow_redirect_chain() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/hop1"))
        .respond_with(
            ResponseTemplate::new(302)
                .insert_header("Location", &*format!("{}/hop2", server.uri())),
        )
        .expect(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/hop2"))
        .respond_with(
            ResponseTemplate::new(302)
                .insert_header("Location", &*format!("{}/hop3", server.uri())),
        )
        .expect(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/hop3"))
        .respond_with(
            ResponseTemplate::new(302)
                .insert_header("Location", &*format!("{}/final", server.uri())),
        )
        .expect(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/final"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_body_raw("<html><body><p>End of chain</p></body></html>", "text/html"),
        )
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let result = tool
        .fetch_with_reqwest(
            &format!("{}/hop1", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .expect("fetch should follow redirect chain");

    assert_eq!(result.url, format!("{}/final", server.uri()));
}

#[tokio::test]
async fn test_redirect_preserves_content() {
    let server = MockServer::start().await;

    let html = r#"<html>
    <head><title>Redirected Article</title></head>
    <body>
        <article>
            <h1>Redirected Article</h1>
            <p>This content was reached via redirect.</p>
            <p>It should be fully extracted despite the redirect hop.</p>
            <p>The article discusses important topics about software engineering.</p>
            <p>Including best practices for building robust systems.</p>
            <p>And patterns for handling distributed architectures.</p>
            <p>Performance optimization is also covered in detail.</p>
        </article>
    </body>
    </html>"#;

    Mock::given(method("GET"))
        .and(path("/redirect-me"))
        .respond_with(
            ResponseTemplate::new(301)
                .insert_header("Location", &*format!("{}/article", server.uri())),
        )
        .expect(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/article"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(html, "text/html; charset=utf-8"))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let result = tool
        .fetch_with_reqwest(
            &format!("{}/redirect-me", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .expect("fetch should succeed after redirect");

    assert!(
        result.content.contains("redirect") || result.content.contains("Redirect"),
        "content should be extracted from redirected page"
    );
}
