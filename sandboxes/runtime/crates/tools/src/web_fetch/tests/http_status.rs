//! HTTP status code tests and Cloudflare-challenge retry.

use super::super::{FetchFormat, WebFetchTool};
use wiremock::matchers::{method, path};
use wiremock::{Mock, MockServer, ResponseTemplate};

const DEFAULT_MAX_LENGTH: usize = crate::truncation::MAX_BYTES;

#[tokio::test]
async fn test_fetch_http_error_status() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/missing"))
        .respond_with(ResponseTemplate::new(404).set_body_string("Not Found"))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let err = tool
        .fetch_with_reqwest(
            &format!("{}/missing", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .unwrap_err();

    assert!(err.contains("HTTP error"));
    assert!(err.contains("404"));
}

#[tokio::test]
async fn test_http_500_returns_error() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/error"))
        .respond_with(ResponseTemplate::new(500).set_body_string("Internal Server Error"))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let err = tool
        .fetch_with_reqwest(
            &format!("{}/error", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .unwrap_err();

    assert!(
        err.contains("HTTP error"),
        "error should mention HTTP error: {err}"
    );
    assert!(
        err.contains("500"),
        "error should contain status code 500: {err}"
    );
}

#[tokio::test]
async fn test_http_403_returns_error() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/forbidden"))
        .respond_with(ResponseTemplate::new(403).set_body_string("Forbidden"))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let err = tool
        .fetch_with_reqwest(
            &format!("{}/forbidden", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .unwrap_err();

    assert!(
        err.contains("HTTP error"),
        "error should mention HTTP error: {err}"
    );
    assert!(
        err.contains("403"),
        "error should contain status code 403: {err}"
    );
}

#[tokio::test]
async fn test_http_503_returns_error() {
    let server = MockServer::start().await;

    Mock::given(method("GET"))
        .and(path("/unavailable"))
        .respond_with(ResponseTemplate::new(503).set_body_string("Service Unavailable"))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let err = tool
        .fetch_with_reqwest(
            &format!("{}/unavailable", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .unwrap_err();

    assert!(
        err.contains("HTTP error"),
        "error should mention HTTP error: {err}"
    );
    assert!(
        err.contains("503"),
        "error should contain status code 503: {err}"
    );
}

#[tokio::test]
async fn test_fetch_cloudflare_retry() {
    let server = MockServer::start().await;

    // First request returns 403 with cf-mitigated: challenge
    // wiremock will match both requests to /cf-page
    // We need two mocks: first returns 403, second returns 200
    // But wiremock matches all requests to a path. We use expect to control.
    Mock::given(method("GET"))
        .and(path("/cf-page"))
        .respond_with(
            ResponseTemplate::new(403)
                .insert_header("cf-mitigated", "challenge")
                .set_body_string("Blocked"),
        )
        .expect(1)
        .up_to_n_times(1)
        .mount(&server)
        .await;

    Mock::given(method("GET"))
        .and(path("/cf-page"))
        .respond_with(ResponseTemplate::new(200).set_body_raw(
            "<html><body><p>Success after retry</p></body></html>",
            "text/html",
        ))
        .expect(1)
        .mount(&server)
        .await;

    let tool = WebFetchTool::with_defaults();
    let result = tool
        .fetch_with_reqwest(
            &format!("{}/cf-page", server.uri()),
            DEFAULT_MAX_LENGTH,
            &FetchFormat::Markdown,
        )
        .await
        .expect("should succeed after CF retry");

    assert!(!result.content.is_empty());
}
