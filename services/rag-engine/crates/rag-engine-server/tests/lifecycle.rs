//! Integration tests for Tranche 2H — graceful shutdown, backpressure
//! (concurrency cap, per-RPC timeout, request-body size limit), and the
//! panic handler.
//!
//! Every test here spawns a real tonic server on an ephemeral loopback
//! port and drives it with a real `RagEngineClient`. No in-process
//! service mounting and no mocked transport — this matches the
//! "transport is always real in tests" clause of `TESTING.md`.

// `tonic::Status` is ~176 B, so clippy's `result_large_err` fires on
// closures that carry `Result<_, Status>` across a `.map`. It's the
// idiomatic tonic return type — boxing it would add allocations on
// every error path. Allowed at the test-file level.
#![allow(clippy::result_large_err)]

mod common;

use std::sync::Arc;
use std::time::Duration;

use common::{with_bearer, LifecycleParams, LifecycleServer, TestRagService};
use rag_engine_proto::rag_engine_client::RagEngineClient;
use rag_engine_proto::{DocumentToIngest, IngestBatchRequest, SearchRequest, Section};
use rag_engine_server::LimitsConfig;
use tokio::sync::Barrier;
use tonic::transport::{Channel, Endpoint};

const SECRET: &str = "lifecycle-test-secret";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async fn connect(uri: &str) -> Channel {
    Endpoint::from_shared(uri.to_string())
        .expect("endpoint")
        .connect()
        .await
        .expect("connect")
}

fn search_req() -> tonic::Request<SearchRequest> {
    let mut req = tonic::Request::new(SearchRequest {
        dataset_name: "rag_chunks__fake__128".into(),
        org_id: "org-lifecycle".into(),
        query_text: "hello".into(),
        query_vector: vec![],
        mode: 0,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        candidate_pool: 20,
        custom_sql_filter: String::new(),
        hybrid_alpha: 0.5,
        rerank: false,
        doc_updated_after: None,
    });
    with_bearer(&mut req, SECRET);
    req
}

// ---------------------------------------------------------------------------
// 1. Concurrency limit
// ---------------------------------------------------------------------------

/// Business behavior: a request arriving while the server is already
/// at `max_concurrent` returns `RESOURCE_EXHAUSTED` immediately — not
/// queued, not blocked. This is the production semantic that prevents
/// memory-exhaustion under sustained overload.
#[tokio::test]
async fn test_concurrency_limit_rejects_over_cap() {
    let params = LifecycleParams {
        limits: LimitsConfig {
            max_concurrent: 2,
            rpc_timeout: Duration::from_secs(60),
            max_request_bytes: 64 * 1024 * 1024,
        },
        initial_handler_delay: Duration::from_millis(500),
    };
    let server = LifecycleServer::start(SECRET, params).await;

    let channel = connect(&server.uri()).await;
    let client = RagEngineClient::new(channel);

    // Coordinate so all three calls enter the server at effectively the
    // same instant. Without the barrier, a fast enough machine could
    // complete the first call before the third arrives.
    let barrier = Arc::new(Barrier::new(3));

    let mut handles = Vec::new();
    for _ in 0..3 {
        let mut c = client.clone();
        let b = barrier.clone();
        handles.push(tokio::spawn(async move {
            b.wait().await;
            c.search(search_req()).await
        }));
    }

    let results = futures::future::join_all(handles).await;
    let codes: Vec<_> = results
        .into_iter()
        .map(|join| join.expect("task panic").map(|_| ()))
        .map(|r| r.map_err(|s| s.code()))
        .collect();

    let ok = codes.iter().filter(|r| r.is_ok()).count();
    let exhausted = codes
        .iter()
        .filter(|r| matches!(r, Err(tonic::Code::ResourceExhausted)))
        .count();

    // Two slots are available; two calls succeed. The third must land
    // on RESOURCE_EXHAUSTED. We don't assert *which* call got rejected
    // because the barrier releases them effectively simultaneously and
    // tokio's scheduler chooses.
    assert_eq!(ok, 2, "expected 2 successful calls, got codes: {codes:?}");
    assert_eq!(
        exhausted, 1,
        "expected 1 RESOURCE_EXHAUSTED, got codes: {codes:?}"
    );
}

// ---------------------------------------------------------------------------
// 2. Timeout layer
// ---------------------------------------------------------------------------

