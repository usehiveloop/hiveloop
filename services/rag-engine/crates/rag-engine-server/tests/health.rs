//! Integration test: health check round-trips.
//!
//! Business value: proves the gRPC server boots, is reachable on its
//! bound port, and reports SERVING for the `hiveloop.rag.v1.RagEngine`
//! logical service. If this breaks, nothing else in Phase 2 builds.

mod common;

use common::TestServer;
use tonic::transport::Endpoint;
use tonic_health::pb::health_check_response::ServingStatus;
use tonic_health::pb::health_client::HealthClient;
use tonic_health::pb::HealthCheckRequest;

#[tokio::test]
async fn grpc_server_reports_serving_for_rag_engine_service() {
    let server = TestServer::start("test-secret").await;

    let channel = Endpoint::from_shared(server.uri())
        .expect("endpoint")
        .connect()
        .await
        .expect("connect to test server");

    let mut client = HealthClient::new(channel);

    let resp = client
        .check(HealthCheckRequest {
            service: "hiveloop.rag.v1.RagEngine".to_string(),
        })
        .await
        .expect("health check rpc");

    assert_eq!(resp.into_inner().status, ServingStatus::Serving as i32);
}
