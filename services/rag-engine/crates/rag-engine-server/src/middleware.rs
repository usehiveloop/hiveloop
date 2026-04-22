//! Tower middleware used by the tonic server.
//!
//! Two concerns live here:
//!
//!   1. **Metrics recording.** A `tower::Layer` (`MetricsLayer`) wraps
//!      every unary gRPC handler. For each call it increments the
//!      inflight gauge on entry, starts a histogram timer, and on exit
//!      decrements the gauge + records the duration + increments
//!      `rag_engine_rpc_total{method, code}`.
//!
//!   2. **Tracing spans.** Each request is wrapped in an `info_span!`
//!      with `rpc.service`, `rpc.method`, and `otel.kind = "server"`
//!      attributes. `tracing-opentelemetry` propagates the span into
//!      the OTLP exporter when it's configured. Incoming
//!      `traceparent` headers from the Go caller are extracted and
//!      attached as the parent context so Go → Rust traces chain.
//!
//! # Why an `http::Request<B>`-level layer?
//!
//! tonic builds its service tower over `http::Request` / `http::Response`
//! (hyper is the underlying transport). Intercepting at the HTTP layer
//! lets us read/write raw headers — specifically `traceparent` — and
//! decode the gRPC method from the path (`/<service>/<method>`). A
//! tonic-level interceptor would run *after* header parsing but without
//! access to the response status, which we need for the `code` label.

use std::sync::Arc;
use std::task::{Context, Poll};
use std::time::Instant;

use futures::future::BoxFuture;
use http::{HeaderMap, Request, Response};
use opentelemetry::global;
use opentelemetry::propagation::Extractor;
use tonic::body::BoxBody;
use tower::{Layer, Service};
use tracing::{field::Empty, info_span, Instrument, Span};
use tracing_opentelemetry::OpenTelemetrySpanExt;

use crate::metrics::Metrics;

/// Tower `Layer` that installs `MetricsService` around the inner tonic
/// service stack. Clones cheaply (`Metrics` is internally `Arc`-backed).
#[derive(Clone)]
pub struct MetricsLayer {
    metrics: Arc<Metrics>,
}

impl MetricsLayer {
    pub fn new(metrics: Metrics) -> Self {
        Self {
            metrics: Arc::new(metrics),
        }
    }
}

impl<S> Layer<S> for MetricsLayer {
    type Service = MetricsService<S>;

    fn layer(&self, inner: S) -> Self::Service {
        MetricsService {
            inner,
            metrics: self.metrics.clone(),
        }
    }
}

/// The wrapping service. Records duration + status + inflight for every
/// request passing through it.
#[derive(Clone)]
pub struct MetricsService<S> {
    inner: S,
    metrics: Arc<Metrics>,
}

impl<S, B> Service<Request<B>> for MetricsService<S>
where
    S: Service<Request<B>, Response = Response<BoxBody>> + Clone + Send + 'static,
    S::Future: Send + 'static,
    S::Error: std::fmt::Display + Send + 'static,
    B: Send + 'static,
{
    type Response = S::Response;
    type Error = S::Error;
    type Future = BoxFuture<'static, Result<S::Response, S::Error>>;

    fn poll_ready(&mut self, cx: &mut Context<'_>) -> Poll<Result<(), Self::Error>> {
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, req: Request<B>) -> Self::Future {
        // `tower` requires that we use the cloned service (see the
        // "oneshot" pattern in tower docs) — the `poll_ready` contract
        // is per-service-clone, and the inner one we already primed
        // is the *original*, not the one we'd move into the future.
        let clone = self.inner.clone();
        let mut inner = std::mem::replace(&mut self.inner, clone);

        let metrics = self.metrics.clone();

        // Derive `(service, method)` from the gRPC path. The path
        // format is `/<package>.<service>/<method>`. If it doesn't
        // match, we still record under `"unknown"` rather than drop
        // the observation — a mislabelled row is better than a hole.
        let (rpc_service, rpc_method) = split_grpc_path(req.uri().path());

        // Pull incoming trace context from headers and make it the
        // parent of our new span. `global::get_text_map_propagator`
        // is installed by `telemetry::init`; if it's not installed
        // (e.g. test without OTel), this is a no-op and we just start
        // a root span.
        let parent_cx = global::get_text_map_propagator(|prop| {
            prop.extract(&HeaderMapExtractor(req.headers()))
        });

        let span = info_span!(
            "grpc.request",
            otel.kind = "server",
            rpc.system = "grpc",
            rpc.service = %rpc_service,
            rpc.method = %rpc_method,
            grpc.status_code = Empty,
        );
        span.set_parent(parent_cx);

        let method_label = rpc_method.to_string();
        let inflight = metrics
            .inflight_requests
            .with_label_values(&[&method_label]);
        inflight.inc();
        let timer = metrics
            .rpc_duration_seconds
            .with_label_values(&[&method_label])
            .start_timer();
        let started = Instant::now();

        Box::pin(
            async move {
                let result = inner.call(req).await;

                // Record duration *before* we consume the result, so
                // errors and successes share the same histogram.
                drop(timer);
                let _elapsed = started.elapsed();

                let code_label = match &result {
                    Ok(resp) => grpc_status_from_response(resp),
                    Err(_) => "UNKNOWN".to_string(),
                };
                metrics
                    .rpc_total
                    .with_label_values(&[&method_label, &code_label])
                    .inc();
                Span::current().record("grpc.status_code", tracing::field::display(&code_label));

                inflight.dec();
                result
            }
            .instrument(span),
        )
    }
}

