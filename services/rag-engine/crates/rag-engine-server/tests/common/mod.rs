//! Shared harness for the integration tests under `tests/`.
//!
//! Every test in this file boots a REAL tonic server on `127.0.0.1:0`
//! (an OS-assigned ephemeral port) and returns the bound address + a
//! handle to shut it down. No in-process service mounting — the tests
//! connect over the loopback socket exactly like a production client
//! would. This matches the "transport is always real in tests" clause
//! of `internal/rag/doc/TESTING.md`.

#![allow(dead_code)]

use std::net::SocketAddr;
use std::time::Duration;

use rag_engine_proto::rag_engine_server::RagEngineServer;
use rag_engine_server::auth::SharedSecretAuth;
use rag_engine_server::metrics::{spawn_metrics_server, Metrics, MetricsServerHandle};
use rag_engine_server::middleware::MetricsLayer;
use rag_engine_server::{RagEngineService, GRPC_SERVICE_NAME};
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;
use tonic::transport::Server;

/// A running test server + the means to shut it down.
pub struct TestServer {
    pub addr: SocketAddr,
    shutdown_tx: Option<oneshot::Sender<()>>,
    handle: Option<JoinHandle<Result<(), tonic::transport::Error>>>,
}

impl TestServer {
    /// Spawn a server with the given shared secret. Returns once the
    /// server is listening and accepting connections on the returned
    /// ephemeral address.
    pub async fn start(shared_secret: &'static str) -> Self {
        // Bind first so we pick a concrete port before starting the
        // tonic accept loop. `TcpListener::bind("127.0.0.1:0")` gives
        // us an OS-assigned port we can then pass to tonic via
        // `serve_with_incoming`. We immediately close this listener
        // and re-bind inside tonic because tonic owns its accept
        // loop; the point is to claim a known port number.
        let probe = TcpListener::bind("127.0.0.1:0").await.expect("bind probe");
        let addr = probe.local_addr().expect("local_addr");
        drop(probe);

        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();

        let (mut health_reporter, health_service) = tonic_health::server::health_reporter();
        health_reporter
            .set_serving::<RagEngineServer<RagEngineService>>()
            .await;
        health_reporter
            .set_service_status(GRPC_SERVICE_NAME, tonic_health::ServingStatus::Serving)
            .await;

        let auth = SharedSecretAuth::new(shared_secret);
        let rag_service = RagEngineServer::with_interceptor(RagEngineService::new(), auth);

        let handle = tokio::spawn(async move {
            Server::builder()
                .add_service(health_service)
                .add_service(rag_service)
                .serve_with_shutdown(addr, async {
                    let _ = shutdown_rx.await;
                })
                .await
        });

        // Poll until the server is actually accepting connections —
        // tonic's serve() returns a future we've spawned, but the
        // listener isn't bound until it runs. We probe with a bare
        // TCP connect.
        wait_for_listening(addr).await;

        TestServer {
            addr,
            shutdown_tx: Some(shutdown_tx),
            handle: Some(handle),
        }
    }

    /// Turn `127.0.0.1:PORT` into `http://127.0.0.1:PORT` — what
    /// `tonic::transport::Endpoint::connect` wants.
    pub fn uri(&self) -> String {
        format!("http://{}", self.addr)
    }
}

impl Drop for TestServer {
    fn drop(&mut self) {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        if let Some(handle) = self.handle.take() {
            // Best-effort: give the server a moment to drain. We can't
            // .await in Drop, so we abort if it hangs.
            handle.abort();
        }
    }
}

async fn wait_for_listening(addr: SocketAddr) {
    use tokio::net::TcpStream;
    let deadline = std::time::Instant::now() + Duration::from_secs(5);
    loop {
        if TcpStream::connect(addr).await.is_ok() {
            return;
        }
        if std::time::Instant::now() > deadline {
            panic!("test server at {addr} never accepted a connection");
        }
        tokio::time::sleep(Duration::from_millis(25)).await;
    }
}

/// Attach a `Bearer <token>` authorization header to a tonic request.
pub fn with_bearer<T>(req: &mut tonic::Request<T>, token: &str) {
    let val: tonic::metadata::MetadataValue<_> = format!("Bearer {token}")
        .parse()
        .expect("valid bearer metadata");
    req.metadata_mut().insert("authorization", val);
}

// ---------------------------------------------------------------------------
// 2G helpers — observability harness
// ---------------------------------------------------------------------------

/// A running test server with the full 2G observability stack wired in:
///   * tonic gRPC server on ephemeral loopback port
///   * `MetricsLayer` over every RPC
///   * a companion `/metrics` HTTP server on its own ephemeral port
///
/// Tests use this when they need to assert on Prometheus scrape output
/// or on middleware behaviour. A dedicated `Metrics` instance (not the
/// process-global singleton) is used so parallel test invocations don't
/// race on counter values.
pub struct ObservabilityServer {
    pub grpc_addr: SocketAddr,
    pub metrics_addr: SocketAddr,
    pub metrics: Metrics,
    shutdown_tx: Option<oneshot::Sender<()>>,
    handle: Option<JoinHandle<Result<(), tonic::transport::Error>>>,
    metrics_handle: Option<MetricsServerHandle>,
}

impl ObservabilityServer {
    pub async fn start(shared_secret: &'static str) -> Self {
        // Pick two free ports (gRPC + metrics). Claim + drop to get a
        // concrete port number without holding the listener.
        let probe = TcpListener::bind("127.0.0.1:0").await.expect("probe grpc");
        let grpc_addr = probe.local_addr().expect("grpc addr");
        drop(probe);

        // Fresh isolated metrics instance per test — never the global.
        let metrics =
            Metrics::new().expect("fresh metrics registry for ObservabilityServer harness");

        let metrics_handle = spawn_metrics_server("127.0.0.1:0".parse().unwrap(), metrics.clone())
            .await
            .expect("spawn metrics server");
        let metrics_addr = metrics_handle.addr;

        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();

        let (mut health_reporter, health_service) = tonic_health::server::health_reporter();
        health_reporter
            .set_serving::<RagEngineServer<RagEngineService>>()
            .await;
        health_reporter
            .set_service_status(GRPC_SERVICE_NAME, tonic_health::ServingStatus::Serving)
            .await;

        let auth = SharedSecretAuth::new(shared_secret);
        let rag_service = RagEngineServer::with_interceptor(RagEngineService::new(), auth);
        let layer = MetricsLayer::new(metrics.clone());

        let handle = tokio::spawn(async move {
            Server::builder()
                .layer(layer)
                .add_service(health_service)
                .add_service(rag_service)
                .serve_with_shutdown(grpc_addr, async {
                    let _ = shutdown_rx.await;
                })
                .await
        });

        wait_for_listening(grpc_addr).await;
        wait_for_listening(metrics_addr).await;

        ObservabilityServer {
            grpc_addr,
            metrics_addr,
            metrics,
            shutdown_tx: Some(shutdown_tx),
            handle: Some(handle),
            metrics_handle: Some(metrics_handle),
        }
    }

    pub fn grpc_uri(&self) -> String {
        format!("http://{}", self.grpc_addr)
    }

    pub fn metrics_url(&self) -> String {
        format!("http://{}/metrics", self.metrics_addr)
    }
}

impl Drop for ObservabilityServer {
    fn drop(&mut self) {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        if let Some(h) = self.handle.take() {
            h.abort();
        }
        // `MetricsServerHandle::Drop` sends its own shutdown signal.
        drop(self.metrics_handle.take());
    }
}
