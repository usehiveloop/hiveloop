//! Inject `tool_choice` into outbound chat-completion requests.
//!
//! Some models (notably reasoning-mode glm-5.1 on Crof) emit a "plan" in
//! `reasoning_content` + a "I'll start by…" content message but never
//! actually call a tool. The default `tool_choice` (omitted, model decides)
//! lets them stop. Setting `tool_choice: "required"` forces the model to
//! emit at least one tool call per turn.
//!
//! Controlled by env var `BRIDGE_TOOL_CHOICE`:
//!   * unset (default) → no injection; rig's request goes through unchanged.
//!   * `"required"` → inject `tool_choice: "required"` whenever `tools`
//!     is non-empty.
//!   * `"auto"` → inject `tool_choice: "auto"` (explicit hint; most providers
//!     default to this).
//!
//! Like the cache_control middleware, this only mutates POST bodies to
//! `/chat/completions`. Anthropic's `/v1/messages` endpoint uses a
//! different shape and is left alone.

use bytes::Bytes;
use http::{Extensions, Method};
use reqwest::{Body, Request};
use reqwest_middleware::{Middleware, Next, Result};
use serde_json::Value;
use tracing::{info, trace};

#[derive(Debug, Clone)]
pub struct ToolChoiceMiddleware {
    /// Pre-resolved choice ("required", "auto", "none", or a function-call
    /// object). When `None`, the middleware is effectively a no-op.
    choice: Option<Value>,
}

impl ToolChoiceMiddleware {
    /// Read `BRIDGE_TOOL_CHOICE` from the environment. Returns a
    /// configured middleware if the env is set to a recognised value,
    /// else a pass-through middleware.
    pub fn from_env() -> Self {
        match std::env::var("BRIDGE_TOOL_CHOICE")
            .ok()
            .as_deref()
            .map(str::trim)
        {
            Some("required") | Some("REQUIRED") => {
                info!(
                    "tool_choice middleware ENABLED — injecting `required` on requests with tools"
                );
                Self {
                    choice: Some(Value::String("required".to_string())),
                }
            }
            Some("auto") | Some("AUTO") => {
                info!("tool_choice middleware ENABLED — injecting `auto` on requests with tools");
                Self {
                    choice: Some(Value::String("auto".to_string())),
                }
            }
            Some("none") | Some("NONE") => {
                info!("tool_choice middleware ENABLED — injecting `none` (suppresses tool calls)");
                Self {
                    choice: Some(Value::String("none".to_string())),
                }
            }
            Some(other) if !other.is_empty() => {
                tracing::warn!(
                    value = %other,
                    "BRIDGE_TOOL_CHOICE has unrecognized value; ignoring"
                );
                Self { choice: None }
            }
            _ => Self { choice: None },
        }
    }
}

#[async_trait::async_trait]
impl Middleware for ToolChoiceMiddleware {
    async fn handle(
        &self,
        req: Request,
        extensions: &mut Extensions,
        next: Next<'_>,
    ) -> Result<reqwest::Response> {
        let Some(choice) = self.choice.clone() else {
            return next.run(req, extensions).await;
        };

        let target =
            req.method() == Method::POST && req.url().path().ends_with("/chat/completions");
        if !target {
            return next.run(req, extensions).await;
        }

        let mutated = match try_apply(req, &choice).await {
            Ok(req) => req,
            Err((original, why)) => {
                trace!(error = %why, "tool_choice_middleware skipped");
                original
            }
        };
        next.run(mutated, extensions).await
    }
}

async fn try_apply(
    req: Request,
    choice: &Value,
) -> std::result::Result<Request, (Request, String)> {
    let method = req.method().clone();
    let url = req.url().clone();
    let headers = req.headers().clone();
    let body_bytes = req
        .body()
        .and_then(|b| b.as_bytes().map(Bytes::copy_from_slice));

    let Some(bytes) = body_bytes else {
        return Err((rebuild(method, url, headers, None), "no body".into()));
    };

    let mut json: Value = match serde_json::from_slice(&bytes) {
        Ok(v) => v,
        Err(e) => {
            return Err((
                rebuild(method, url, headers, Some(bytes)),
                format!("not json: {}", e),
            ))
        }
    };

    // Only inject when the request actually has tools — `tool_choice` on
    // a no-tool request is meaningless and may be rejected.
    let has_tools = json
        .get("tools")
        .and_then(Value::as_array)
        .is_some_and(|a| !a.is_empty());
    if !has_tools {
        return Err((
            rebuild(method, url, headers, Some(bytes)),
            "no tools array".into(),
        ));
    }

    // Don't override an explicit choice the caller already set.
    if json.get("tool_choice").is_some() {
        return Err((
            rebuild(method, url, headers, Some(bytes)),
            "tool_choice already set".into(),
        ));
    }

    if let Value::Object(ref mut map) = json {
        map.insert("tool_choice".to_string(), choice.clone());
    } else {
        return Err((
            rebuild(method, url, headers, Some(bytes)),
            "body is not a JSON object".into(),
        ));
    }

    let new_body = match serde_json::to_vec(&json) {
        Ok(b) => Bytes::from(b),
        Err(e) => {
            return Err((
                rebuild(method, url, headers, Some(bytes)),
                format!("reserialize failed: {}", e),
            ))
        }
    };

    info!(choice = %choice, "tool_choice_applied");
    Ok(rebuild(method, url, headers, Some(new_body)))
}

fn rebuild(
    method: Method,
    url: reqwest::Url,
    headers: http::HeaderMap,
    body: Option<Bytes>,
) -> Request {
    let mut new = Request::new(method, url);
    *new.headers_mut() = headers;
    if let Some(b) = body {
        *new.body_mut() = Some(Body::from(b));
    }
    new
}
