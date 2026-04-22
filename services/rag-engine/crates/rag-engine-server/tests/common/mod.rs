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
use std::sync::Arc;
use std::time::Duration;

use rag_engine_chunker::tokenizer::TiktokenTokenizer;
use rag_engine_chunker::{Chunker, ChunkerConfig};
use rag_engine_embed::{Embedder, FakeEmbedder};
use rag_engine_lance::{LanceStore, StoreConfig};
use rag_engine_proto::rag_engine_server::RagEngineServer;
use rag_engine_rerank::{FakeReranker, Reranker};
use rag_engine_server::auth::SharedSecretAuth;
use rag_engine_server::metrics::{spawn_metrics_server, Metrics, MetricsServerHandle};
use rag_engine_server::middleware::MetricsLayer;
use rag_engine_server::{AppState, RagEngineService, StateLimits, GRPC_SERVICE_NAME};
use tempfile::TempDir;
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;
use tonic::transport::Server;

/// Default vector dim used across the 2F tests. Small enough for fast
/// fakes, large enough that dim-mismatch tests have something to differ
/// against.
pub const TEST_DIM: u32 = 128;

/// Build a minimal `AppState` backed by fakes + a local lance store in a
/// `TempDir`. The `TempDir` handle is returned so the caller can keep
/// the directory alive for the duration of the test; dropping it cleans
/// up the on-disk data.
pub async fn build_test_state(dim: u32) -> (Arc<AppState>, TempDir) {
    let tempdir = tempfile::tempdir().expect("tempdir");
    let uri = tempdir.path().to_string_lossy().to_string();
    let store = LanceStore::open(StoreConfig::Local { uri })
        .await
        .expect("open lance store");

    let embedder: Arc<dyn Embedder> = Arc::new(FakeEmbedder::new("fake", dim));
    let reranker: Arc<dyn Reranker> = Arc::new(FakeReranker::new());
    let chunker = Arc::new(Chunker::new(
        TiktokenTokenizer::cl100k_base(),
        ChunkerConfig::default(),
    ));
    let limits = StateLimits::defaults();
    let state = Arc::new(AppState::new(store, embedder, reranker, chunker, limits));
    (state, tempdir)
}

/// A running test server + the means to shut it down.
pub struct TestServer {
    pub addr: SocketAddr,
    /// Kept alive so the lance tempdir isn't removed under the server.
    _tempdir: Option<TempDir>,
    /// Kept so tests can reach into the state for assertions.
    pub state: Option<Arc<AppState>>,
    shutdown_tx: Option<oneshot::Sender<()>>,
    handle: Option<JoinHandle<Result<(), tonic::transport::Error>>>,
}

impl TestServer {
    /// Spawn a server with the given shared secret. Returns once the
    /// server is listening. Builds a fresh `AppState` backed by fakes.
    pub async fn start(shared_secret: &'static str) -> Self {
        let (state, tempdir) = build_test_state(TEST_DIM).await;
        Self::start_with_state(shared_secret, state, Some(tempdir)).await
    }

    /// Variant that accepts a pre-built state (e.g. built against a real
    /// MinIO container or with a different dim).
    pub async fn start_with_state(
        shared_secret: &'static str,
        state: Arc<AppState>,
        tempdir: Option<TempDir>,
    ) -> Self {
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
        let rag_service =
            RagEngineServer::with_interceptor(RagEngineService::new(state.clone()), auth);

        let handle = tokio::spawn(async move {
            Server::builder()
                .add_service(health_service)
                .add_service(rag_service)
                .serve_with_shutdown(addr, async {
                    let _ = shutdown_rx.await;
                })
                .await
        });

        wait_for_listening(addr).await;

        TestServer {
            addr,
            _tempdir: tempdir,
            state: Some(state),
            shutdown_tx: Some(shutdown_tx),
            handle: Some(handle),
        }
    }

    /// Turn `127.0.0.1:PORT` into `http://127.0.0.1:PORT`.
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
// 2G observability harness — carried forward from 2G with 2F's AppState
// ---------------------------------------------------------------------------

pub struct ObservabilityServer {
    pub grpc_addr: SocketAddr,
    pub metrics_addr: SocketAddr,
    pub metrics: Metrics,
    _tempdir: Option<TempDir>,
    pub state: Arc<AppState>,
    shutdown_tx: Option<oneshot::Sender<()>>,
    handle: Option<JoinHandle<Result<(), tonic::transport::Error>>>,
    metrics_handle: Option<MetricsServerHandle>,
}

impl ObservabilityServer {
    pub async fn start(shared_secret: &'static str) -> Self {
        let probe = TcpListener::bind("127.0.0.1:0").await.expect("probe grpc");
        let grpc_addr = probe.local_addr().expect("grpc addr");
        drop(probe);

        let metrics =
            Metrics::new().expect("fresh metrics registry for ObservabilityServer harness");

        let metrics_handle = spawn_metrics_server("127.0.0.1:0".parse().unwrap(), metrics.clone())
            .await
            .expect("spawn metrics server");
        let metrics_addr = metrics_handle.addr;

        let (state, tempdir) = build_test_state(TEST_DIM).await;

        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();

        let (mut health_reporter, health_service) = tonic_health::server::health_reporter();
        health_reporter
            .set_serving::<RagEngineServer<RagEngineService>>()
            .await;
        health_reporter
            .set_service_status(GRPC_SERVICE_NAME, tonic_health::ServingStatus::Serving)
            .await;

        let auth = SharedSecretAuth::new(shared_secret);
        let rag_service =
            RagEngineServer::with_interceptor(RagEngineService::new(state.clone()), auth);
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
            _tempdir: Some(tempdir),
            state,
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
        drop(self.metrics_handle.take());
    }
}
