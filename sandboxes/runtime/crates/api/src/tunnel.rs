use std::collections::HashMap;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::extract::FromRequest;
use axum::{
    body::Body,
    extract::{Query, State},
    http::{header, HeaderMap, HeaderValue, Request, StatusCode},
    response::{IntoResponse, Redirect, Response},
    Json,
};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use futures::{SinkExt, StreamExt};
use hmac::{Hmac, Mac};
use sha2::Sha256;
use tracing::{debug, warn};

use crate::state::ApiState;

const TUNNEL_COOKIE_NAME: &str = "hivy_tunnel_token";
const COOKIE_MAX_AGE: u64 = 86400;
const BLOCKED_PORTS: &[u16] = &[7080];
const MIN_PORT: u16 = 1024;

type HmacSha256 = Hmac<Sha256>;

const AUTH_PAGE: &str = include_str!("../assets/tunnel-auth.html");

#[derive(serde::Deserialize)]
pub struct TunnelAuthRequest {
    pub password: String,
}

#[derive(serde::Serialize)]
pub struct TunnelAuthResponse {
    pub ok: bool,
}

#[derive(serde::Deserialize)]
struct TunnelTokenClaims {
    sub: String,
    port: u16,
    exp: u64,
}

fn now_unix() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

fn sign_data(data: &[u8], secret: &str) -> String {
    let mut mac =
        HmacSha256::new_from_slice(secret.as_bytes()).expect("HMAC accepts any key length");
    mac.update(data);
    hex::encode(mac.finalize().into_bytes())
}

fn verify_data(data: &[u8], signature_hex: &str, secret: &str) -> bool {
    let Ok(mut mac) = HmacSha256::new_from_slice(secret.as_bytes()) else {
        return false;
    };
    mac.update(data);
    let Ok(expected) = hex::decode(signature_hex) else {
        return false;
    };
    mac.verify_slice(&expected).is_ok()
}

fn sign_cookie(expiry: u64, secret: &str) -> String {
    let expiry_str = expiry.to_string();
    let sig = sign_data(expiry_str.as_bytes(), secret);
    format!("{expiry_str}.{sig}")
}

fn verify_cookie(token: &str, secret: &str) -> bool {
    let Some((expiry_str, sig)) = token.split_once('.') else {
        return false;
    };
    let Ok(expiry) = expiry_str.parse::<u64>() else {
        return false;
    };
    if expiry < now_unix() {
        return false;
    }
    verify_data(expiry_str.as_bytes(), sig, secret)
}

fn verify_jwt(token: &str, secret: &str, expected_port: u16) -> bool {
    let parts: Vec<&str> = token.splitn(3, '.').collect();
    if parts.len() != 3 {
        return false;
    }
    let signing_input = format!("{}.{}", parts[0], parts[1]);
    if !verify_data(signing_input.as_bytes(), parts[2], secret) {
        return false;
    }
    let Ok(claims_bytes) = URL_SAFE_NO_PAD.decode(parts[1]) else {
        return false;
    };
    let Ok(claims) = serde_json::from_slice::<TunnelTokenClaims>(&claims_bytes) else {
        return false;
    };
    if claims.sub != "tunnel" {
        return false;
    }
    if claims.exp < now_unix() {
        return false;
    }
    claims.port == expected_port
}

fn parse_cookies(cookie_header: &str) -> HashMap<&str, &str> {
    cookie_header
        .split(';')
        .filter_map(|pair| {
            let pair = pair.trim();
            let (key, value) = pair.split_once('=')?;
            Some((key.trim(), value.trim()))
        })
        .collect()
}

fn validate_port(port: u16) -> Result<(), (StatusCode, &'static str)> {
    if port < MIN_PORT {
        return Err((StatusCode::FORBIDDEN, "port must be >= 1024"));
    }
    if BLOCKED_PORTS.contains(&port) {
        return Err((StatusCode::FORBIDDEN, "port is blocked"));
    }
    Ok(())
}

fn is_browser_request(headers: &HeaderMap) -> bool {
    headers
        .get(header::ACCEPT)
        .and_then(|v| v.to_str().ok())
        .map(|v| v.contains("text/html"))
        .unwrap_or(false)
}

