//! Integration test: every business RPC returns `UNIMPLEMENTED` in 2A.
//!
//! Business value: proves the service trait is correctly wired for ALL
//! RPCs, so downstream tranches can override one method at a time
//! (via the generated `RagEngine` trait) and be confident the surface
//! is otherwise live.

mod common;

use common::{with_bearer, TestServer};
use rag_engine_proto::rag_engine_client::RagEngineClient;
use rag_engine_proto::IngestBatchRequest;
use tonic::transport::Endpoint;

const SECRET: &str = "test-secret";

#[tokio::test]
async fn ingest_batch_returns_unimplemented_with_message() {
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

    let err = client
        .ingest_batch(req)
        .await
        .expect_err("stub must reject with UNIMPLEMENTED");

    assert_eq!(err.code(), tonic::Code::Unimplemented);
    assert!(
        err.message().contains("not yet implemented"),
        "expected 'not yet implemented' in message, got {:?}",
        err.message()
    );
}