/// Business behavior: an RPC that runs longer than the server-side
/// timeout returns `DEADLINE_EXCEEDED`.
#[tokio::test]
async fn test_timeout_layer_enforces_per_rpc_timeout() {
    let params = LifecycleParams {
        limits: LimitsConfig {
            max_concurrent: 512,
            rpc_timeout: Duration::from_millis(100),
            max_request_bytes: 64 * 1024 * 1024,
        },
        initial_handler_delay: Duration::from_secs(1),
    };
    let server = LifecycleServer::start(SECRET, params).await;

    let channel = connect(&server.uri()).await;
    let mut client = RagEngineClient::new(channel);

    let err = client
        .search(search_req())
        .await
        .expect_err("handler exceeds timeout; should be DEADLINE_EXCEEDED");

    assert_eq!(err.code(), tonic::Code::DeadlineExceeded);
}

// ---------------------------------------------------------------------------
// 3. Body-size limit
// ---------------------------------------------------------------------------

/// Business behavior: a request whose encoded body exceeds the
/// configured limit is rejected with `RESOURCE_EXHAUSTED` before the
/// handler ever runs.
#[tokio::test]
async fn test_body_size_limit_rejects_large_request() {
    let params = LifecycleParams {
        limits: LimitsConfig {
            max_concurrent: 512,
            rpc_timeout: Duration::from_secs(30),
            // Tight ceiling — big enough to accept our small
            // "control" request, small enough to reject the "big"
            // request below.
            max_request_bytes: 4 * 1024,
        },
        initial_handler_delay: Duration::from_millis(0),
    };
    let server = LifecycleServer::start(SECRET, params).await;

    let channel = connect(&server.uri()).await;
    let mut client = RagEngineClient::new(channel);

    // Control: a small request passes through.
    let mut small = tonic::Request::new(IngestBatchRequest {
        dataset_name: "rag_chunks__fake__128".into(),
        org_id: "org-lifecycle".into(),
        mode: 0,
        idempotency_key: "idem-small".into(),
        declared_vector_dim: 128,
        documents: vec![],
    });
    with_bearer(&mut small, SECRET);
    client
        .ingest_batch(small)
        .await
        .expect("small body must succeed");

    // Over-limit: pack a single document with a 16 KiB payload. Easily
    // over the 4 KiB limit we configured.
    let big_text = "x".repeat(16 * 1024);
    let big_doc = DocumentToIngest {
        doc_id: "doc-1".into(),
        semantic_id: "semantic".into(),
        link: "https://example.invalid/doc-1".into(),
        doc_updated_at: None,
        acl: vec![],
        is_public: false,
        sections: vec![Section {
            text: big_text,
            link: String::new(),
            title: String::new(),
        }],
        metadata: Default::default(),
        primary_owners: vec![],
        secondary_owners: vec![],
    };
    let mut big = tonic::Request::new(IngestBatchRequest {
        dataset_name: "rag_chunks__fake__128".into(),
        org_id: "org-lifecycle".into(),
        mode: 0,
        idempotency_key: "idem-big".into(),
        declared_vector_dim: 128,
        documents: vec![big_doc],
    });
    with_bearer(&mut big, SECRET);

    let err = client
        .ingest_batch(big)
        .await
        .expect_err("oversize body must be rejected");
    // Tonic 0.12 maps "decoded message too large" to `OUT_OF_RANGE`
    // (see `tonic::codec::decode::Status::out_of_range`). Our own
    // `body_size_limit_layer` returns `RESOURCE_EXHAUSTED` based on
    // `content-length`; but gRPC clients don't always set that header
    // over HTTP/2 (the Rust `tonic` client notably does not), so the
    // actual rejection observed in practice is whichever layer trips
    // first. Accept either — both are "server refused oversized
    // payload" with the same operational meaning.
    assert!(
        matches!(
            err.code(),
            tonic::Code::ResourceExhausted | tonic::Code::OutOfRange
        ),
        "oversize body must be rejected with RESOURCE_EXHAUSTED or OUT_OF_RANGE, got {:?}",
        err.code()
    );
}

// ---------------------------------------------------------------------------
// 4. Graceful shutdown drains in-flight
// ---------------------------------------------------------------------------