fn authenticate(
    headers: &HeaderMap,
    query_params: &HashMap<String, String>,
    port: u16,
    request_path: &str,
    state: &ApiState,
) -> Result<(), Box<Response>> {
    if let Some(cookies) = headers.get(header::COOKIE).and_then(|v| v.to_str().ok()) {
        let parsed = parse_cookies(cookies);
        if let Some(token) = parsed.get(TUNNEL_COOKIE_NAME) {
            let bearer = state.bearer_token.try_read();
            if let Ok(secret) = bearer {
                if verify_cookie(token, &secret) {
                    return Ok(());
                }
            }
        }
    }

    if let Some(jwt) = query_params.get("token") {
        let bearer = state.bearer_token.try_read();
        if let Ok(secret) = bearer {
            if verify_jwt(jwt, &secret, port) {
                return Ok(());
            }
        }
    }

    if state.tunnel_password.is_none() {
        return Ok(());
    }

    if is_browser_request(headers) {
        let return_to = urlencoding_encode(request_path);
        return Err(Box::new(
            Redirect::temporary(&format!("/tunnel/auth?return_to={return_to}")).into_response(),
        ));
    }

    Err(Box::new(
        (StatusCode::UNAUTHORIZED, "tunnel authentication required").into_response(),
    ))
}

pub async fn get_tunnel_auth(
    State(state): State<ApiState>,
    Query(params): Query<HashMap<String, String>>,
) -> Response {
    let return_to = params.get("return_to").cloned().unwrap_or_default();

    if state.tunnel_password.is_none() && !return_to.is_empty() {
        let cookie = sign_cookie(
            now_unix() + COOKIE_MAX_AGE,
            &state.bearer_token.read().await,
        );
        let mut response = Redirect::temporary(&return_to).into_response();
        if let Ok(val) = HeaderValue::from_str(&format!(
            "{}={}; Path=/tunnel; Max-Age={}; HttpOnly; Secure; SameSite=Lax",
            TUNNEL_COOKIE_NAME, cookie, COOKIE_MAX_AGE,
        )) {
            response.headers_mut().insert(header::SET_COOKIE, val);
        }
        return response;
    }

    let error = params.get("error").cloned().unwrap_or_default();
    let html = AUTH_PAGE
        .replace("{RETURN_TO}", &html_escape(&return_to))
        .replace("{ERROR_CLASS}", error_class(&error));

    let mut response = Response::new(Body::from(html));
    response.headers_mut().insert(
        header::CONTENT_TYPE,
        HeaderValue::from_static("text/html; charset=utf-8"),
    );
    response
}

fn html_escape(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#x27;")
}

fn error_class(error: &str) -> &str {
    if error == "1" {
        " show"
    } else {
        ""
    }
}

