//! Convert a panic thrown inside a gRPC handler into a proper
//! `grpc-status: 13 (INTERNAL)` response, so one bad request does not
//! kill the connection (let alone the process).
//!
//! # Why not `tower_http::catch_panic::CatchPanicLayer`?
//!
//! `tower-http`'s `CatchPanicLayer` is the obvious answer, but its
//! response-body type is
//! `UnsyncBoxBody<Bytes, tower::BoxError>` — whereas tonic's internal
//! `Server` expects `UnsyncBoxBody<Bytes, tonic::Status>` (aliased as
//! `tonic::body::BoxBody`). The error-parameter mismatch means stacking
//! `CatchPanicLayer` inside `Server::builder().layer(...)` forces a
//! body remap on every response — doable but noisy, and it hides the
//! single assumption that a tonic-internal tower stack always speaks
//! `tonic::body::BoxBody`.
//!
//! Writing this layer ourselves keeps the service chain strictly
//! `Request<B> → Response<tonic::body::BoxBody>` and lets us reuse the
//! exact `Status::internal(...).into_http()` path every other
//! rejection uses.
//!
//! # Relationship to the process-wide panic hook
//!
//! `crate::panic::install_panic_handler` installs a `std::panic::set_hook`
//! that runs for *every* panic — regardless of whether the panic was
//! caught. That hook owns the `rag_engine_panics_total` counter and
//! the structured log. This layer is orthogonal: it wraps each handler
//! in `std::panic::catch_unwind` so the unwinding stops at the request
//! boundary, and it returns `Status::internal(...)` in the response.
//!
//! Both pieces are needed. Without the hook, we'd lose the counter and
//! structured log; without the layer, hyper drops the connection when
//! a handler panics and the client sees `UNAVAILABLE`/`CANCELLED`
//! instead of `INTERNAL`.

use std::panic::AssertUnwindSafe;
use std::task::{Context, Poll};

use futures::future::{BoxFuture, FutureExt};
use http::{Request, Response};
use tonic::body::BoxBody;
use tonic::Status;
use tower::{Layer, Service};

/// A tower layer that catches panics inside the inner service's
/// `call`-returned future, logs via the installed panic hook (by
/// resuming the unwind inside `catch_unwind` — which invokes the
/// installed hook before unwinding stops), and returns a gRPC
/// `INTERNAL` response to the client.
#[derive(Clone, Copy, Default)]
pub struct CatchPanicLayer;

impl CatchPanicLayer {
    pub const fn new() -> Self {
        Self
    }
}

/// Build the catch-panic layer. Exposed as a free function for symmetry
/// with the other lifecycle layers.
pub fn grpc_catch_panic_layer() -> CatchPanicLayer {
    CatchPanicLayer::new()
}

impl<S> Layer<S> for CatchPanicLayer {
    type Service = CatchPanicService<S>;

    fn layer(&self, inner: S) -> Self::Service {
        CatchPanicService { inner }
    }
}

#[derive(Clone)]
pub struct CatchPanicService<S> {
    inner: S,
}

impl<S, B> Service<Request<B>> for CatchPanicService<S>
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

        // `catch_unwind` on the *synchronous* `inner.call(req)` catches
        // panics that fire before the first `.await` inside the handler
        // (e.g. a `panic!` at the top of the async fn). For panics that
        // fire *during* polling of the returned future, we wrap the
        // polling in a small adapter that re-invokes `catch_unwind` on
        // every `poll` call.
        let make_fut = std::panic::catch_unwind(AssertUnwindSafe(move || inner.call(req)));
        Box::pin(async move {
            match make_fut {
                Ok(fut) => {
                    // `AssertUnwindSafe` is justified: tonic's tower stack
                    // is not designed to be resumed across a panic; the
                    // enclosing request is already being failed, and a
                    // lingering mutable-state issue across a panic would
                    // manifest as a later panic we'd also catch.
                    match AssertUnwindSafe(fut).catch_unwind().await {
                        Ok(result) => result,
                        Err(_payload) => {
                            // Panic hook already logged + counted. We
                            // just return the gRPC INTERNAL response.
                            Ok(Status::internal("internal server error").into_http())
                        }
                    }
                }
                Err(_payload) => Ok(Status::internal("internal server error").into_http()),
            }
        })
    }
}