/// Business behavior: an RPC that's already been admitted when the
/// shutdown signal fires runs to completion; the client receives a
/// normal OK response.
#[tokio::test]
async fn test_graceful_shutdown_drains_inflight() {
    let params = LifecycleParams {
        limits: LimitsConfig::default(),
        initial_handler_delay: Duration::from_millis(500),
    };
    let server = LifecycleServer::start(SECRET, params).await;

    let uri = server.uri();
    let channel = connect(&uri).await;
    let mut client = RagEngineClient::new(channel);

    // Issue a long-running RPC on a tokio task so we can trigger
    // shutdown independently.
    let rpc_task = tokio::spawn(async move { client.search(search_req()).await });

    // Give the RPC time to arrive at the server *and* enter the
    // handler's delay. 50 ms is generous on loopback.
    tokio::time::sleep(Duration::from_millis(100)).await;

    let deadline = Duration::from_secs(5);
    let shutdown = server.shutdown_with_deadline(deadline).await;
    shutdown
        .expect("server must drain within deadline")
        .expect("tonic transport must not error on drain");

    // The in-flight RPC must have completed normally — NOT aborted.
    let resp = rpc_task
        .await
        .expect("rpc task panic")
        .expect("in-flight RPC must complete successfully during drain");
    // Response body is the default `SearchResponse`; the only thing we
    // check is that the server returned it rather than bailing.
    let _ = resp.into_inner();
}

// ---------------------------------------------------------------------------
// 5. Graceful shutdown respects drain deadline
// ---------------------------------------------------------------------------

/// Business behavior: if a handler runs past the drain deadline, the
/// shutdown path forcibly terminates the server rather than hanging
/// forever. The existence of a deadline is what makes
/// `terminationGracePeriodSeconds` safe to set in Kubernetes — the pod
/// IS going to exit, even if a wedged handler would otherwise pin it.
#[tokio::test]
async fn test_graceful_shutdown_respects_deadline() {
    let params = LifecycleParams {
        limits: LimitsConfig {
            max_concurrent: 512,
            // Large per-RPC timeout so we're measuring drain enforcement,
            // not per-RPC enforcement.
            rpc_timeout: Duration::from_secs(60),
            max_request_bytes: 64 * 1024 * 1024,
        },
        // Handler intentionally outlasts the drain deadline we pick below.
        initial_handler_delay: Duration::from_secs(5),
    };
    let server = LifecycleServer::start(SECRET, params).await;

    let uri = server.uri();
    let channel = connect(&uri).await;
    let mut client = RagEngineClient::new(channel);

    let _rpc_task = tokio::spawn(async move {
        // Best-effort; we only care that it's in flight.
        let _ = client.search(search_req()).await;
    });

    tokio::time::sleep(Duration::from_millis(100)).await;

    // Short drain deadline, shorter than the 5s handler.
    let deadline = Duration::from_millis(300);
    let outcome = server.shutdown_with_deadline(deadline).await;

    // The shutdown path returns `Err(Elapsed)` when the drain deadline
    // was exceeded. This is the forced-abort signal main.rs maps to
    // exit code 1.
    assert!(
        outcome.is_err(),
        "expected drain deadline to be exceeded, got: {outcome:?}"
    );
}

// ---------------------------------------------------------------------------
// 6. Panic handler returns INTERNAL, server stays alive
// ---------------------------------------------------------------------------

/// Business behavior: a panic inside a handler (a) does not kill the
/// process, (b) returns a gRPC `INTERNAL` status to the caller, and
/// (c) leaves the server able to serve subsequent RPCs from other
/// callers.
#[tokio::test]
async fn test_panic_handler_returns_internal_and_server_stays_alive() {
    // We need `install_panic_handler` to have run so the counter and
    // hook are active. The hook is global per-process; we re-install
    // it here with this test's isolated registry.
    let params = LifecycleParams::default();
    let server = LifecycleServer::start(SECRET, params).await;
    rag_engine_server::install_panic_handler(&server.metrics);

    let before = panic_count_from_metrics(&server.metrics);

    // Arm the next call to panic.
    server.service.arm_panic();

    let channel = connect(&server.uri()).await;
    let mut client = RagEngineClient::new(channel);

    let err = client
        .search(search_req())
        .await
        .expect_err("panicking handler must surface an error to the client");
    assert_eq!(
        err.code(),
        tonic::Code::Internal,
        "panic must become INTERNAL, got: {err:?}"
    );

    // Server must still be alive — issue a second call on the same
    // channel, expect OK.
    client
        .search(search_req())
        .await
        .expect("server must still serve after a handler panic");

    // Counter must have incremented at least once.
    let after = panic_count_from_metrics(&server.metrics);
    assert!(
        after > before,
        "rag_engine_panics_total must increment (before={before}, after={after})"
    );
}