pub async fn post_tunnel_auth(
    State(state): State<ApiState>,
    headers: HeaderMap,
    body: String,
) -> Response {
    let content_type = headers
        .get(header::CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");

    let (password, return_to, is_form) =
        if content_type.contains("application/x-www-form-urlencoded") {
            let params = parse_form_urlencoded(&body);
            (
                params.get("password").cloned().unwrap_or_default(),
                params.get("return_to").cloned().unwrap_or_default(),
                true,
            )
        } else {
            let req: TunnelAuthRequest = match serde_json::from_str(&body) {
                Ok(r) => r,
                Err(_) => return (StatusCode::BAD_REQUEST, "invalid request body").into_response(),
            };
            (req.password, String::new(), false)
        };

    if let Some(ref expected) = state.tunnel_password {
        if password.len() != expected.len()
            || !constant_time_eq(password.as_bytes(), expected.as_bytes())
        {
            if is_form {
                let return_param = if return_to.is_empty() {
                    String::new()
                } else {
                    format!("&return_to={}", urlencoding_encode(&return_to))
                };
                return Redirect::temporary(&format!("/tunnel/auth?error=1{return_param}"))
                    .into_response();
            }
            return (StatusCode::UNAUTHORIZED, "invalid password").into_response();
        }
    }

    let cookie = sign_cookie(
        now_unix() + COOKIE_MAX_AGE,
        &state.bearer_token.read().await,
    );
    let cookie_value = format!(
        "{}={}; Path=/tunnel; Max-Age={}; HttpOnly; Secure; SameSite=Lax",
        TUNNEL_COOKIE_NAME, cookie, COOKIE_MAX_AGE,
    );

    if is_form && !return_to.is_empty() {
        let mut response = Redirect::temporary(&return_to).into_response();
        if let Ok(val) = HeaderValue::from_str(&cookie_value) {
            response.headers_mut().insert(header::SET_COOKIE, val);
        }
        return response;
    }

    let mut response = Json(TunnelAuthResponse { ok: true }).into_response();
    if let Ok(val) = HeaderValue::from_str(&cookie_value) {
        response.headers_mut().insert(header::SET_COOKIE, val);
    }
    response
}

fn parse_form_urlencoded(body: &str) -> HashMap<String, String> {
    body.split('&')
        .filter_map(|pair| {
            let (key, value) = pair.split_once('=')?;
            Some((urlencoding_decode(key), urlencoding_decode(value)))
        })
        .collect()
}

fn urlencoding_decode(s: &str) -> String {
    let s = s.replace('+', " ");
    let mut decoded = String::with_capacity(s.len());
    let mut chars = s.chars();
    while let Some(c) = chars.next() {
        if c == '%' {
            let hex: String = chars.by_ref().take(2).collect();
            if let Ok(byte) = u8::from_str_radix(&hex, 16) {
                decoded.push(byte as char);
            } else {
                decoded.push('%');
                decoded.push_str(&hex);
            }
        } else {
            decoded.push(c);
        }
    }
    decoded
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

pub async fn handle_tunnel(
    State(state): State<ApiState>,
    Query(params): Query<HashMap<String, String>>,
    headers: HeaderMap,
    request: Request<Body>,
) -> Response {
    let path = request.uri().path().to_string();
    let remainder = path.strip_prefix("/tunnel/").unwrap_or("");
    let (port_str, sub_path) = match remainder.split_once('/') {
        Some((p, rest)) => (p, rest.to_string()),
        None => (remainder, String::new()),
    };

    let port: u16 = match port_str.parse() {
        Ok(p) => p,
        Err(_) => return (StatusCode::BAD_REQUEST, "invalid port").into_response(),
    };

    if let Err((status, msg)) = validate_port(port) {
        return (status, msg).into_response();
    }

    if let Err(resp) = authenticate(&headers, &params, port, &path, &state) {
        return *resp;
    }

    let is_ws = headers
        .get(header::UPGRADE)
        .and_then(|v| v.to_str().ok())
        .map(|v| v.eq_ignore_ascii_case("websocket"))
        .unwrap_or(false);

    if is_ws {
        proxy_ws(port, sub_path, headers, request, state).await
    } else {
        proxy_http(port, sub_path, params, headers, request).await
    }
}

async fn proxy_http(
    port: u16,
    path: String,
    params: HashMap<String, String>,
    headers: HeaderMap,
    request: Request<Body>,
) -> Response {
    let query_string = if params.is_empty() {
        String::new()
    } else {
        let pairs: Vec<String> = params
            .iter()
            .map(|(k, v)| format!("{}={}", urlencoding_encode(k), urlencoding_encode(v)))
            .collect();
        format!("?{}", pairs.join("&"))
    };
    let upstream_url = format!("http://localhost:{port}/{path}{query_string}");

    let method = request.method().clone();
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(300))
        .build()
        .unwrap_or_default();

    let mut upstream_req = client.request(method, &upstream_url);

    let original_host = headers
        .get(header::HOST)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("localhost");

    for (key, value) in headers.iter() {
        let key_str = key.as_str().to_lowercase();
        if matches!(
            key_str.as_str(),
            "host"
                | "connection"
                | "cookie"
                | "authorization"
                | "proxy-authorization"
                | "keep-alive"
                | "transfer-encoding"
                | "te"
                | "trailer"
                | "upgrade"
        ) {
            continue;
        }
        if let Ok(val_str) = value.to_str() {
            upstream_req = upstream_req.header(key.as_str(), val_str);
        }
    }

    upstream_req = upstream_req
        .header("Host", format!("localhost:{port}"))
        .header("X-Forwarded-For", "127.0.0.1")
        .header("X-Forwarded-Proto", "http")
        .header("X-Forwarded-Host", original_host);

    let body_bytes = match axum::body::to_bytes(request.into_body(), 10 * 1024 * 1024).await {
        Ok(b) => b,
        Err(_) => return (StatusCode::BAD_REQUEST, "failed to read request body").into_response(),
    };
    if !body_bytes.is_empty() {
        upstream_req = upstream_req.body(body_bytes);
    }

    let upstream_resp = match upstream_req.send().await {
        Ok(r) => r,
        Err(e) => {
            warn!(error = %e, "tunnel upstream request failed");
            return (StatusCode::BAD_GATEWAY, "upstream unreachable").into_response();
        }
    };

    let status =
        StatusCode::from_u16(upstream_resp.status().as_u16()).unwrap_or(StatusCode::BAD_GATEWAY);
    let resp_headers = upstream_resp.headers().clone();

    let mut response = Response::new(Body::empty());
    *response.status_mut() = status;

    for (key, value) in resp_headers.iter() {
        let key_str = key.as_str().to_lowercase();
        if matches!(
            key_str.as_str(),
            "connection"
                | "keep-alive"
                | "transfer-encoding"
                | "te"
                | "trailer"
                | "proxy-authenticate"
                | "proxy-authorization"
                | "upgrade"
        ) {
            continue;
        }
        if let Ok(val) = HeaderValue::from_bytes(value.as_bytes()) {
            response.headers_mut().insert(key, val);
        }
    }

    let stream = upstream_resp.bytes_stream();
    let body = Body::from_stream(stream.map(|r| r.map_err(std::io::Error::other)));
    *response.body_mut() = body;

    response
}

async fn proxy_ws(
    port: u16,
    path: String,
    headers: HeaderMap,
    request: Request<Body>,
    state: ApiState,
) -> Response {
    let original_origin = headers
        .get(header::ORIGIN)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_string();
    let ws_protocol = headers
        .get("sec-websocket-protocol")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_string();

    let upgrade_future = axum::extract::ws::WebSocketUpgrade::from_request(request, &state);
    let ws_upgrade: axum::extract::ws::WebSocketUpgrade = match upgrade_future.await {
        Ok(u) => u,
        Err(e) => {
            warn!(error = %e, "websocket upgrade failed");
            return (StatusCode::BAD_REQUEST, "websocket upgrade failed").into_response();
        }
    };

    ws_upgrade.on_upgrade(move |client_ws| async move {
        let upstream_path = if path.is_empty() {
            "/".to_string()
        } else {
            format!("/{path}")
        };
        let upstream_url = format!("ws://localhost:{port}{upstream_path}");

        let mut request_builder =
            tokio_tungstenite::tungstenite::client::IntoClientRequest::into_client_request(
                &upstream_url,
            )
            .unwrap_or_else(|_| {
                let mut r = http::Request::new(());
                *r.uri_mut() = upstream_url.parse().unwrap();
                r
            });
        {
            let req_headers = request_builder.headers_mut();
            req_headers.insert("Host", format!("localhost:{port}").parse().unwrap());
            if !original_origin.is_empty() {
                req_headers.insert("Origin", original_origin.parse().unwrap());
            }
            if !ws_protocol.is_empty() {
                req_headers.insert("Sec-WebSocket-Protocol", ws_protocol.parse().unwrap());
            }
        }

        let upstream_ws = match tokio_tungstenite::connect_async(request_builder).await {
            Ok((ws, _)) => ws,
            Err(e) => {
                warn!(error = %e, "tunnel ws upstream connect failed");
                return;
            }
        };

        debug!("tunnel ws: upstream handshake complete, starting bidirectional copy");

        let (mut client_sink, mut client_stream) = client_ws.split();
        let (mut upstream_sink, mut upstream_stream) = upstream_ws.split();

        let c2u = tokio::spawn(async move {
            while let Some(msg) = client_stream.next().await {
                let msg = match msg {
                    Ok(m) => m,
                    Err(_) => break,
                };
                let ws_msg = match msg {
                    axum::extract::ws::Message::Text(t) => {
                        tokio_tungstenite::tungstenite::Message::Text(t)
                    }
                    axum::extract::ws::Message::Binary(b) => {
                        tokio_tungstenite::tungstenite::Message::Binary(b)
                    }
                    axum::extract::ws::Message::Ping(p) => {
                        tokio_tungstenite::tungstenite::Message::Ping(p)
                    }
                    axum::extract::ws::Message::Pong(p) => {
                        tokio_tungstenite::tungstenite::Message::Pong(p)
                    }
                    axum::extract::ws::Message::Close(_) => break,
                };
                if upstream_sink.send(ws_msg).await.is_err() {
                    break;
                }
            }
        });

        let u2c = tokio::spawn(async move {
            while let Some(msg) = upstream_stream.next().await {
                let msg = match msg {
                    Ok(m) => m,
                    Err(_) => break,
                };
                let axum_msg = match msg {
                    tokio_tungstenite::tungstenite::Message::Text(t) => {
                        axum::extract::ws::Message::Text(t)
                    }
                    tokio_tungstenite::tungstenite::Message::Binary(b) => {
                        axum::extract::ws::Message::Binary(b)
                    }
                    tokio_tungstenite::tungstenite::Message::Ping(p) => {
                        axum::extract::ws::Message::Ping(p)
                    }
                    tokio_tungstenite::tungstenite::Message::Pong(p) => {
                        axum::extract::ws::Message::Pong(p)
                    }
                    tokio_tungstenite::tungstenite::Message::Close(_) => break,
                    _ => continue,
                };
                if client_sink.send(axum_msg).await.is_err() {
                    break;
                }
            }
        });

        let _ = tokio::join!(c2u, u2c);
        debug!("tunnel ws: connection closed");
    })
}

fn urlencoding_encode(s: &str) -> String {
    let mut encoded = String::with_capacity(s.len() * 3);
    for byte in s.bytes() {
        match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                encoded.push(byte as char);
            }
            _ => {
                encoded.push('%');
                encoded.push_str(&format!("{byte:02X}"));
            }
        }
    }
    encoded
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sign_jwt(port: u16, secret: &str) -> String {
        let header = URL_SAFE_NO_PAD.encode(r#"{"alg":"HS256","typ":"JWT"}"#);
        let now = now_unix();
        let claims = format!(
            r#"{{"sub":"tunnel","port":{},"exp":{},"iat":{}}}"#,
            port,
            now + 3600,
            now
        );
        let payload = URL_SAFE_NO_PAD.encode(claims.as_bytes());
        let signing_input = format!("{header}.{payload}");
        let sig = sign_data(signing_input.as_bytes(), secret);
        format!("{signing_input}.{sig}")
    }

    #[test]
    fn cookie_sign_and_verify_roundtrip() {
        let secret = "test-secret-key-for-hmac-signing";
        let expiry = now_unix() + 3600;
        let token = sign_cookie(expiry, secret);
        assert!(verify_cookie(&token, secret));
        assert!(!verify_cookie(&token, "wrong-secret"));
    }

    #[test]
    fn cookie_expired_rejected() {
        let secret = "test-secret-key-for-hmac-signing";
        let expiry = now_unix().saturating_sub(10);
        let token = sign_cookie(expiry, secret);
        assert!(!verify_cookie(&token, secret));
    }

    #[test]
    fn jwt_sign_and_verify_roundtrip() {
        let secret = "test-secret-key-for-hmac-signing";
        let port = 5173u16;
        let jwt = sign_jwt(port, secret);
        assert!(verify_jwt(&jwt, secret, port));
        assert!(!verify_jwt(&jwt, secret, 3000));
        assert!(!verify_jwt(&jwt, "wrong-secret", port));
    }

    #[test]
    fn cookie_tamper_detected() {
        let secret = "test-secret-key-for-hmac-signing";
        let expiry = now_unix() + 3600;
        let mut token = sign_cookie(expiry, secret);
        token.push('x');
        assert!(!verify_cookie(&token, secret));
    }

    #[test]
    fn parse_cookies_extracts_pairs() {
        let cookies = "foo=bar; hivy_tunnel_token=abc123; other=val";
        let parsed = parse_cookies(cookies);
        assert_eq!(parsed.get("foo"), Some(&"bar"));
        assert_eq!(parsed.get("hivy_tunnel_token"), Some(&"abc123"));
        assert_eq!(parsed.get("other"), Some(&"val"));
    }

    #[test]
    fn validate_port_blocks_restricted() {
        assert!(validate_port(7080).is_err());
        assert!(validate_port(80).is_err());
        assert!(validate_port(1023).is_err());
        assert!(validate_port(5173).is_ok());
        assert!(validate_port(3000).is_ok());
    }
}
