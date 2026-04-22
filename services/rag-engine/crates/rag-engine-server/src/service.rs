//! `RagEngineService` — the gRPC service implementation.
//!
//! In Tranche 2A every business RPC returns
//! `Status::unimplemented("not yet implemented")`. Downstream tranches
//! replace these stubs one method at a time:
//!   * 2B → `CreateDataset`, `DropDataset` (+ storage primitives for the rest)
//!   * 2C/2D/2E → `IngestBatch` (depends on embedder, reranker, chunker)
//!   * 2F → `Search`, `UpdateACL`, `DeleteByDocID`, `DeleteByOrg`, `Prune`
//!
//! The health check is served by `tonic_health::server::health_reporter`
//! registered in `main.rs`, not by this struct.

use rag_engine_proto::rag_engine_server::RagEngine;
use rag_engine_proto::{
    CreateDatasetRequest, CreateDatasetResponse, DeleteByDocIdRequest, DeleteByDocIdResponse,
    DeleteByOrgRequest, DeleteByOrgResponse, DropDatasetRequest, DropDatasetResponse,
    IngestBatchRequest, IngestBatchResponse, PruneRequest, PruneResponse, SearchRequest,
    SearchResponse, UpdateAclRequest, UpdateAclResponse,
};
use tonic::{Request, Response, Status};

const UNIMPL: &str = "not yet implemented";

/// The Tranche-2A stub implementation of the `RagEngine` gRPC service.
/// Every RPC returns `UNIMPLEMENTED`; the struct holds no state.
#[derive(Debug, Default, Clone)]
pub struct RagEngineService;

impl RagEngineService {
    /// Construct the stub. Kept as a named constructor so tests and
    /// `main.rs` share the same factory and downstream tranches can add
    /// fields here without touching every call site.
    pub fn new() -> Self {
        Self
    }
}

#[tonic::async_trait]
impl RagEngine for RagEngineService {
    async fn create_dataset(
        &self,
        _request: Request<CreateDatasetRequest>,
    ) -> Result<Response<CreateDatasetResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn drop_dataset(
        &self,
        _request: Request<DropDatasetRequest>,
    ) -> Result<Response<DropDatasetResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn ingest_batch(
        &self,
        _request: Request<IngestBatchRequest>,
    ) -> Result<Response<IngestBatchResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn update_acl(
        &self,
        _request: Request<UpdateAclRequest>,
    ) -> Result<Response<UpdateAclResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn search(
        &self,
        _request: Request<SearchRequest>,
    ) -> Result<Response<SearchResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn delete_by_doc_id(
        &self,
        _request: Request<DeleteByDocIdRequest>,
    ) -> Result<Response<DeleteByDocIdResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn delete_by_org(
        &self,
        _request: Request<DeleteByOrgRequest>,
    ) -> Result<Response<DeleteByOrgResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }

    async fn prune(
        &self,
        _request: Request<PruneRequest>,
    ) -> Result<Response<PruneResponse>, Status> {
        Err(Status::unimplemented(UNIMPL))
    }
}
