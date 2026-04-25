//! HTTP middleware that injects Anthropic-style `cache_control` markers into
//! outbound `/chat/completions` and `/messages` request bodies.
//!
//! Most OpenAI-compatible providers we use (OpenRouter, Crof, DeepInfra) accept
//! the same `cache_control: { type: "ephemeral" }` structure on individual
//! message-content blocks that Anthropic's native API uses. Without these
//! markers the provider has to rely on its own opportunistic prefix-cache
//! detection — possible but not guaranteed, and capped at much lower hit rates
//! than explicit markers.
//!
//! Pattern mirrors `opencode`'s `applyCaching` (packages/opencode/src/provider/
//! transform.ts:228): mark up to four breakpoints — the first two system
//! messages, plus the last two non-system messages. Anthropic enforces a hard
//! ceiling of four breakpoints per request; OpenAI-compat providers either
//! enforce the same limit or silently ignore extras.
//!
//! ## Why a middleware (and not a rig hook)
//!
//! `rig-core` 0.35 only exposes `with_prompt_caching` on its native Anthropic
//! `CompletionModel`. For OpenAI-compatible providers there is no equivalent;
//! the underlying request body is built inside rig and dispatched through the
//! shared `reqwest_middleware::Client`. A middleware is the only seam where we
//! can mutate the body without forking rig.
//!
//! ## Observability
//!
//! Every `/chat/completions` or `/messages` POST that this middleware mutates
//! emits a structured info trace (`cache_control_applied`) with the number of
//! markers placed and the breakpoint locations. On the response side, callers
//! that parse the usage payload from the SSE stream get `cached_tokens` /
//! `cache_creation_input_tokens` / `cache_read_input_tokens` already — see
//! the dispatch macros in `dispatch.rs`. Combining the two lets us measure
//! cache hit ratio per turn.

use std::str::FromStr;

use bytes::Bytes;
use http::Extensions;
use http::Method;
use reqwest::{Body, Request};
use reqwest_middleware::{Middleware, Next, Result};
use serde_json::Value;
use tracing::info;

/// Drop-in middleware for `reqwest_middleware::ClientBuilder::with`. Forwards
/// all requests; mutates the JSON body of POSTs to chat-completion endpoints.
#[derive(Debug, Clone, Default)]
pub struct CacheControlMiddleware;

#[async_trait::async_trait]
impl Middleware for CacheControlMiddleware {
    async fn handle(
        &self,
        req: Request,
        extensions: &mut Extensions,
        next: Next<'_>,
    ) -> Result<reqwest::Response> {
        // Fast path: only POSTs to known chat-completion paths get touched.
        let is_target = req.method() == Method::POST
            && (req.url().path().ends_with("/chat/completions")
                || req.url().path().ends_with("/messages")
                || req.url().path().ends_with("/v1/messages"));
        if !is_target {
            return next.run(req, extensions).await;
        }

        let mutated = match try_apply(req).await {
            Ok(req) => req,
            Err((original, why)) => {
                tracing::debug!(error = %why, "cache_control_middleware skipped (parse failed)");
                original
            }
        };
        next.run(mutated, extensions).await
    }
}

/// Read the request body, parse as JSON, inject `cache_control` markers on
/// the canonical breakpoint slots, and rebuild the request with the new body.
///
/// Returns `Err((original, msg))` on any failure — the request is forwarded
/// unmodified rather than dropped, so caching is best-effort.
async fn try_apply(req: Request) -> std::result::Result<Request, (Request, String)> {
    let (parts, body_opt) = split_request(req);
    let Some(body_bytes) = body_opt else {
        return Err((rebuild(parts, None), "no body".into()));
    };

    let mut json: Value = match serde_json::from_slice(&body_bytes) {
        Ok(v) => v,
        Err(e) => return Err((rebuild(parts, Some(body_bytes)), format!("not json: {}", e))),
    };

    let breakpoints = apply_breakpoints(&mut json);
    if breakpoints == 0 {
        return Err((
            rebuild(parts, Some(body_bytes)),
            "no eligible messages".into(),
        ));
    }

    let new_body = match serde_json::to_vec(&json) {
        Ok(b) => Bytes::from(b),
        Err(e) => {
            return Err((
                rebuild(parts, Some(body_bytes)),
                format!("reserialize failed: {}", e),
            ))
        }
    };

    info!(
        breakpoints,
        body_bytes = new_body.len(),
        "cache_control_applied"
    );

    Ok(rebuild(parts, Some(new_body)))
}

