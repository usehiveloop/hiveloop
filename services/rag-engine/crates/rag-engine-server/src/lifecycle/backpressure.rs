//! Backpressure middleware: concurrency cap, per-RPC timeout, and
//! request-body-size ceiling. Each layer here is written to produce a
//! *proper gRPC response* (trailers-encoded `grpc-status`) on rejection
//! rather than an HTTP-level error that tonic's client would surface as
//! an opaque transport failure.
//!
//! # Why custom tower layers (not vanilla `tower::limit` /
//! `tower-http::limit`)
//!
//! * `tower::limit::ConcurrencyLimit` **queues** requests past the cap
//!   instead of rejecting them. A wedged handler under load produces
//!   unbounded memory growth. We want immediate rejection.
//! * `tower::load_shed::LoadShed` converts `poll_ready` "not ready" into
//!   an `Overloaded` *error* — tonic converts unmapped errors into a
//!   dropped connection, not a gRPC status the caller can retry on.
//! * `tower-http::limit::RequestBodyLimitLayer` returns HTTP 413. tonic's
//!   client maps 413 to `Code::Unknown` (per the gRPC HTTP-status
//!   mapping table in `tonic/src/status.rs`), not `RESOURCE_EXHAUSTED`.
//! * `tower-http::timeout::TimeoutLayer` returns HTTP 408 which tonic
//!   also maps to `Code::Unknown`.
//!
//! So each layer below is a small custom `tower::Service` that, on
//! rejection, produces `Status::{resource_exhausted, deadline_exceeded}
//! .into_http()` — a trailers-only response with the canonical
//! `grpc-status` header tonic's client unambiguously decodes into the
//! right `Code`.
//!
//! # Env variables (2H owns these)
//!
//! * `RAG_ENGINE_MAX_CONCURRENT`     — default 512
//! * `RAG_ENGINE_RPC_TIMEOUT_SECS`   — default 30
//! * `RAG_ENGINE_MAX_REQUEST_BYTES`  — default 67_108_864 (64 MiB)
//! * `RAG_ENGINE_DRAIN_DEADLINE_SECS` — handled in `shutdown.rs`

use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Duration;

use futures::future::BoxFuture;
use http::{HeaderValue, Request, Response};
use tokio::sync::Semaphore;
use tonic::body::BoxBody;
use tonic::Status;
use tower::{Layer, Service};

/// Tunable limits. Built from env via [`LimitsConfig::from_env`] or
/// hand-constructed in tests.
#[derive(Debug, Clone, Copy)]
pub struct LimitsConfig {
    /// Hard cap on in-flight gRPC requests. Requests arriving while the
    /// server is at the cap are rejected immediately with
    /// `RESOURCE_EXHAUSTED` — never queued.
    pub max_concurrent: usize,

    /// Server-side per-RPC deadline. Cheaper-than-client deadlines are
    /// not overridden: if the caller's `grpc-timeout` header is *shorter*
    /// than this value, tonic's own timeout logic fires first and returns
    /// `DEADLINE_EXCEEDED` to the caller. This layer only guards against
    /// clients that never sent a deadline.
    pub rpc_timeout: Duration,

    /// Maximum Content-Length (bytes) we'll accept on a gRPC request.
    /// Oversize requests get `RESOURCE_EXHAUSTED` before the inner
    /// handler ever runs, avoiding protobuf-decoding work on junk.
    pub max_request_bytes: usize,
}

impl Default for LimitsConfig {
    fn default() -> Self {
        Self {
            max_concurrent: 512,
            rpc_timeout: Duration::from_secs(30),
            max_request_bytes: 64 * 1024 * 1024,
        }
    }
}

impl LimitsConfig {
    /// Parse from `RAG_ENGINE_MAX_CONCURRENT`,
    /// `RAG_ENGINE_RPC_TIMEOUT_SECS`, and `RAG_ENGINE_MAX_REQUEST_BYTES`.
    /// Invalid values fall back to defaults — a misparsed env var
    /// should not prevent the server booting.
    pub fn from_env() -> Self {
        let default = Self::default();
        let max_concurrent = std::env::var("RAG_ENGINE_MAX_CONCURRENT")
            .ok()
            .and_then(|s| s.parse::<usize>().ok())
            .filter(|&n| n > 0)
            .unwrap_or(default.max_concurrent);
        let rpc_timeout = std::env::var("RAG_ENGINE_RPC_TIMEOUT_SECS")
            .ok()
            .and_then(|s| s.parse::<u64>().ok())
            .filter(|&n| n > 0)
            .map(Duration::from_secs)
            .unwrap_or(default.rpc_timeout);
        let max_request_bytes = std::env::var("RAG_ENGINE_MAX_REQUEST_BYTES")
            .ok()
            .and_then(|s| s.parse::<usize>().ok())
            .filter(|&n| n > 0)
            .unwrap_or(default.max_request_bytes);
        Self {
            max_concurrent,
            rpc_timeout,
            max_request_bytes,
        }
    }
}

