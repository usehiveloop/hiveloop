//! `RagEngineService` — the gRPC service implementation (Tranche 2F).
//!
//! This module replaces every `Status::unimplemented` stub from Tranche
//! 2A with a real handler composed of:
//!   * domain-layer calls into `rag-engine-{lance,embed,rerank,chunker}`,
//!   * per-doc error isolation on the ingest hot path,
//!   * idempotency cache look-up + replay,
//!   * metric increments and tracing spans,
//!   * error mapping via `crate::error`.
//!
//! Production behaviour is defined inside `handle_*` free functions so
//! they can be unit-tested without a running tonic server; the `impl
//! RagEngine` wrappers are thin adapters that unpack the `Request`, call
//! the free function, and repack into a `Response`.

use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::{Duration, Instant};

use chrono::{DateTime, TimeZone, Utc};
use rag_engine_chunker::{Document as ChunkerDocument, Section as ChunkerSection};
use rag_engine_embed::{embed_batched, EmbedKind};
use rag_engine_lance::{
    chunk::ChunkRow,
    dataset::DatasetHandle,
    delete,
    ingest::{self, IngestionMode as LanceIngestionMode, MAX_CHUNKS_PER_CALL},
    search::{self as lance_search, SearchParams as LanceSearchParams},
    update::{update_acl as lance_update_acl, UpdateAclEntry as LanceUpdateAclEntry},
};
use rag_engine_proto::rag_engine_server::RagEngine;
use rag_engine_proto::{
    BatchTotals, CreateDatasetRequest, CreateDatasetResponse, DeleteByDocIdRequest,
    DeleteByDocIdResponse, DeleteByOrgRequest, DeleteByOrgResponse, DocumentResult, DocumentStatus,
    DropDatasetRequest, DropDatasetResponse, IngestBatchRequest, IngestBatchResponse,
    IngestionMode as ProtoIngestionMode, PruneRequest, PruneResponse, SearchHit as ProtoSearchHit,
    SearchMode as ProtoSearchMode, SearchRequest, SearchResponse, UpdateAclRequest,
    UpdateAclResponse,
};
use tonic::{Code, Request, Response, Status};
use tracing::{debug, info, warn};

use crate::error::{
    classify_embed_error, embed_to_status, lance_to_status, rerank_to_status, DocErrorCode,
};
use crate::idempotency::IdempotencyCache;
use crate::state::AppState;

/// Concrete service type. Wraps an `Arc<AppState>` so every handler sees
/// the same domain instances without cloning them into the service.
#[derive(Clone)]
pub struct RagEngineService {
    state: Arc<AppState>,
}

impl RagEngineService {
    /// Construct with already-built application state.
    pub fn new(state: Arc<AppState>) -> Self {
        Self { state }
    }

    /// Borrow the underlying app state. Used by tests that want to peek
    /// into the idempotency cache or the underlying store.
    pub fn state(&self) -> &Arc<AppState> {
        &self.state
    }
}

impl std::fmt::Debug for RagEngineService {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("RagEngineService")
            .field("state", &self.state)
            .finish()
    }
}

// ---------------------------------------------------------------------------
// RagEngine trait impl — thin adapters around handle_* functions.
// ---------------------------------------------------------------------------

