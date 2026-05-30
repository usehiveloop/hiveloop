use axum::{
    extract::State,
    http::{header::AUTHORIZATION, Request, StatusCode},
    middleware::Next,
    response::Response,
};

use crate::state::ApiState;

pub async fn bearer_auth(
    State(state): State<ApiState>,
    request: Request<axum::body::Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    let path = request.uri().path();
    if path == "/healthz" || path.starts_with("/tunnel/") {
        return Ok(next.run(request).await);
    }
    let authorization_header = request
        .headers()
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .unwrap_or("");
    let authorized = {
        let token = state.bearer_token.read().await;
        let expected = format!("Bearer {}", token.as_str());
        constant_time_eq(authorization_header.as_bytes(), expected.as_bytes())
    };
    if !authorized {
        return Err(StatusCode::UNAUTHORIZED);
    }
    Ok(next.run(request).await)
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    let mut diff: u8 = 0;
    for (left, right) in a.iter().zip(b.iter()) {
        diff |= left ^ right;
    }
    diff == 0
}