// ---------------------------------------------------------------------------
// 7. Panic metric is observable via the metrics registry
// ---------------------------------------------------------------------------

/// Business behavior: the panic counter is registered on the same
/// Prometheus registry the `/metrics` HTTP endpoint scrapes, so an
/// operator watching a dashboard sees the count change after a panic.
///
/// (We scrape the registry directly rather than spinning up an HTTP
/// server because the HTTP plumbing is exercised exhaustively in
/// `tests/metrics.rs`; duplicating that here would not add business
/// value.)
#[tokio::test]
async fn test_panic_handler_increments_metric() {
    let params = LifecycleParams::default();
    let server = LifecycleServer::start(SECRET, params).await;
    rag_engine_server::install_panic_handler(&server.metrics);

    let before = panic_count_from_metrics(&server.metrics);
    server.service.arm_panic();

    let channel = connect(&server.uri()).await;
    let mut client = RagEngineClient::new(channel);
    let _ = client.search(search_req()).await;

    // Give the hook a moment to run on a potentially different thread.
    tokio::time::sleep(Duration::from_millis(50)).await;

    let after = panic_count_from_metrics(&server.metrics);
    assert!(
        after > before,
        "panic counter must advance after a handler panic (before={before}, after={after})"
    );
}

// ---------------------------------------------------------------------------
// 8. Concurrency-layer rejections are recorded by the metrics layer
// ---------------------------------------------------------------------------

/// Business behavior: when the concurrency layer rejects a request,
/// the metrics layer still records a completed RPC observation with
/// the appropriate `RESOURCE_EXHAUSTED` code label. Operators watching
/// the error-rate dashboard see "rejected" count climb, not silence.
#[tokio::test]
async fn test_rejected_requests_are_counted_by_metrics_layer() {
    let params = LifecycleParams {
        limits: LimitsConfig {
            max_concurrent: 1,
            rpc_timeout: Duration::from_secs(60),
            max_request_bytes: 64 * 1024 * 1024,
        },
        initial_handler_delay: Duration::from_millis(300),
    };
    let server = LifecycleServer::start(SECRET, params).await;

    let channel = connect(&server.uri()).await;
    let client = RagEngineClient::new(channel);

    let barrier = Arc::new(Barrier::new(2));
    let mut handles = Vec::new();
    for _ in 0..2 {
        let mut c = client.clone();
        let b = barrier.clone();
        handles.push(tokio::spawn(async move {
            b.wait().await;
            c.search(search_req()).await
        }));
    }
    let results: Vec<_> = futures::future::join_all(handles).await;
    let exhausted = results
        .iter()
        .filter(|r| {
            matches!(
                r.as_ref().expect("join"),
                Err(s) if s.code() == tonic::Code::ResourceExhausted
            )
        })
        .count();
    assert_eq!(exhausted, 1, "one of the two calls must be rejected");

    // Scrape the metrics and confirm the rejected request is in the
    // rpc_total counter under the RESOURCE_EXHAUSTED label.
    let (text, _ct) = server.metrics.encode_text();
    assert!(
        text.contains("rag_engine_rpc_total"),
        "rpc_total family must be present in scrape output"
    );
    assert!(
        text.lines().any(|l| {
            l.starts_with("rag_engine_rpc_total{")
                && l.contains("method=\"Search\"")
                && l.contains("code=\"RESOURCE_EXHAUSTED\"")
        }),
        "scrape must contain a Search+RESOURCE_EXHAUSTED row; got:\n{text}"
    );
}

// ---------------------------------------------------------------------------
// Local helpers
// ---------------------------------------------------------------------------

/// Extract the current value of `rag_engine_panics_total` from the
/// supplied metrics registry. Returns 0 if the counter is not yet
/// registered — which is exactly the state before
/// `install_panic_handler(&metrics)` has run.
fn panic_count_from_metrics(metrics: &rag_engine_server::Metrics) -> u64 {
    let families = metrics.registry().gather();
    for fam in families {
        if fam.get_name() == "rag_engine_panics_total" {
            if let Some(m) = fam.get_metric().first() {
                return m.get_counter().get_value() as u64;
            }
        }
    }
    0
}

// Silence a purely-mechanical unused-import lint on `TestRagService`:
// it's referenced via `LifecycleServer::service`, but some test
// configurations may not directly construct one.
#[allow(dead_code)]
fn _ignored_import_lint() -> Option<TestRagService> {
    None
}