#[tonic::async_trait]
impl RagEngine for RagEngineService {
    async fn create_dataset(
        &self,
        request: Request<CreateDatasetRequest>,
    ) -> Result<Response<CreateDatasetResponse>, Status> {
        handle_create_dataset(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn drop_dataset(
        &self,
        request: Request<DropDatasetRequest>,
    ) -> Result<Response<DropDatasetResponse>, Status> {
        handle_drop_dataset(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn ingest_batch(
        &self,
        request: Request<IngestBatchRequest>,
    ) -> Result<Response<IngestBatchResponse>, Status> {
        handle_ingest_batch(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn update_acl(
        &self,
        request: Request<UpdateAclRequest>,
    ) -> Result<Response<UpdateAclResponse>, Status> {
        handle_update_acl(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn search(
        &self,
        request: Request<SearchRequest>,
    ) -> Result<Response<SearchResponse>, Status> {
        handle_search(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn delete_by_doc_id(
        &self,
        request: Request<DeleteByDocIdRequest>,
    ) -> Result<Response<DeleteByDocIdResponse>, Status> {
        handle_delete_by_doc_id(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn delete_by_org(
        &self,
        request: Request<DeleteByOrgRequest>,
    ) -> Result<Response<DeleteByOrgResponse>, Status> {
        handle_delete_by_org(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }

    async fn prune(
        &self,
        request: Request<PruneRequest>,
    ) -> Result<Response<PruneResponse>, Status> {
        handle_prune(&self.state, request.into_inner())
            .await
            .map(Response::new)
    }
}

// ---------------------------------------------------------------------------
// CreateDataset
// ---------------------------------------------------------------------------

async fn handle_create_dataset(
    state: &AppState,
    req: CreateDatasetRequest,
) -> Result<CreateDatasetResponse, Status> {
    validate_nonempty("dataset_name", &req.dataset_name)?;
    if req.vector_dim == 0 {
        return Err(Status::invalid_argument("vector_dim must be > 0"));
    }

    let started = Instant::now();
    let (_handle, created) =
        DatasetHandle::create_or_open(&state.store, &req.dataset_name, req.vector_dim as usize)
            .await
            .map_err(lance_to_status)?;
    state
        .metrics
        .lance_operation_duration_seconds
        .with_label_values(&["create_dataset"])
        .observe(started.elapsed().as_secs_f64());

    info!(
        dataset_name = %req.dataset_name,
        vector_dim = req.vector_dim,
        created,
        "create_dataset"
    );
    Ok(CreateDatasetResponse {
        created,
        schema_ok: true,
    })
}

// ---------------------------------------------------------------------------
// DropDataset
// ---------------------------------------------------------------------------

async fn handle_drop_dataset(
    state: &AppState,
    req: DropDatasetRequest,
) -> Result<DropDatasetResponse, Status> {
    validate_nonempty("dataset_name", &req.dataset_name)?;
    if !req.confirm {
        return Err(Status::failed_precondition(
            "DropDataset requires confirm=true as a safety interlock",
        ));
    }

    let started = Instant::now();
    let dropped = DatasetHandle::drop(&state.store, &req.dataset_name)
        .await
        .map_err(lance_to_status)?;
    state
        .metrics
        .lance_operation_duration_seconds
        .with_label_values(&["drop_dataset"])
        .observe(started.elapsed().as_secs_f64());
    info!(dataset_name = %req.dataset_name, dropped, "drop_dataset");
    Ok(DropDatasetResponse { dropped })
}

// ---------------------------------------------------------------------------
// IngestBatch — the hot path.
// ---------------------------------------------------------------------------

async fn handle_ingest_batch(
    state: &AppState,
    req: IngestBatchRequest,
) -> Result<IngestBatchResponse, Status> {
    let batch_started = Instant::now();
    validate_nonempty("dataset_name", &req.dataset_name)?;
    validate_nonempty("org_id", &req.org_id)?;

    if req.documents.is_empty() {
        return Ok(IngestBatchResponse {
            results: vec![],
            totals: Some(BatchTotals {
                batch_duration_ms: batch_started.elapsed().as_millis() as u32,
                ..Default::default()
            }),
        });
    }

    if req.documents.len() > state.limits.max_docs_per_batch {
        return Err(Status::invalid_argument(format!(
            "batch of {} documents exceeds MAX_DOCS_PER_BATCH = {}",
            req.documents.len(),
            state.limits.max_docs_per_batch
        )));
    }

    let declared_dim = req.declared_vector_dim as usize;
    let embedder_dim = state.embedder.dimension() as usize;
    if declared_dim != embedder_dim {
        return Err(Status::invalid_argument(format!(
            "declared_vector_dim {declared_dim} != server embedder dimension {embedder_dim}"
        )));
    }

    let mode = proto_to_lance_mode(req.mode)?;

    let idem_key = maybe_idem_key(&req.dataset_name, &req.org_id, &req.idempotency_key);
    if let Some(key) = idem_key.as_ref() {
        if let Some(cached) = state.idempotency.ingest.get(key) {
            debug!(%key, "ingest: idempotency hit");
            return Ok(cached);
        }
    }

    let dataset = open_dataset_checked(state, &req.dataset_name, declared_dim).await?;

    // Phase 1: chunk.
    let chunk_started = Instant::now();
    let mut doc_plans: Vec<DocPlan> = Vec::with_capacity(req.documents.len());
    let mut total_chunks: usize = 0;
    for proto_doc in &req.documents {
        let plan = plan_document(state, proto_doc);
        total_chunks += plan.chunks.len();
        doc_plans.push(plan);
    }
    let chunk_duration = chunk_started.elapsed();

    if total_chunks > state.limits.max_chunks_per_batch || total_chunks > MAX_CHUNKS_PER_CALL {
        return Err(Status::invalid_argument(format!(
            "batch produces {total_chunks} chunks, exceeds limit of {}; reduce docs per batch",
            state.limits.max_chunks_per_batch.min(MAX_CHUNKS_PER_CALL)
        )));
    }

    // Phase 2: embed.
    let embed_started = Instant::now();
    let (flat_texts, doc_offsets) = flatten_for_embedding(&doc_plans);
    let embed_duration;
    let vectors = if flat_texts.is_empty() {
        embed_duration = embed_started.elapsed();
        Vec::new()
    } else {
        let model = state.embedder.id().to_string();
        let approx_tokens: u64 = flat_texts.iter().map(|s| s.chars().count() as u64).sum();
        let emb_result = embed_batched(
            state.embedder.clone(),
            flat_texts,
            EmbedKind::Passage,
            32,
            4,
        )
        .await;
        embed_duration = embed_started.elapsed();
        match emb_result {
            Ok(v) => {
                state
                    .metrics
                    .embedding_batch_latency_seconds
                    .with_label_values(&[&model])
                    .observe(embed_duration.as_secs_f64());
                state
                    .metrics
                    .embedding_tokens_total
                    .with_label_values(&[&model])
                    .inc_by(approx_tokens);
                v
            }
            Err(e) => {
                warn!(error = %e, "embedder failed; marking every non-skipped doc as FAILED");
                let code = classify_embed_error(&e);
                let response = build_embedder_failure_response(
                    &req.documents,
                    doc_plans,
                    code,
                    e.to_string(),
                    chunk_duration,
                    embed_duration,
                    Duration::ZERO,
                    batch_started,
                    state,
                );
                if let Some(key) = idem_key {
                    state.idempotency.ingest.put(key, response.clone());
                }
                return Ok(response);
            }
        }
    };

    // Phase 3: assemble rows per doc.
    let mut per_doc_rows: Vec<Vec<ChunkRow>> = Vec::with_capacity(doc_plans.len());
    let mut per_doc_result: Vec<DocumentResult> = Vec::with_capacity(doc_plans.len());
    for (idx, plan) in doc_plans.iter().enumerate() {
        let proto_doc = &req.documents[idx];
        if let Some(skip) = plan.skip_reason {
            per_doc_rows.push(Vec::new());
            per_doc_result.push(DocumentResult {
                doc_id: proto_doc.doc_id.clone(),
                status: DocumentStatus::Skipped as i32,
                chunks_written: 0,
                tokens_embedded: 0,
                error_code: skip.as_proto_str().to_string(),
                error_reason: "document has no indexable content".to_string(),
            });
            continue;
        }

        let start = doc_offsets[idx];
        let end = start + plan.chunks.len();
        let doc_vectors = &vectors[start..end];

        let mut per_doc_error: Option<(DocErrorCode, String)> = None;
        for v in doc_vectors {
            if v.len() != declared_dim {
                per_doc_error = Some((
                    DocErrorCode::EmbeddingDimMismatch,
                    format!(
                        "embedder returned {} floats, expected {declared_dim}",
                        v.len()
                    ),
                ));
                break;
            }
            if v.iter().any(|f| !f.is_finite()) {
                per_doc_error = Some((
                    DocErrorCode::NanVector,
                    "embedder returned non-finite vector".to_string(),
                ));
                break;
            }
        }

        if let Some((code, reason)) = per_doc_error {
            per_doc_rows.push(Vec::new());
            per_doc_result.push(DocumentResult {
                doc_id: proto_doc.doc_id.clone(),
                status: DocumentStatus::Failed as i32,
                chunks_written: 0,
                tokens_embedded: 0,
                error_code: code.as_proto_str().to_string(),
                error_reason: reason,
            });
            continue;
        }

        let rows = assemble_rows(
            &req.org_id,
            proto_doc,
            &plan.chunks,
            doc_vectors,
            plan.metadata.clone(),
        );
        let tokens_embedded: u32 = plan.chunks.iter().map(|c| c.chunk_tok_count).sum();
        per_doc_rows.push(rows);
        per_doc_result.push(DocumentResult {
            doc_id: proto_doc.doc_id.clone(),
            status: DocumentStatus::Success as i32,
            chunks_written: plan.chunks.len() as u32,
            tokens_embedded,
            error_code: String::new(),
            error_reason: String::new(),
        });
    }

    // Phase 4: write.
    let write_started = Instant::now();
    let flat_rows: Vec<ChunkRow> = per_doc_rows.iter().flatten().cloned().collect();

    let write_duration;
    let mut rows_written: u64 = 0;
    if flat_rows.is_empty() {
        write_duration = write_started.elapsed();
    } else {
        match ingest::upsert_chunks(&dataset, &req.org_id, mode, &flat_rows).await {
            Ok(stats) => {
                write_duration = write_started.elapsed();
                rows_written = stats.rows_written + stats.rows_updated;
                state
                    .metrics
                    .lance_operation_duration_seconds
                    .with_label_values(&["upsert_chunks"])
                    .observe(write_duration.as_secs_f64());
            }
            Err(e) => {
                write_duration = write_started.elapsed();
                warn!(error = %e, "lance upsert failed");
                for r in per_doc_result.iter_mut() {
                    if r.status == DocumentStatus::Success as i32 {
                        r.status = DocumentStatus::Failed as i32;
                        r.error_code = DocErrorCode::StorageWriteFailed.as_proto_str().to_string();
                        r.error_reason = e.to_string();
                        r.chunks_written = 0;
                    }
                }
            }
        }
    }

    // Per-doc status counters.
    for r in &per_doc_result {
        let label = status_label(r.status);
        state
            .metrics
            .ingest_docs_total
            .with_label_values(&[label])
            .inc();
    }

    state
        .metrics
        .ingest_batch_size
        .with_label_values(&[])
        .observe(req.documents.len() as f64);

    let (succeeded, failed, skipped) = count_statuses(&per_doc_result);
    let totals = BatchTotals {
        rows_written,
        bytes_written: 0,
        batch_duration_ms: batch_started.elapsed().as_millis() as u32,
        chunk_duration_ms: chunk_duration.as_millis() as u32,
        embedding_duration_ms: embed_duration.as_millis() as u32,
        write_duration_ms: write_duration.as_millis() as u32,
        docs_succeeded: succeeded,
        docs_failed: failed,
        docs_skipped: skipped,
    };
    let response = IngestBatchResponse {
        results: per_doc_result,
        totals: Some(totals),
    };

    if let Some(key) = idem_key {
        state.idempotency.ingest.put(key, response.clone());
    }

    Ok(response)
}

struct DocPlan {
    chunks: Vec<rag_engine_chunker::Chunk>,
    metadata: BTreeMap<String, String>,
    skip_reason: Option<DocErrorCode>,
}

fn plan_document(state: &AppState, proto_doc: &rag_engine_proto::DocumentToIngest) -> DocPlan {
    let has_any_text = proto_doc.sections.iter().any(|s| !s.text.trim().is_empty());
    if proto_doc.sections.is_empty() || !has_any_text {
        return DocPlan {
            chunks: Vec::new(),
            metadata: proto_to_metadata(&proto_doc.metadata),
            skip_reason: Some(DocErrorCode::EmptyContent),
        };
    }

    let doc = ChunkerDocument {
        doc_id: proto_doc.doc_id.clone(),
        title: if proto_doc.semantic_id.is_empty() {
            None
        } else {
            Some(proto_doc.semantic_id.clone())
        },
        sections: proto_doc
            .sections
            .iter()
            .map(|s| ChunkerSection {
                text: s.text.clone(),
                link: if s.link.is_empty() {
                    None
                } else {
                    Some(s.link.clone())
                },
                title: if s.title.is_empty() {
                    None
                } else {
                    Some(s.title.clone())
                },
            })
            .collect(),
        metadata: proto_doc
            .metadata
            .iter()
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect(),
    };
    let chunks = state.chunker.chunk(&doc);
    if chunks.is_empty() {
        return DocPlan {
            chunks: Vec::new(),
            metadata: proto_to_metadata(&proto_doc.metadata),
            skip_reason: Some(DocErrorCode::EmptyContent),
        };
    }
    DocPlan {
        chunks,
        metadata: proto_to_metadata(&proto_doc.metadata),
        skip_reason: None,
    }
}

fn flatten_for_embedding(plans: &[DocPlan]) -> (Vec<String>, Vec<usize>) {
    let mut texts: Vec<String> = Vec::new();
    let mut offsets: Vec<usize> = Vec::with_capacity(plans.len());
    for plan in plans {
        offsets.push(texts.len());
        if plan.skip_reason.is_none() {
            for c in &plan.chunks {
                texts.push(c.content_for_embedding.clone());
            }
        }
    }
    (texts, offsets)
}

fn assemble_rows(
    org_id: &str,
    proto_doc: &rag_engine_proto::DocumentToIngest,
    chunks: &[rag_engine_chunker::Chunk],
    vectors: &[Vec<f32>],
    metadata: BTreeMap<String, String>,
) -> Vec<ChunkRow> {
    let doc_updated = proto_doc
        .doc_updated_at
        .as_ref()
        .and_then(proto_timestamp_ref_to_chrono);
    chunks
        .iter()
        .zip(vectors.iter())
        .map(|(chunk, vec)| {
            let id = format!("{}__{}", proto_doc.doc_id, chunk.chunk_index);
            ChunkRow {
                id,
                org_id: org_id.to_string(),
                doc_id: proto_doc.doc_id.clone(),
                chunk_index: chunk.chunk_index,
                content: chunk.content.clone(),
                title_prefix: if proto_doc.semantic_id.is_empty() {
                    None
                } else {
                    Some(proto_doc.semantic_id.clone())
                },
                blurb: if chunk.blurb.is_empty() {
                    None
                } else {
                    Some(chunk.blurb.clone())
                },
                vector: vec.clone(),
                acl: proto_doc.acl.clone(),
                is_public: proto_doc.is_public,
                doc_updated_at: doc_updated,
                metadata: metadata.clone(),
            }
        })
        .collect()
}

#[allow(clippy::too_many_arguments)]
fn build_embedder_failure_response(
    docs: &[rag_engine_proto::DocumentToIngest],
    plans: Vec<DocPlan>,
    code: DocErrorCode,
    reason: String,
    chunk_duration: Duration,
    embed_duration: Duration,
    write_duration: Duration,
    batch_started: Instant,
    state: &AppState,
) -> IngestBatchResponse {
    let mut results: Vec<DocumentResult> = Vec::with_capacity(docs.len());
    for (i, proto_doc) in docs.iter().enumerate() {
        if let Some(skip) = plans.get(i).and_then(|p| p.skip_reason) {
            results.push(DocumentResult {
                doc_id: proto_doc.doc_id.clone(),
                status: DocumentStatus::Skipped as i32,
                chunks_written: 0,
                tokens_embedded: 0,
                error_code: skip.as_proto_str().to_string(),
                error_reason: "document has no indexable content".to_string(),
            });
        } else {
            results.push(DocumentResult {
                doc_id: proto_doc.doc_id.clone(),
                status: DocumentStatus::Failed as i32,
                chunks_written: 0,
                tokens_embedded: 0,
                error_code: code.as_proto_str().to_string(),
                error_reason: reason.clone(),
            });
        }
    }
    for r in &results {
        let label = status_label(r.status);
        state
            .metrics
            .ingest_docs_total
            .with_label_values(&[label])
            .inc();
    }
    let (succeeded, failed, skipped) = count_statuses(&results);
    IngestBatchResponse {
        results,
        totals: Some(BatchTotals {
            rows_written: 0,
            bytes_written: 0,
            batch_duration_ms: batch_started.elapsed().as_millis() as u32,
            chunk_duration_ms: chunk_duration.as_millis() as u32,
            embedding_duration_ms: embed_duration.as_millis() as u32,
            write_duration_ms: write_duration.as_millis() as u32,
            docs_succeeded: succeeded,
            docs_failed: failed,
            docs_skipped: skipped,
        }),
    }
}

fn count_statuses(results: &[DocumentResult]) -> (u32, u32, u32) {
    let mut succeeded = 0;
    let mut failed = 0;
    let mut skipped = 0;
    for r in results {
        match DocumentStatus::try_from(r.status).unwrap_or(DocumentStatus::Unspecified) {
            DocumentStatus::Success => succeeded += 1,
            DocumentStatus::Failed => failed += 1,
            DocumentStatus::Skipped => skipped += 1,
            DocumentStatus::Unspecified => {}
        }
    }
    (succeeded, failed, skipped)
}

fn status_label(status: i32) -> &'static str {
    match DocumentStatus::try_from(status).unwrap_or(DocumentStatus::Unspecified) {
        DocumentStatus::Success => "success",
        DocumentStatus::Failed => "failed",
        DocumentStatus::Skipped => "skipped",
        DocumentStatus::Unspecified => "unspecified",
    }
}

// ---------------------------------------------------------------------------
// UpdateACL
// ---------------------------------------------------------------------------

async fn handle_update_acl(
    state: &AppState,
    req: UpdateAclRequest,
) -> Result<UpdateAclResponse, Status> {
    validate_nonempty("dataset_name", &req.dataset_name)?;
    validate_nonempty("org_id", &req.org_id)?;
    if req.entries.is_empty() {
        return Ok(UpdateAclResponse {
            docs_updated: 0,
            chunks_updated: 0,
        });
    }
    for (i, e) in req.entries.iter().enumerate() {
        if e.doc_id.is_empty() {
            return Err(Status::invalid_argument(format!(
                "entries[{i}].doc_id must be non-empty"
            )));
        }
    }

    let idem_key = maybe_idem_key(&req.dataset_name, &req.org_id, &req.idempotency_key);
    if let Some(key) = idem_key.as_ref() {
        if let Some(cached) = state.idempotency.update_acl.get(key) {
            debug!(%key, "update_acl: idempotency hit");
            return Ok(cached);
        }
    }

    let dataset = open_dataset_any_dim(state, &req.dataset_name).await?;

    let entries: Vec<LanceUpdateAclEntry> = req
        .entries
        .iter()
        .map(|e| LanceUpdateAclEntry {
            doc_id: e.doc_id.clone(),
            acl: e.acl.clone(),
            is_public: e.is_public,
        })
        .collect();

    let started = Instant::now();
    let stats = lance_update_acl(&dataset, &req.org_id, &entries)
        .await
        .map_err(lance_to_status)?;
    state
        .metrics
        .lance_operation_duration_seconds
        .with_label_values(&["update_acl"])
        .observe(started.elapsed().as_secs_f64());

    let response = UpdateAclResponse {
        docs_updated: stats.docs_updated,
        chunks_updated: stats.chunks_updated,
    };
    if let Some(key) = idem_key {
        state.idempotency.update_acl.put(key, response);
    }
    Ok(UpdateAclResponse {
        docs_updated: stats.docs_updated,
        chunks_updated: stats.chunks_updated,
    })
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

async fn handle_search(state: &AppState, req: SearchRequest) -> Result<SearchResponse, Status> {
    validate_nonempty("dataset_name", &req.dataset_name)?;
    validate_nonempty("org_id", &req.org_id)?;

    if !req.custom_sql_filter.trim().is_empty() {
        return Err(Status::invalid_argument(
            "custom_sql_filter is not supported in Phase 2; server rejects requests that set it",
        ));
    }

    let mode = proto_to_lance_search_mode(req.mode)?;
    let limit = if req.limit == 0 {
        10
    } else {
        req.limit as usize
    };

    let dataset = open_dataset_any_dim(state, &req.dataset_name).await?;

    let needs_vector = matches!(
        mode,
        rag_engine_lance::SearchMode::Hybrid | rag_engine_lance::SearchMode::VectorOnly
    );
    let query_vector = if needs_vector {
        if !req.query_vector.is_empty() {
            Some(req.query_vector.clone())
        } else if req.query_text.trim().is_empty() {
            return Err(Status::invalid_argument(
                "vector/hybrid search requires either query_vector or query_text",
            ));
        } else {
            let v = state
                .embedder
                .embed(vec![req.query_text.clone()], EmbedKind::Query)
                .await
                .map_err(embed_to_status)?;
            Some(
                v.into_iter()
                    .next()
                    .ok_or_else(|| Status::internal("embedder returned empty result for query"))?,
            )
        }
    } else {
        None
    };

    let doc_updated_after = req
        .doc_updated_after
        .as_ref()
        .and_then(proto_timestamp_ref_to_chrono);

    let params = LanceSearchParams {
        org_id: req.org_id.clone(),
        query_text: req.query_text.clone(),
        query_vector,
        mode,
        acl_any_of: req.acl_any_of.clone(),
        include_public: req.include_public,
        limit,
        doc_updated_after,
    };

    let mode_label = lance_search_mode_label(mode);
    let started = Instant::now();
    let hits = lance_search::search(&dataset, &params)
        .await
        .map_err(lance_to_status)?;
    let duration = started.elapsed();
    state
        .metrics
        .lance_operation_duration_seconds
        .with_label_values(&[&format!("search_{mode_label}")])
        .observe(duration.as_secs_f64());

    let after_rerank_count;
    let final_hits: Vec<(lance_search::SearchHit, f64)> = if req.rerank && !hits.is_empty() {
        let query = req.query_text.clone();
        let candidates: Vec<String> = hits.iter().map(|h| h.content.clone()).collect();
        let scores = state
            .reranker
            .rerank(&query, candidates)
            .await
            .map_err(rerank_to_status)?;
        let mut paired: Vec<(lance_search::SearchHit, f32)> =
            hits.into_iter().zip(scores.into_iter()).collect();
        paired.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        paired.truncate(limit);
        after_rerank_count = paired.len() as u32;
        paired
            .into_iter()
            .map(|(mut h, s)| {
                h.score = s as f64;
                (h, s as f64)
            })
            .collect()
    } else {
        after_rerank_count = 0;
        hits.into_iter().map(|h| (h, 0.0f64)).collect()
    };

    let proto_hits: Vec<ProtoSearchHit> = final_hits
        .into_iter()
        .map(|(h, rerank_score)| lance_hit_to_proto(h, rerank_score))
        .collect();

    let after_fusion = proto_hits.len() as u32;
    Ok(SearchResponse {
        hits: proto_hits,
        bm25_candidates: 0,
        vector_candidates: 0,
        after_fusion,
        after_rerank: after_rerank_count,
    })
}

fn lance_hit_to_proto(h: lance_search::SearchHit, rerank_score: f64) -> ProtoSearchHit {
    ProtoSearchHit {
        chunk_id: h.chunk_id,
        doc_id: h.doc_id,
        chunk_index: h.chunk_index,
        score: h.score,
        vector_score: 0.0,
        bm25_score: 0.0,
        rerank_score,
        content: h.content,
        blurb: h.blurb.unwrap_or_default(),
        doc_updated_at: h.doc_updated_at.map(chrono_to_proto_timestamp),
        metadata: h.metadata.into_iter().collect(),
    }
}

fn lance_search_mode_label(m: rag_engine_lance::SearchMode) -> &'static str {
    match m {
        rag_engine_lance::SearchMode::Hybrid => "hybrid",
        rag_engine_lance::SearchMode::VectorOnly => "vector",
        rag_engine_lance::SearchMode::Bm25Only => "bm25",
    }
}

// ---------------------------------------------------------------------------
// DeleteByDocID
// ---------------------------------------------------------------------------

async fn handle_delete_by_doc_id(
    state: &AppState,
    req: DeleteByDocIdRequest,
) -> Result<DeleteByDocIdResponse, Status> {
    validate_nonempty("dataset_name", &req.dataset_name)?;
    validate_nonempty("org_id", &req.org_id)?;
    if req.doc_ids.is_empty() {
        return Ok(DeleteByDocIdResponse {
            chunks_deleted: 0,
            docs_deleted: 0,
        });
    }

    let idem_key = maybe_idem_key(&req.dataset_name, &req.org_id, &req.idempotency_key);
    if let Some(key) = idem_key.as_ref() {
        if let Some(cached) = state.idempotency.delete_by_doc.get(key) {
            return Ok(cached);
        }
    }

    let dataset = open_dataset_any_dim(state, &req.dataset_name).await?;

    let started = Instant::now();
    let chunks_deleted = delete::delete_by_doc_id(&dataset, &req.org_id, &req.doc_ids)
        .await
        .map_err(lance_to_status)?;
    state
        .metrics
        .lance_operation_duration_seconds
        .with_label_values(&["delete_by_doc_id"])
        .observe(started.elapsed().as_secs_f64());

    let response = DeleteByDocIdResponse {
        chunks_deleted,
        docs_deleted: req.doc_ids.len() as u64,
    };
    if let Some(key) = idem_key {
        state.idempotency.delete_by_doc.put(key, response);
    }
    Ok(DeleteByDocIdResponse {
        chunks_deleted,
        docs_deleted: req.doc_ids.len() as u64,
    })
}

// ---------------------------------------------------------------------------
// DeleteByOrg
// ---------------------------------------------------------------------------

async fn handle_delete_by_org(
    state: &AppState,
    req: DeleteByOrgRequest,
) -> Result<DeleteByOrgResponse, Status> {
    validate_nonempty("org_id", &req.org_id)?;
    if !req.confirm {
        return Err(Status::failed_precondition(
            "DeleteByOrg requires confirm=true as a safety interlock",
        ));
    }

    let idem_key = maybe_idem_key("*", &req.org_id, &req.idempotency_key);
    if let Some(key) = idem_key.as_ref() {
        if let Some(cached) = state.idempotency.delete_by_org.get(key) {
            return Ok(cached);
        }
    }

    let mut total: u64 = 0;
    for dataset_name in &req.dataset_names {
        if dataset_name.is_empty() {
            continue;
        }
        let exists = DatasetHandle::exists(&state.store, dataset_name)
            .await
            .map_err(lance_to_status)?;
        if !exists {
            continue;
        }
        let dataset = open_dataset_any_dim(state, dataset_name).await?;
        let started = Instant::now();
        let deleted = delete::delete_by_org(&dataset, &req.org_id)
            .await
            .map_err(lance_to_status)?;
        state
            .metrics
            .lance_operation_duration_seconds
            .with_label_values(&["delete_by_org"])
            .observe(started.elapsed().as_secs_f64());
        total += deleted;
    }

    let response = DeleteByOrgResponse {
        chunks_deleted: total,
    };
    if let Some(key) = idem_key {
        state.idempotency.delete_by_org.put(key, response);
    }
    Ok(DeleteByOrgResponse {
        chunks_deleted: total,
    })
}

// ---------------------------------------------------------------------------
// Prune
// ---------------------------------------------------------------------------

async fn handle_prune(state: &AppState, req: PruneRequest) -> Result<PruneResponse, Status> {
    validate_nonempty("dataset_name", &req.dataset_name)?;
    validate_nonempty("org_id", &req.org_id)?;
    if req.keep_doc_ids.is_empty() {
        return Err(Status::invalid_argument(
            "keep_doc_ids must be non-empty; use DeleteByOrg to wipe a tenant",
        ));
    }

    let idem_key = maybe_idem_key(&req.dataset_name, &req.org_id, &req.idempotency_key);
    if let Some(key) = idem_key.as_ref() {
        if let Some(cached) = state.idempotency.prune.get(key) {
            return Ok(cached);
        }
    }

    let dataset = open_dataset_any_dim(state, &req.dataset_name).await?;

    let started = Instant::now();
    let chunks_pruned = delete::prune(&dataset, &req.org_id, &req.keep_doc_ids)
        .await
        .map_err(lance_to_status)?;
    state
        .metrics
        .lance_operation_duration_seconds
        .with_label_values(&["prune"])
        .observe(started.elapsed().as_secs_f64());

    let response = PruneResponse {
        docs_pruned: 0,
        chunks_pruned,
    };
    if let Some(key) = idem_key {
        state.idempotency.prune.put(key, response);
    }
    Ok(PruneResponse {
        docs_pruned: 0,
        chunks_pruned,
    })
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

fn validate_nonempty(field: &'static str, value: &str) -> Result<(), Status> {
    if value.trim().is_empty() {
        Err(Status::invalid_argument(format!(
            "{field} must be non-empty"
        )))
    } else {
        Ok(())
    }
}

fn maybe_idem_key(dataset: &str, org_id: &str, key: &str) -> Option<String> {
    if key.trim().is_empty() {
        None
    } else {
        Some(IdempotencyCache::<()>::compose_key(dataset, org_id, key))
    }
}

/// Open a dataset whose dim we don't already know. We probe the known
/// Onyx-catalogued dims in descending order; an O(1) metadata open per
/// probe, and the typical deployment uses 2560 (Qwen3-4B) so the first
/// probe wins.
async fn open_dataset_any_dim(
    state: &AppState,
    dataset_name: &str,
) -> Result<DatasetHandle, Status> {
    let exists = DatasetHandle::exists(&state.store, dataset_name)
        .await
        .map_err(lance_to_status)?;
    if !exists {
        return Err(Status::new(
            Code::NotFound,
            format!("dataset `{dataset_name}` does not exist"),
        ));
    }
    // Known embedding dims across the models our registry seeds:
    //   text-embedding-3-large = 3072
    //   text-embedding-3-small = 1536
    //   Qwen3-Embedding-8B     = 4096
    //   Qwen3-Embedding-4B     = 2560
    //   Qwen3-Embedding-0.6B   = 1024
    //   Voyage-3-lite          = 512
    //   plus 768/384/128 for smaller OSS models and test fakes.
    for dim in &[4096usize, 3072, 2560, 1536, 1024, 768, 512, 384, 128] {
        match DatasetHandle::create_or_open(&state.store, dataset_name, *dim).await {
            Ok((h, _)) => return Ok(h),
            Err(rag_engine_lance::LanceStoreError::SchemaMismatch(_)) => continue,
            Err(e) => return Err(lance_to_status(e)),
        }
    }
    Err(Status::failed_precondition(format!(
        "dataset `{dataset_name}` exists but its vector_dim is outside the supported set"
    )))
}

async fn open_dataset_checked(
    state: &AppState,
    dataset_name: &str,
    declared_dim: usize,
) -> Result<DatasetHandle, Status> {
    let exists = DatasetHandle::exists(&state.store, dataset_name)
        .await
        .map_err(lance_to_status)?;
    if !exists {
        return Err(Status::new(
            Code::NotFound,
            format!("dataset `{dataset_name}` does not exist; call CreateDataset first"),
        ));
    }
    match DatasetHandle::create_or_open(&state.store, dataset_name, declared_dim).await {
        Ok((h, _)) => Ok(h),
        Err(rag_engine_lance::LanceStoreError::SchemaMismatch(msg)) => Err(Status::new(
            Code::FailedPrecondition,
            format!("dataset `{dataset_name}` schema mismatch: {msg}"),
        )),
        Err(e) => Err(lance_to_status(e)),
    }
}

fn proto_to_lance_mode(mode: i32) -> Result<LanceIngestionMode, Status> {
    let m = ProtoIngestionMode::try_from(mode).unwrap_or(ProtoIngestionMode::Unspecified);
    match m {
        ProtoIngestionMode::Upsert | ProtoIngestionMode::Unspecified => {
            Ok(LanceIngestionMode::Upsert)
        }
        ProtoIngestionMode::Reindex => Ok(LanceIngestionMode::Reindex),
    }
}

fn proto_to_lance_search_mode(mode: i32) -> Result<rag_engine_lance::SearchMode, Status> {
    let m = ProtoSearchMode::try_from(mode).unwrap_or(ProtoSearchMode::Unspecified);
    match m {
        ProtoSearchMode::Hybrid | ProtoSearchMode::Unspecified => {
            Ok(rag_engine_lance::SearchMode::Hybrid)
        }
        ProtoSearchMode::VectorOnly => Ok(rag_engine_lance::SearchMode::VectorOnly),
        ProtoSearchMode::Bm25Only => Ok(rag_engine_lance::SearchMode::Bm25Only),
    }
}

fn proto_to_metadata(m: &std::collections::HashMap<String, String>) -> BTreeMap<String, String> {
    m.iter().map(|(k, v)| (k.clone(), v.clone())).collect()
}

fn proto_timestamp_ref_to_chrono(ts: &prost_types::Timestamp) -> Option<DateTime<Utc>> {
    Utc.timestamp_opt(ts.seconds, ts.nanos.max(0) as u32)
        .single()
}

fn chrono_to_proto_timestamp(t: DateTime<Utc>) -> prost_types::Timestamp {
    prost_types::Timestamp {
        seconds: t.timestamp(),
        nanos: t.timestamp_subsec_nanos() as i32,
    }
}