/// Split a `reqwest::Request` into a tuple we can rebuild from. We pull the
/// body bytes if any and clone the rest.
struct RequestParts {
    method: Method,
    url: reqwest::Url,
    headers: http::HeaderMap,
}

fn split_request(req: Request) -> (RequestParts, Option<Bytes>) {
    let method = req.method().clone();
    let url = req.url().clone();
    let headers = req.headers().clone();
    let body_bytes = req
        .body()
        .and_then(|b| b.as_bytes().map(Bytes::copy_from_slice));
    (
        RequestParts {
            method,
            url,
            headers,
        },
        body_bytes,
    )
}

fn rebuild(parts: RequestParts, body: Option<Bytes>) -> Request {
    let url_str = parts.url.as_str().to_string();
    let url = reqwest::Url::from_str(&url_str).expect("url roundtrip");
    let mut new = Request::new(parts.method, url);
    *new.headers_mut() = parts.headers;
    if let Some(b) = body {
        *new.body_mut() = Some(Body::from(b));
    }
    new
}

/// Place the breakpoints. Returns the number of markers actually written.
///
/// Layout (mirrors forgecode/opencode's `applyCaching`):
///   - At most the first two `system` messages.
///   - At most the last two non-system messages.
///
/// On OpenAI-style payloads the `cache_control` field rides on the message
/// itself when its `content` is a plain string; when content is an array of
/// content blocks, the marker rides on the LAST block (so the cache covers
/// every prior block in that message). Anthropic's `/v1/messages` requires
/// the second pattern always.
fn apply_breakpoints(json: &mut Value) -> usize {
    let messages = match json.get_mut("messages").and_then(Value::as_array_mut) {
        Some(m) => m,
        None => return 0,
    };

    let mut targets: Vec<usize> = Vec::with_capacity(4);

    // System breakpoints — first two system messages.
    let mut sys_count = 0usize;
    for (idx, msg) in messages.iter().enumerate() {
        if msg.get("role").and_then(Value::as_str) == Some("system") {
            targets.push(idx);
            sys_count += 1;
            if sys_count >= 2 {
                break;
            }
        }
    }

    // Tail breakpoints — last two messages of role `user` or `assistant`.
    // We deliberately SKIP `role: "tool"` (OpenAI-style tool-result messages)
    // because (a) tool results are usually short-lived volatile content
    // unworthy of a precious cache slot and (b) some providers reject or
    // silently malform `cache_control` placed on a tool-role message,
    // which has caused the model to early-`stop` mid-task in testing.
    let mut tail: Vec<usize> = messages
        .iter()
        .enumerate()
        .filter(|(_, m)| {
            matches!(
                m.get("role").and_then(Value::as_str),
                Some("user") | Some("assistant")
            )
        })
        .map(|(i, _)| i)
        .collect();
    tail.reverse();
    for idx in tail.into_iter().take(2) {
        if !targets.contains(&idx) {
            targets.push(idx);
        }
    }

    let mut placed = 0usize;
    for idx in targets {
        if let Some(msg) = messages.get_mut(idx) {
            if mark_message(msg) {
                placed += 1;
            }
        }
    }
    placed
}

/// Attach `cache_control` to a single message. Handles both content shapes:
///   - String content → marker rides on the message object itself.
///   - Array content → marker rides on the LAST block (so the cache covers
///     every preceding block in the message).
///
/// Returns true if a marker was placed.
fn mark_message(msg: &mut Value) -> bool {
    let cc_value = serde_json::json!({ "type": "ephemeral" });
    match msg.get_mut("content") {
        Some(Value::Array(blocks)) if !blocks.is_empty() => {
            if let Some(Value::Object(last)) = blocks.last_mut() {
                last.insert("cache_control".to_string(), cc_value);
                return true;
            }
            false
        }
        _ => {
            // String content (or missing): mark the message itself.
            if let Value::Object(obj) = msg {
                obj.insert("cache_control".to_string(), cc_value);
                return true;
            }
            false
        }
    }
}

#[cfg(test)]
mod tests;
