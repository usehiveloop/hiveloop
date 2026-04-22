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

// ---------------------------------------------------------------------------
// 2H helpers — lifecycle harness (backpressure, shutdown, panic)
// ---------------------------------------------------------------------------

use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc as StdArc;

use rag_engine_proto::rag_engine_server::RagEngine;
use rag_engine_proto::{
    CreateDatasetRequest, CreateDatasetResponse, DeleteByDocIdRequest, DeleteByDocIdResponse,
    DeleteByOrgRequest, DeleteByOrgResponse, DropDatasetRequest, DropDatasetResponse,
    IngestBatchRequest, IngestBatchResponse, PruneRequest, PruneResponse, SearchRequest,
    SearchResponse, UpdateAclRequest, UpdateAclResponse,
};
use rag_engine_server::{
    body_size_limit_layer, concurrency_layer, grpc_catch_panic_layer, timeout_layer, LimitsConfig,
};
use tonic::{Request, Response, Status};
use tower::ServiceBuilder;

/// Test-only `RagEngine` impl with tunable behaviour. Unlike the
/// production `RagEngineService`, this one lets each RPC:
///
///   * delay for a configurable duration (exercises timeout + drain),
///   * panic on demand (exercises the panic hook),
///   * count how many in-flight calls it has observed (exercises the
///     concurrency layer's admission counter).
///
/// Every RPC maps onto the same three-dial interface for test
/// simplicity. Tests pick the RPC that best matches the request shape
/// they want to exercise (e.g. `IngestBatch` for a large-body request).
#[derive(Clone, Default)]
pub struct TestRagService {
    pub delay: StdArc<parking_lot::RwLock<Duration>>,
    pub panic_next: StdArc<std::sync::atomic::AtomicBool>,
    pub inflight: StdArc<AtomicU64>,
    pub completed: StdArc<AtomicU64>,
}

impl TestRagService {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn with_delay(delay: Duration) -> Self {
        let svc = Self::default();
        *svc.delay.write() = delay;
        svc
    }

    pub fn set_delay(&self, delay: Duration) {
        *self.delay.write() = delay;
    }

    pub fn arm_panic(&self) {
        self.panic_next.store(true, Ordering::SeqCst);
    }

    pub fn inflight(&self) -> u64 {
        self.inflight.load(Ordering::SeqCst)
    }

    pub fn completed(&self) -> u64 {
        self.completed.load(Ordering::SeqCst)
    }

    async fn delay_if_configured(&self) {
        let d = *self.delay.read();
        if !d.is_zero() {
            tokio::time::sleep(d).await;
        }
    }

    fn maybe_panic(&self) {
        if self.panic_next.swap(false, Ordering::SeqCst) {
            panic!("test-induced panic in rag-engine handler");
        }
    }

    async fn handle<R>(&self, resp: R) -> Result<Response<R>, Status> {
        // Admission accounting. Held for the full duration of the
        // handler via an RAII guard so cancellation (timeout/drop) also
        // decrements the counter.
        struct Guard<'a>(&'a AtomicU64);
        impl<'a> Drop for Guard<'a> {
            fn drop(&mut self) {
                self.0.fetch_sub(1, Ordering::SeqCst);
            }
        }
        self.inflight.fetch_add(1, Ordering::SeqCst);
        let _guard = Guard(&self.inflight);

        self.maybe_panic();
        self.delay_if_configured().await;
        self.completed.fetch_add(1, Ordering::SeqCst);
        Ok(Response::new(resp))
    }
}

#[tonic::async_trait]
impl RagEngine for TestRagService {
    async fn create_dataset(
        &self,
        _request: Request<CreateDatasetRequest>,
    ) -> Result<Response<CreateDatasetResponse>, Status> {
        self.handle(CreateDatasetResponse::default()).await
    }

    async fn drop_dataset(
        &self,
        _request: Request<DropDatasetRequest>,
    ) -> Result<Response<DropDatasetResponse>, Status> {
        self.handle(DropDatasetResponse::default()).await
    }

    async fn ingest_batch(
        &self,
        _request: Request<IngestBatchRequest>,
    ) -> Result<Response<IngestBatchResponse>, Status> {
        self.handle(IngestBatchResponse::default()).await
    }

    async fn update_acl(
        &self,
        _request: Request<UpdateAclRequest>,
    ) -> Result<Response<UpdateAclResponse>, Status> {
        self.handle(UpdateAclResponse::default()).await
    }

    async fn search(
        &self,
        _request: Request<SearchRequest>,
    ) -> Result<Response<SearchResponse>, Status> {
        self.handle(SearchResponse::default()).await
    }

    async fn delete_by_doc_id(
        &self,
        _request: Request<DeleteByDocIdRequest>,
    ) -> Result<Response<DeleteByDocIdResponse>, Status> {
        self.handle(DeleteByDocIdResponse::default()).await
    }