// ---------------------------------------------------------------------------
// Concurrency layer — non-blocking, fail-fast.
// ---------------------------------------------------------------------------

/// Factory for the concurrency-limit layer. Produced by
/// [`concurrency_layer`].
#[derive(Clone)]
pub struct ConcurrencyLayer {
    semaphore: Arc<Semaphore>,
}

/// Build a concurrency-limit layer backed by a shared `Semaphore`.
/// `max` permits are allocated; over-the-cap requests are rejected
/// immediately with `RESOURCE_EXHAUSTED` (NOT queued).
pub fn concurrency_layer(max: usize) -> ConcurrencyLayer {
    ConcurrencyLayer {
        semaphore: Arc::new(Semaphore::new(max)),
    }
}

impl<S> Layer<S> for ConcurrencyLayer {
    type Service = ConcurrencyService<S>;

    fn layer(&self, inner: S) -> Self::Service {
        ConcurrencyService {
            inner,
            semaphore: self.semaphore.clone(),
        }
    }
}

#[derive(Clone)]
pub struct ConcurrencyService<S> {
    inner: S,
    semaphore: Arc<Semaphore>,
}

impl<S, B> Service<Request<B>> for ConcurrencyService<S>
where
    S: Service<Request<B>, Response = Response<BoxBody>> + Clone + Send + 'static,
    S::Future: Send + 'static,
    B: Send + 'static,
{
    type Response = S::Response;
    type Error = S::Error;
    type Future = BoxFuture<'static, Result<S::Response, S::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        // Always ready — admission is decided *inside* `call` via a
        // non-blocking `try_acquire_owned`. We specifically do NOT want
        // backpressure at `poll_ready` because tonic's Router calls
        // `poll_ready` on the inner service exactly once at startup;
        // making it ever return `Pending` would stall the whole server.
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, req: Request<B>) -> Self::Future {
        // Per tower's service-clone pattern: the already-primed `inner`
        // is the one we must call; leave a fresh clone behind for the
        // next invocation.
        let clone = self.inner.clone();
        let mut inner = std::mem::replace(&mut self.inner, clone);

        let permit = self.semaphore.clone().try_acquire_owned();

        Box::pin(async move {
            match permit {
                Ok(permit) => {
                    // Permit is released when the future resolves and
                    // the `_permit` binding drops. This is what gates
                    // the concurrency counter.
                    let _permit = permit;
                    inner.call(req).await
                }
                Err(_) => {
                    tracing::warn!(
                        "concurrency cap reached; rejecting request with RESOURCE_EXHAUSTED"
                    );
                    Ok(Status::resource_exhausted(
                        "server at concurrency limit; retry with backoff",
                    )
                    .into_http())
                }
            }
        })
    }
}

// ---------------------------------------------------------------------------
// Timeout layer — deadline exceeded → gRPC DEADLINE_EXCEEDED.
// ---------------------------------------------------------------------------

/// Factory for the per-RPC timeout layer. Produced by [`timeout_layer`].
#[derive(Clone, Copy)]
pub struct TimeoutLayer {
    timeout: Duration,
}

/// Build a timeout layer that caps each inner RPC at `timeout`. On
/// expiry the handler's future is dropped (cancelled) and a
/// `DEADLINE_EXCEEDED` gRPC status is returned.
///
/// The caller's `grpc-timeout` header is honoured by tonic's own
/// per-request timeout logic — this layer is a server-side ceiling, not
/// a floor. When the client asks for a shorter deadline, theirs wins.
pub fn timeout_layer(timeout: Duration) -> TimeoutLayer {
    TimeoutLayer { timeout }
}

impl<S> Layer<S> for TimeoutLayer {
    type Service = TimeoutService<S>;

    fn layer(&self, inner: S) -> Self::Service {
        TimeoutService {
            inner,
            timeout: self.timeout,
        }
    }
}

#[derive(Clone)]
pub struct TimeoutService<S> {
    inner: S,
    timeout: Duration,
}

impl<S, B> Service<Request<B>> for TimeoutService<S>
where
    S: Service<Request<B>, Response = Response<BoxBody>> + Clone + Send + 'static,
    S::Future: Send + 'static,
    B: Send + 'static,
{
    type Response = S::Response;
    type Error = S::Error;
    type Future = BoxFuture<'static, Result<S::Response, S::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, req: Request<B>) -> Self::Future {
        let clone = self.inner.clone();
        let mut inner = std::mem::replace(&mut self.inner, clone);
        let timeout = self.timeout;

        Box::pin(async move {
            match tokio::time::timeout(timeout, inner.call(req)).await {
                Ok(result) => result,
                Err(_elapsed) => {
                    tracing::warn!(
                        timeout_secs = timeout.as_secs_f64(),
                        "rpc exceeded server-side deadline"
                    );
                    Ok(Status::deadline_exceeded(format!(
                        "rpc exceeded server-side deadline of {:?}",
                        timeout
                    ))
                    .into_http())
                }
            }
        })
    }
}

