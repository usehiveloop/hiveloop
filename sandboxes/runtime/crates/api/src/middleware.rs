use axum::extract::Request;
use axum::middleware::Next;
use axum::response::Response;

/// Middleware that injects an X-Request-ID header if not present.
pub async fn request_id(mut request: Request, next: Next) -> Response {
    let request_id = request
        .headers()
        .get("x-request-id")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
        .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

    request.headers_mut().insert(
        "x-request-id",
        request_id.parse().expect("valid header value"),
    );

    let mut response = next.run(request).await;
    response.headers_mut().insert(
        "x-request-id",
        request_id.parse().expect("valid header value"),
    );
    response
}