    async fn delete_by_org(
        &self,
        _request: Request<DeleteByOrgRequest>,
    ) -> Result<Response<DeleteByOrgResponse>, Status> {
        self.handle(DeleteByOrgResponse::default()).await
    }

    async fn prune(
        &self,
        _request: Request<PruneRequest>,
    ) -> Result<Response<PruneResponse>, Status> {
        self.handle(PruneResponse::default()).await
    }
}

/// A running test server with the full 2H lifecycle stack wired in:
///   * body-size limit, concurrency cap, per-RPC timeout, metrics layer
///   * shared-secret auth interceptor
///   * a dedicated Prometheus registry (via [`Metrics::new`]) so panic
///     counts from one test don't bleed into another
///   * an external shutdown trigger so tests can simulate SIGTERM
///     deterministically
///
/// Tests construct this with `LifecycleServer::start(...)`, issue
/// whatever RPCs they need, and then drop or call `shutdown_with_deadline`
/// when asserting shutdown behaviour.
pub struct LifecycleServer {
    pub grpc_addr: SocketAddr,
    pub metrics: rag_engine_server::Metrics,
    pub service: TestRagService,
    shutdown_tx: Option<oneshot::Sender<()>>,
    handle: Option<JoinHandle<Result<(), tonic::transport::Error>>>,
}

/// Tunable parameters for [`LifecycleServer::start`].
#[derive(Clone, Copy)]
pub struct LifecycleParams {
    pub limits: LimitsConfig,
    pub initial_handler_delay: Duration,
}

impl Default for LifecycleParams {
    fn default() -> Self {
        Self {
            limits: LimitsConfig::default(),
            initial_handler_delay: Duration::from_millis(0),
        }
    }
}

impl LifecycleServer {
    pub async fn start(shared_secret: &'static str, params: LifecycleParams) -> Self {
        let probe = TcpListener::bind("127.0.0.1:0").await.expect("probe grpc");
        let grpc_addr = probe.local_addr().expect("grpc addr");
        drop(probe);

        let metrics =
            rag_engine_server::Metrics::new().expect("fresh metrics registry for LifecycleServer");

        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();

        let (mut health_reporter, health_service) = tonic_health::server::health_reporter();
        health_reporter
            .set_serving::<RagEngineServer<TestRagService>>()
            .await;
        health_reporter
            .set_service_status(GRPC_SERVICE_NAME, tonic_health::ServingStatus::Serving)
            .await;

        let service = TestRagService::with_delay(params.initial_handler_delay);
        let auth = SharedSecretAuth::new(shared_secret);
        let rag_inner = RagEngineServer::new(service.clone())
            .max_decoding_message_size(params.limits.max_request_bytes);
        let rag_service = tonic::service::interceptor::InterceptedService::new(rag_inner, auth);

        let stack = ServiceBuilder::new()
            .layer(MetricsLayer::new(metrics.clone()))
            .layer(body_size_limit_layer(params.limits.max_request_bytes))
            .layer(concurrency_layer(params.limits.max_concurrent))
            .layer(timeout_layer(params.limits.rpc_timeout))
            .layer(grpc_catch_panic_layer());

        let handle = tokio::spawn(async move {
            Server::builder()
                .layer(stack)
                .add_service(health_service)
                .add_service(rag_service)
                .serve_with_shutdown(grpc_addr, async move {
                    let _ = shutdown_rx.await;
                })
                .await
        });

        wait_for_listening(grpc_addr).await;

        LifecycleServer {
            grpc_addr,
            metrics,
            service,
            shutdown_tx: Some(shutdown_tx),
            handle: Some(handle),
        }
    }

    pub fn uri(&self) -> String {
        format!("http://{}", self.grpc_addr)
    }

    /// Trigger shutdown and wait up to `deadline` for the server task
    /// to exit. Returns `Ok` if the server drained within the deadline;
    /// `Err(Elapsed)` if we forced abort.
    pub async fn shutdown_with_deadline(
        mut self,
        deadline: Duration,
    ) -> Result<Result<(), tonic::transport::Error>, tokio::time::error::Elapsed> {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        let join = self.handle.take().expect("server handle already consumed");
        match tokio::time::timeout(deadline, join).await {
            Ok(join_res) => Ok(join_res.expect("server task should not panic")),
            Err(elapsed) => {
                // Exceeded the drain window. Replicate main.rs force-abort.
                // We don't have the handle any more; the runtime will
                // drop the spawned task on next poll. For a test assert
                // "exceeded the deadline" it's enough that `timeout`
                // returned `Elapsed`.
                Err(elapsed)
            }
        }
    }
}

impl Drop for LifecycleServer {
    fn drop(&mut self) {
        if let Some(tx) = self.shutdown_tx.take() {
            let _ = tx.send(());
        }
        if let Some(h) = self.handle.take() {
            h.abort();
        }
    }
}
