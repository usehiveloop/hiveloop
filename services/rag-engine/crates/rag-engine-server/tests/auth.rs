//! Integration test: shared-secret auth interceptor end-to-end.
//!
//! Business value: verifies the security boundary between Go and Rust.
//! If Go's Hiveloop backend sends no token — or the wrong token — the
//! Rust engine rejects. If Go sends the right token, the request
//! passes through to the service (and hits the 2A `UNIMPLEMENTED`
//! stub). Also pins the "same-length wrong secret" code path so we
//! catch regressions in the constant-time comparison branch.

mod common;

use common::{with_bearer, TestServer};
use rag_engine_proto::rag_engine_client::RagEngineClient;
use rag_engine_proto::IngestBatchRequest;
use tonic::transport::Endpoint;

const SECRET: &str = "test-secret-value";

fn sample_request() -> IngestBatchRequest {
    IngestBatchRequest {
        dataset_name: "rag_chunks__fake__128".into(),
        org_id: "org-abc".into(),
        mode: 0,
        idempotency_key: "idem-auth".into(),
        declared_vector_dim: 128,
        documents: vec![],
    }
}

async fn connect(server: &TestServer) -> RagEngineClient<tonic::transport::Channel> {
    let channel = Endpoint::from_shared(server.uri())
        .expect("endpoint")
        .connect()
        .await
        .expect("connect");
    RagEngineClient::new(channel)
}

#[tokio::test]
async fn missing_authorization_header_is_unauthenticated() {
    let server = TestServer::start(SECRET).await;
    let mut client = connect(&server).await;

    let req = tonic::Request::new(sample_request()); // no bearer attached
    let err = client
        .ingest_batch(req)
        .await
        .expect_err("must reject without auth");

    assert_eq!(err.code(), tonic::Code::Unauthenticated);
}

#[tokio::test]
async fn correct_bearer_passes_auth_and_hits_unimplemented_stub() {
    let server = TestServer::start(SECRET).await;
    let mut client = connect(&server).await;

    let mut req = tonic::Request::new(sample_request());
    with_bearer(&mut req, SECRET);

    let err = client
        .ingest_batch(req)
        .await
        .expect_err("auth passes, stub says UNIMPLEMENTED");

    assert_eq!(err.code(), tonic::Code::Unimplemented);
}

#[tokio::test]
async fn wrong_bearer_same_length_is_unauthenticated() {
    // Same-length wrong value: exercises the constant-time compare
    // branch in `SharedSecretAuth::check`, not the length-mismatch
    // short-circuit. `"test-secret-value"` is 17 bytes — the attack
    // below is also 17 bytes.
    let server = TestServer::start(SECRET).await;
    let mut client = connect(&server).await;

    let mut req = tonic::Request::new(sample_request());
    with_bearer(&mut req, "wrong-secret-xyzz");

    let err = client
        .ingest_batch(req)
        .await
        .expect_err("same-length wrong bearer must be rejected");
    assert_eq!(err.code(), tonic::Code::Unauthenticated);
}

#[tokio::test]
async fn wrong_bearer_different_length_is_unauthenticated() {
    // Different-length wrong value: exercises the length-mismatch
    // branch — still constant-time via a sink buffer of equal length.
    let server = TestServer::start(SECRET).await;
    let mut client = connect(&server).await;

    let mut req = tonic::Request::new(sample_request());
    with_bearer(&mut req, "short");

    let err = client
        .ingest_batch(req)
        .await
        .expect_err("different-length wrong bearer must be rejected");
    assert_eq!(err.code(), tonic::Code::Unauthenticated);
}

#[tokio::test]
async fn non_bearer_authorization_value_is_unauthenticated() {
    let server = TestServer::start(SECRET).await;
    let mut client = connect(&server).await;

    // Attach raw metadata without the `Bearer ` prefix.
    let mut req = tonic::Request::new(sample_request());
    let val: tonic::metadata::MetadataValue<_> =
        format!("Basic {SECRET}").parse().expect("valid metadata");
    req.metadata_mut().insert("authorization", val);

    let err = client
        .ingest_batch(req)
        .await
        .expect_err("non-Bearer scheme must be rejected");
    assert_eq!(err.code(), tonic::Code::Unauthenticated);
}