/// Adapter to let `opentelemetry`'s `Extractor` trait read from
/// `http::HeaderMap`. The OTel SDK ships this for `hyper` 0.x header
/// maps but not for `http` 1.x's — it's one trait method, trivial to
/// implement.
struct HeaderMapExtractor<'a>(&'a HeaderMap);

impl Extractor for HeaderMapExtractor<'_> {
    fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).and_then(|v| v.to_str().ok())
    }

    fn keys(&self) -> Vec<&str> {
        self.0.keys().map(|k| k.as_str()).collect()
    }
}

fn split_grpc_path(path: &str) -> (&str, &str) {
    // `/<service>/<method>` — split on the single non-leading `/`.
    let trimmed = path.strip_prefix('/').unwrap_or(path);
    match trimmed.split_once('/') {
        Some((svc, method)) => (svc, method),
        None => ("unknown", "unknown"),
    }
}

/// Map the tonic response's `grpc-status` trailer (or header for
/// trailer-less errors) to a label. Tonic sets the `grpc-status`
/// header on error responses even before body polling; for successful
/// responses it lives in the trailer, which we can't read
/// synchronously here — we treat any 2xx with no explicit `grpc-status`
/// as `OK` (which is what the trailer will eventually say).
fn grpc_status_from_response(resp: &Response<BoxBody>) -> String {
    if let Some(status) = resp.headers().get("grpc-status") {
        if let Ok(s) = status.to_str() {
            return code_str_from_numeric(s).to_string();
        }
    }
    // HTTP status 200 with no grpc-status header → assumed OK at the
    // point the handler returns; the trailer will confirm.
    "OK".to_string()
}

/// Translate a tonic numeric `grpc-status` string into the canonical
/// textual code. We keep this ourselves rather than depending on
/// `tonic::Code::from_i32 + Debug` because tonic's `Debug` emits
/// CamelCase (`InvalidArgument`) while the gRPC ecosystem convention
/// (and our metric label) is SCREAMING_SNAKE_CASE.
fn code_str_from_numeric(s: &str) -> &'static str {
    match s {
        "0" => "OK",
        "1" => "CANCELLED",
        "2" => "UNKNOWN",
        "3" => "INVALID_ARGUMENT",
        "4" => "DEADLINE_EXCEEDED",
        "5" => "NOT_FOUND",
        "6" => "ALREADY_EXISTS",
        "7" => "PERMISSION_DENIED",
        "8" => "RESOURCE_EXHAUSTED",
        "9" => "FAILED_PRECONDITION",
        "10" => "ABORTED",
        "11" => "OUT_OF_RANGE",
        "12" => "UNIMPLEMENTED",
        "13" => "INTERNAL",
        "14" => "UNAVAILABLE",
        "15" => "DATA_LOSS",
        "16" => "UNAUTHENTICATED",
        _ => "UNKNOWN",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn split_grpc_path_handles_canonical_paths() {
        assert_eq!(
            split_grpc_path("/hiveloop.rag.v1.RagEngine/Search"),
            ("hiveloop.rag.v1.RagEngine", "Search")
        );
    }

    #[test]
    fn split_grpc_path_falls_back_on_unknown() {
        // Only-slash and no-slash both have no second path segment, so
        // we can't derive `(service, method)` — the middleware records
        // against the "unknown" label rather than dropping the sample.
        assert_eq!(split_grpc_path("/"), ("unknown", "unknown"));
        assert_eq!(split_grpc_path("no-slash"), ("unknown", "unknown"));
    }

    #[test]
    fn code_str_from_numeric_covers_the_canonical_set() {
        assert_eq!(code_str_from_numeric("0"), "OK");
        assert_eq!(code_str_from_numeric("3"), "INVALID_ARGUMENT");
        assert_eq!(code_str_from_numeric("12"), "UNIMPLEMENTED");
        assert_eq!(code_str_from_numeric("99"), "UNKNOWN");
    }
}
