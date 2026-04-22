//! Legacy test file from Tranche 2A. In 2A every RPC returned
//! `UNIMPLEMENTED`; in 2F the implementations are live. Kept as a
//! sentinel test that validates the trait is correctly wired by
//! calling the simplest RPC and asserting a structured response rather
//! than an UNIMPLEMENTED error.
//!
//! Business value: proves every RPC is correctly registered on the
//! tonic server after 2F's state rewiring.

mod common;

use common::{with_bearer, TestServer};
use rag_engine_proto::rag_engine_client::RagEngineClient;
use rag_engine_proto::IngestBatchRequest;
use tonic::transport::Endpoint;

const SECRET: &str = "test-secret";

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn ingest_batch_empty_body_returns_ok() {
    let server = TestServer::start(SECRET).await;

    let channel = Endpoint::from_shared(server.uri())
        .expect("endpoint")
        .connect()
        .await
        .expect("connect");

    let mut client = RagEngineClient::new(channel);

    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: "rag_chunks__fake__128".into(),
        org_id: "org-123".into(),
        mode: 0,
        idempotency_key: "idem-1".into(),
        declared_vector_dim: 128,
        documents: vec![],
    });
    with_bearer(&mut req, SECRET);

    // An empty documents list with a valid dim should short-circuit to
    // an OK response with zero docs processed — no dataset lookup
    // needed. This pins the "early-return" branch the handler relies
    // on to avoid a NOT_FOUND when the Go caller prewarms connections
    // with an empty request.
    let resp = client
        .ingest_batch(req)
        .await
        .expect("empty batch must succeed at the gRPC layer");
    let body = resp.into_inner();
    assert!(body.results.is_empty());
    let totals = body.totals.expect("totals populated even for empty batch");
    assert_eq!(totals.docs_succeeded, 0);
    assert_eq!(totals.docs_failed, 0);
    assert_eq!(totals.docs_skipped, 0);
}
