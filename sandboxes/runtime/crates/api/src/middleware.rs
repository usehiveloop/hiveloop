use axum::extract::{Request, State};
use axum::middleware::Next;
use axum::response::Response;
use bridge_core::BridgeError;

use crate::state::AppState;

/// Middleware that validates Bearer token authentication for push endpoints.
pub async fn bearer_auth(
    State(state): State<AppState>,
    request: Request,
    next: Next,
) -> Result<Response, BridgeError> {
    let auth_header = request
        .headers()
        .get("authorization")
        .and_then(|v| v.to_str().ok());

    match auth_header {
        Some(header) if header.starts_with("Bearer ") => {
            let token = &header[7..];
            if token == state.control_plane_api_key {
                Ok(next.run(request).await)
            } else {
                Err(BridgeError::Unauthorized("invalid token".into()))
            }
        }
        _ => Err(BridgeError::Unauthorized(
            "missing or invalid authorization header".into(),
        )),
    }
}

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