// ---------------------------------------------------------------------------
// Body-size limit layer — oversize requests → RESOURCE_EXHAUSTED.
// ---------------------------------------------------------------------------

/// Factory for the request-body-size layer.
#[derive(Clone, Copy)]
pub struct BodyLimitLayer {
    max_bytes: usize,
}

/// Reject requests whose `content-length` exceeds `max_bytes`.
/// Requests without a `content-length` header (streaming uploads) are
/// NOT gated here — tonic's `max_decoding_message_size` catches them
/// at protobuf-decode time.
pub fn body_size_limit_layer(max_bytes: usize) -> BodyLimitLayer {
    BodyLimitLayer { max_bytes }
}

impl<S> Layer<S> for BodyLimitLayer {
    type Service = BodyLimitService<S>;

    fn layer(&self, inner: S) -> Self::Service {
        BodyLimitService {
            inner,
            max_bytes: self.max_bytes,
        }
    }
}

#[derive(Clone)]
pub struct BodyLimitService<S> {
    inner: S,
    max_bytes: usize,
}

impl<S, B> Service<Request<B>> for BodyLimitService<S>
where
    S: Service<Request<B>, Response = Response<BoxBody>> + Clone + Send + 'static,
    S::Future: Send + 'static,
    B: Send + 'static,
{
    type Response = S::Response;
    type Error = S::Error;
    type Future = BoxFuture<'static, Result<S::Response, S::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, req: Request<B>) -> Self::Future {
        let clone = self.inner.clone();
        let mut inner = std::mem::replace(&mut self.inner, clone);
        let max_bytes = self.max_bytes;

        // Honour `content-length` if the client supplied one. The gRPC
        // default framing is HTTP/2 with a length-prefixed message, so
        // `content-length` is typically set by the `tonic` client on
        // unary requests. Streaming requests often omit it; for those
        // tonic's `max_decoding_message_size` provides the second line
        // of defence.
        if let Some(value) = content_length(req.headers_ref()) {
            if value > max_bytes as u64 {
                tracing::warn!(
                    content_length = value,
                    max_bytes,
                    "rejecting oversize request with RESOURCE_EXHAUSTED"
                );
                return Box::pin(async move {
                    Ok(Status::resource_exhausted(format!(
                        "request body {value} bytes exceeds server limit of {max_bytes} bytes"
                    ))
                    .into_http())
                });
            }
        }

        Box::pin(async move { inner.call(req).await })
    }
}

/// Small helper: read a `u64` from a `content-length` header if
/// present + parseable. Tolerant of malformed values (treated as
/// "unknown length" — fall through to the inner service).
fn content_length(headers: Option<&http::HeaderMap>) -> Option<u64> {
    let headers = headers?;
    let value: &HeaderValue = headers.get(http::header::CONTENT_LENGTH)?;
    value.to_str().ok()?.parse::<u64>().ok()
}

// Trait helper: extracting `&HeaderMap` from an `http::Request<B>`
// without forcing us to own `req`. `Request::headers()` returns
// `&HeaderMap` but `.headers_ref()` is our own shorthand because we
// accept generic `Request<B>` and the method name `headers` collides
// with our local helper. The impl below provides exactly that.
trait HeadersRef {
    fn headers_ref(&self) -> Option<&http::HeaderMap>;
}

impl<B> HeadersRef for Request<B> {
    fn headers_ref(&self) -> Option<&http::HeaderMap> {
        Some(self.headers())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn defaults_match_spec() {
        let cfg = LimitsConfig::default();
        assert_eq!(cfg.max_concurrent, 512);
        assert_eq!(cfg.rpc_timeout, Duration::from_secs(30));
        assert_eq!(cfg.max_request_bytes, 64 * 1024 * 1024);
    }

    #[test]
    fn content_length_parses_standard_headers() {
        let mut headers = http::HeaderMap::new();
        headers.insert(
            http::header::CONTENT_LENGTH,
            HeaderValue::from_static("1234"),
        );
        assert_eq!(content_length(Some(&headers)), Some(1234));
    }

    #[test]
    fn content_length_none_when_absent() {
        let headers = http::HeaderMap::new();
        assert_eq!(content_length(Some(&headers)), None);
    }

    #[test]
    fn content_length_ignores_garbage() {
        let mut headers = http::HeaderMap::new();
        headers.insert(
            http::header::CONTENT_LENGTH,
            HeaderValue::from_static("not-a-number"),
        );
        assert_eq!(content_length(Some(&headers)), None);
    }
}
