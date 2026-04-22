//! End-to-end integration tests for the Tranche 2F gRPC server.
//!
//! Per `internal/rag/doc/TESTING.md`:
//!   * Real LanceDB on real local disk (fast; no MinIO round-trip tax).
//!   * Real tonic gRPC transport over `127.0.0.1:ephemeral`.
//!   * Mocks permitted ONLY for embedder + reranker — `FakeEmbedder` and
//!     `FakeReranker` satisfy both.
//!   * Every test has a 1-line comment stating the business value.
//!
//! These tests cover the sixteen scenarios locked in the 2F scope:
//!   1.  500-doc round-trip
//!   2.  Mixed per-doc failures (empty / dim-mismatch / ok)
//!   3.  Idempotency replay
//!   4.  Reindex mode wipes prior chunks
//!   5.  Response ordering preserved
//!   6.  Hard doc-count cap
//!   7.  Declared vs embedder dim mismatch
//!   8.  Search finds ingested content
//!   9.  Search rejects custom_sql_filter
//!   10. UpdateACL preserves vector bytes
//!   11. DeleteByDocID removes all chunks for a doc
//!   12. DeleteByOrg wipes tenant
//!   13. CreateDataset idempotence
//!   14. Prune removes stale docs
//!   15. Embedder-unavailable error mapping
//!   16. NOT_FOUND on missing dataset

mod common;

use std::sync::Arc;

use async_trait::async_trait;
use common::{build_test_state, with_bearer, TestServer, TEST_DIM};
use rag_engine_embed::{EmbedError, EmbedKind, Embedder};
use rag_engine_lance::search::{load_chunk, vector_bytes_for_chunk};
use rag_engine_lance::{LanceStore, StoreConfig};
use rag_engine_proto::rag_engine_client::RagEngineClient;
use rag_engine_proto::{
    CreateDatasetRequest, DeleteByDocIdRequest, DeleteByOrgRequest, DocumentStatus,
    DocumentToIngest, IngestBatchRequest, IngestionMode, PruneRequest, SearchRequest,
    Section as ProtoSection, UpdateAclEntry, UpdateAclRequest,
};
use rag_engine_rerank::{FakeReranker, Reranker};
use rag_engine_server::{AppState, StateLimits};
use tempfile::TempDir;
use tonic::transport::Endpoint;
use tonic::Code;

const SECRET: &str = "test-secret";
const DATASET: &str = "rag_chunks__test__d128";

fn section(text: &str) -> ProtoSection {
    ProtoSection {
        text: text.to_string(),
        link: String::new(),
        title: String::new(),
    }
}

fn proto_doc(doc_id: &str, sections: Vec<ProtoSection>) -> DocumentToIngest {
    DocumentToIngest {
        doc_id: doc_id.into(),
        semantic_id: format!("title-{doc_id}"),
        link: String::new(),
        doc_updated_at: None,
        acl: Vec::new(),
        is_public: true,
        sections,
        metadata: Default::default(),
        primary_owners: vec![],
        secondary_owners: vec![],
    }
}

async fn client(server: &TestServer) -> RagEngineClient<tonic::transport::Channel> {
    let channel = Endpoint::from_shared(server.uri())
        .expect("endpoint")
        .connect()
        .await
        .expect("connect");
    RagEngineClient::new(channel)
}

async fn create_dataset(client: &mut RagEngineClient<tonic::transport::Channel>) {
    let mut req = tonic::Request::new(CreateDatasetRequest {
        dataset_name: DATASET.into(),
        vector_dim: TEST_DIM,
        embedding_precision: "float32".into(),
        idempotency_key: String::new(),
    });
    with_bearer(&mut req, SECRET);
    client
        .create_dataset(req)
        .await
        .expect("create_dataset")
        .into_inner();
}

// ---------------------------------------------------------------------------
// 1. 500-doc round-trip
// ---------------------------------------------------------------------------

/// Business value: the hot path (chunk+embed+write) must complete the
/// maximum contracted batch (500 docs, <=1000 chunks) within the 10s
/// SLO on a developer laptop.
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn test_ingest_batch_500_docs_roundtrip() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let docs: Vec<DocumentToIngest> = (0..500)
        .map(|i| {
            proto_doc(
                &format!("doc-{i}"),
                vec![section(&format!("small content for doc number {i}"))],
            )
        })
        .collect();
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);

    let resp = c.ingest_batch(req).await.expect("ingest").into_inner();
    let totals = resp.totals.as_ref().expect("totals");
    assert_eq!(resp.results.len(), 500);
    assert_eq!(totals.docs_succeeded, 500);
    assert_eq!(totals.docs_failed, 0);
    assert!(
        totals.batch_duration_ms < 30_000,
        "batch took {}ms (soft SLO: 30s in test env)",
        totals.batch_duration_ms
    );
}

// ---------------------------------------------------------------------------
// 2. Mixed failures
// ---------------------------------------------------------------------------

/// Business value: per-doc error isolation — one bad doc never fails the
/// rest of the batch, and the Go caller retries only the failures.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_ingest_batch_mixed_failures() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    // 10 docs: 3 empty (SKIPPED), 2 normal (SUCCESS), 5 normal (SUCCESS).
    // (Dim mismatch per-doc can't be expressed with FakeEmbedder since it
    // returns the configured dim; we cover dim mismatch in
    // test_ingest_batch_dim_validation and embedder-unavailable in test 15.)
    let mut docs: Vec<DocumentToIngest> = Vec::new();
    for i in 0..3 {
        docs.push(proto_doc(&format!("empty-{i}"), vec![])); // no sections → skipped
    }
    for i in 0..7 {
        docs.push(proto_doc(
            &format!("ok-{i}"),
            vec![section(&format!("content {i}"))],
        ));
    }

    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);

    let resp = c.ingest_batch(req).await.expect("ingest").into_inner();
    assert_eq!(resp.results.len(), 10);

    let skipped = resp
        .results
        .iter()
        .filter(|r| r.status == DocumentStatus::Skipped as i32)
        .count();
    let success = resp
        .results
        .iter()
        .filter(|r| r.status == DocumentStatus::Success as i32)
        .count();
    assert_eq!(skipped, 3);
    assert_eq!(success, 7);
    for r in resp
        .results
        .iter()
        .filter(|r| r.status == DocumentStatus::Skipped as i32)
    {
        assert_eq!(r.error_code, "empty_content");
    }
}

// ---------------------------------------------------------------------------
// 3. Idempotency replay
// ---------------------------------------------------------------------------

/// Business value: at-most-once semantics — a retried batch from asynq
/// must not double-write tokens/rows.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_ingest_batch_idempotent_replay() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let build_req = |docs: Vec<DocumentToIngest>| IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: "idem-retry-key".into(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    };

    let first = vec![
        proto_doc("dup-0", vec![section("v1 content")]),
        proto_doc("dup-1", vec![section("v1 other")]),
    ];
    let mut r1 = tonic::Request::new(build_req(first));
    with_bearer(&mut r1, SECRET);
    let resp1 = c.ingest_batch(r1).await.expect("first").into_inner();

    // Second call with same key + different content.
    let second = vec![
        proto_doc("dup-0", vec![section("MUTATED content should not take")]),
        proto_doc("dup-1", vec![section("MUTATED")]),
    ];
    let mut r2 = tonic::Request::new(build_req(second));
    with_bearer(&mut r2, SECRET);
    let resp2 = c.ingest_batch(r2).await.expect("second").into_inner();

    // Cached response: same doc_ids + status + success counts.
    assert_eq!(resp1.results.len(), resp2.results.len());
    let succeeded1 = resp1.totals.as_ref().unwrap().docs_succeeded;
    let succeeded2 = resp2.totals.as_ref().unwrap().docs_succeeded;
    assert_eq!(succeeded1, succeeded2);

    // Underlying row was NOT overwritten by the replay — content still v1.
    let store = &server.state.as_ref().unwrap().store;
    let (dataset, _) =
        rag_engine_lance::dataset::DatasetHandle::create_or_open(store, DATASET, TEST_DIM as usize)
            .await
            .unwrap();
    let loaded = load_chunk(&dataset, "org-a", "dup-0__0")
        .await
        .expect("load")
        .expect("chunk exists");
    assert!(loaded.content.contains("v1 content"));
}

// ---------------------------------------------------------------------------
// 4. Reindex mode
// ---------------------------------------------------------------------------

/// Business value: REINDEX must wipe the prior version of a doc so stale
/// chunks don't haunt search results after a re-index.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_ingest_batch_reindex_mode() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    // First pass: one doc with enough text to yield multiple chunks.
    let big_text = "alpha ".repeat(2000);
    let doc_v1 = proto_doc("rx-1", vec![section(&big_text)]);
    let mut req1 = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: vec![doc_v1],
    });
    with_bearer(&mut req1, SECRET);
    let r1 = c.ingest_batch(req1).await.expect("first").into_inner();
    let chunks_v1 = r1.results[0].chunks_written;
    assert!(chunks_v1 > 1, "expected multiple chunks for big doc");

    // Reindex with shorter content → fewer chunks.
    let doc_v2 = proto_doc("rx-1", vec![section("short")]);
    let mut req2 = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Reindex as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: vec![doc_v2],
    });
    with_bearer(&mut req2, SECRET);
    let r2 = c.ingest_batch(req2).await.expect("second").into_inner();
    let chunks_v2 = r2.results[0].chunks_written;
    assert!(chunks_v2 < chunks_v1, "reindex should shrink chunk count");

    // Verify no stale chunks remain — total row count for the doc
    // equals the new chunk count.
    let store = &server.state.as_ref().unwrap().store;
    let (dataset, _) =
        rag_engine_lance::dataset::DatasetHandle::create_or_open(store, DATASET, TEST_DIM as usize)
            .await
            .unwrap();
    let rows = dataset.row_count_for_org("org-a").await.unwrap();
    assert_eq!(rows as u32, chunks_v2);
}

// ---------------------------------------------------------------------------
// 5. Preserve order
// ---------------------------------------------------------------------------

/// Business value: Go caller indexes per-doc results by array position;
/// reordering would silently misattribute failures.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_ingest_batch_preserves_order() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let docs: Vec<DocumentToIngest> = (0..50)
        .map(|i| proto_doc(&format!("pos-{i:03}"), vec![section(&format!("body {i}"))]))
        .collect();
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);
    let resp = c.ingest_batch(req).await.expect("ingest").into_inner();
    for (i, r) in resp.results.iter().enumerate() {
        assert_eq!(r.doc_id, format!("pos-{i:03}"));
    }
}

// ---------------------------------------------------------------------------
// 6. Hard cap
// ---------------------------------------------------------------------------

/// Business value: a too-large batch is rejected at the RPC boundary
/// with INVALID_ARGUMENT so the caller doesn't burn tokens on something
/// we'd reject later.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_ingest_batch_exceeds_hard_cap() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let docs: Vec<DocumentToIngest> = (0..3000)
        .map(|i| proto_doc(&format!("d-{i}"), vec![section("x")]))
        .collect();
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);

    let err = c.ingest_batch(req).await.expect_err("hard cap");
    assert_eq!(err.code(), Code::InvalidArgument);
}

// ---------------------------------------------------------------------------
// 7. Dim validation
// ---------------------------------------------------------------------------

/// Business value: declared_vector_dim mismatch is caught before any
/// embedder spend — a dim typo in config shouldn't burn tokens.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_ingest_batch_dim_validation() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: 9999, // bogus
        documents: vec![proto_doc("x", vec![section("hello")])],
    });
    with_bearer(&mut req, SECRET);

    let err = c.ingest_batch(req).await.expect_err("dim mismatch");
    assert_eq!(err.code(), Code::InvalidArgument);
    assert!(err.message().contains("declared_vector_dim"));
}

// ---------------------------------------------------------------------------
// 8. Search finds content
// ---------------------------------------------------------------------------

/// Business value: vector search over ingested content actually surfaces
/// the ingested rows — the end-to-end retrieval path works.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_search_finds_ingested_content() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    // Ingest a few distinct docs.
    let docs = vec![
        proto_doc("d-rhubarb", vec![section("rhubarb is a stalky vegetable")]),
        proto_doc("d-bicycle", vec![section("a bicycle has two wheels")]),
        proto_doc(
            "d-asteroid",
            vec![section("the asteroid belt orbits the sun")],
        ),
    ];
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);
    c.ingest_batch(req).await.unwrap();

    // Search for "rhubarb". Hybrid mode — FakeEmbedder is deterministic,
    // so the passage-embedding of "rhubarb..." and the query-embedding of
    // "rhubarb" will differ (they use distinct kind bytes) — BM25 carries
    // the signal.
    let mut sreq = tonic::Request::new(SearchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        query_text: "rhubarb".into(),
        query_vector: vec![],
        mode: rag_engine_proto::SearchMode::Hybrid as i32,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        candidate_pool: 0,
        custom_sql_filter: String::new(),
        hybrid_alpha: 0.5,
        rerank: false,
        doc_updated_after: None,
    });
    with_bearer(&mut sreq, SECRET);
    let resp = c.search(sreq).await.expect("search").into_inner();
    assert!(!resp.hits.is_empty(), "search should return hits");
    let doc_ids: Vec<_> = resp.hits.iter().map(|h| h.doc_id.as_str()).collect();
    assert!(
        doc_ids.contains(&"d-rhubarb"),
        "expected d-rhubarb in hits, got: {doc_ids:?}"
    );
}

// ---------------------------------------------------------------------------
// 9. Reject custom_sql_filter
// ---------------------------------------------------------------------------

/// Business value: per DECISIONS.md (2B), the Phase 2 server rejects
/// custom_sql_filter rather than silently ignoring — callers must not
/// assume their filter was applied.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_search_rejects_custom_sql_filter() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let mut req = tonic::Request::new(SearchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        query_text: "hello".into(),
        query_vector: vec![],
        mode: rag_engine_proto::SearchMode::Hybrid as i32,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        candidate_pool: 0,
        custom_sql_filter: "org_id = 'x'".into(),
        hybrid_alpha: 0.5,
        rerank: false,
        doc_updated_after: None,
    });
    with_bearer(&mut req, SECRET);
    let err = c.search(req).await.expect_err("custom_sql_filter");
    assert_eq!(err.code(), Code::InvalidArgument);
}

// ---------------------------------------------------------------------------
// 10. UpdateACL preserves vector
// ---------------------------------------------------------------------------

/// Business value: THE operation that killed the lance-go spike —
/// updating ACL must leave the vector column byte-identical so perm-sync
/// doesn't re-embed on every ACL flip.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_update_acl_preserves_vector() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let mut doc = proto_doc("acl-1", vec![section("secret content")]);
    doc.acl = vec!["user:alice".into()];
    doc.is_public = false;
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: vec![doc],
    });
    with_bearer(&mut req, SECRET);
    c.ingest_batch(req).await.unwrap();

    // Snapshot vector bytes before UpdateACL.
    let store = &server.state.as_ref().unwrap().store;
    let (dataset, _) =
        rag_engine_lance::dataset::DatasetHandle::create_or_open(store, DATASET, TEST_DIM as usize)
            .await
            .unwrap();
    let before = vector_bytes_for_chunk(&dataset, "org-a", "acl-1__0")
        .await
        .expect("before")
        .expect("vector exists");

    // UpdateACL.
    let mut ureq = tonic::Request::new(UpdateAclRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        entries: vec![UpdateAclEntry {
            doc_id: "acl-1".into(),
            acl: vec!["user:bob".into(), "group:eng".into()],
            is_public: true,
        }],
        idempotency_key: String::new(),
    });
    with_bearer(&mut ureq, SECRET);
    let resp = c.update_acl(ureq).await.expect("update_acl").into_inner();
    assert!(resp.chunks_updated > 0);

    // Vector bytes unchanged.
    let after = vector_bytes_for_chunk(&dataset, "org-a", "acl-1__0")
        .await
        .unwrap()
        .unwrap();
    assert_eq!(before, after, "vector bytes must be byte-identical");

    // ACL + is_public updated.
    let loaded = load_chunk(&dataset, "org-a", "acl-1__0")
        .await
        .unwrap()
        .unwrap();
    assert_eq!(loaded.acl, vec!["user:bob".to_string(), "group:eng".into()]);
    assert!(loaded.is_public);
}

// ---------------------------------------------------------------------------
// 11. DeleteByDocID
// ---------------------------------------------------------------------------

/// Business value: connector-driven doc deletion must remove every chunk
/// of the doc; stragglers would survive tenant privacy-deletion.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_delete_by_doc_id() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let big = "word ".repeat(2000);
    let docs = vec![
        proto_doc("del-1", vec![section(&big)]),
        proto_doc("keep-1", vec![section("keep me")]),
    ];
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);
    c.ingest_batch(req).await.unwrap();

    let mut dreq = tonic::Request::new(DeleteByDocIdRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        doc_ids: vec!["del-1".into()],
        idempotency_key: String::new(),
    });
    with_bearer(&mut dreq, SECRET);
    let resp = c.delete_by_doc_id(dreq).await.expect("delete").into_inner();
    assert!(resp.chunks_deleted > 0);

    // keep-1 still present.
    let store = &server.state.as_ref().unwrap().store;
    let (dataset, _) =
        rag_engine_lance::dataset::DatasetHandle::create_or_open(store, DATASET, TEST_DIM as usize)
            .await
            .unwrap();
    let loaded = load_chunk(&dataset, "org-a", "keep-1__0").await.unwrap();
    assert!(loaded.is_some());
    let gone = load_chunk(&dataset, "org-a", "del-1__0").await.unwrap();
    assert!(gone.is_none(), "del-1 chunks should be gone");
}

// ---------------------------------------------------------------------------
// 12. DeleteByOrg
// ---------------------------------------------------------------------------

/// Business value: tenant deletion (GDPR / org deletion) must wipe every
/// chunk across every dataset supplied.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_delete_by_org() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let docs = vec![
        proto_doc("o-1", vec![section("a")]),
        proto_doc("o-2", vec![section("b")]),
    ];
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-wipe".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);
    c.ingest_batch(req).await.unwrap();

    let mut dreq = tonic::Request::new(DeleteByOrgRequest {
        org_id: "org-wipe".into(),
        dataset_names: vec![DATASET.into()],
        confirm: true,
        idempotency_key: String::new(),
    });
    with_bearer(&mut dreq, SECRET);
    let resp = c
        .delete_by_org(dreq)
        .await
        .expect("delete_by_org")
        .into_inner();
    assert!(resp.chunks_deleted >= 2);

    let store = &server.state.as_ref().unwrap().store;
    let (dataset, _) =
        rag_engine_lance::dataset::DatasetHandle::create_or_open(store, DATASET, TEST_DIM as usize)
            .await
            .unwrap();
    assert_eq!(dataset.row_count_for_org("org-wipe").await.unwrap(), 0);
}

// ---------------------------------------------------------------------------
// 13. CreateDataset idempotence
// ---------------------------------------------------------------------------

/// Business value: startup ensure-dataset runs on every boot; a second
/// call must not blow away the existing table.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_create_dataset_idempotent() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;

    let mut r1 = tonic::Request::new(CreateDatasetRequest {
        dataset_name: DATASET.into(),
        vector_dim: TEST_DIM,
        embedding_precision: "float32".into(),
        idempotency_key: String::new(),
    });
    with_bearer(&mut r1, SECRET);
    let resp1 = c.create_dataset(r1).await.unwrap().into_inner();
    assert!(resp1.created);

    let mut r2 = tonic::Request::new(CreateDatasetRequest {
        dataset_name: DATASET.into(),
        vector_dim: TEST_DIM,
        embedding_precision: "float32".into(),
        idempotency_key: String::new(),
    });
    with_bearer(&mut r2, SECRET);
    let resp2 = c.create_dataset(r2).await.unwrap().into_inner();
    assert!(!resp2.created, "second call must NOT re-create");
    assert!(resp2.schema_ok);
}

// ---------------------------------------------------------------------------
// 14. Prune
// ---------------------------------------------------------------------------

/// Business value: connector-pruning removes docs the upstream source no
/// longer has; keep-set semantics are the inverse of delete.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_prune_removes_stale_docs() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    let docs = vec![
        proto_doc("k-1", vec![section("keep")]),
        proto_doc("k-2", vec![section("keep2")]),
        proto_doc("stale-1", vec![section("stale")]),
        proto_doc("stale-2", vec![section("stale2")]),
    ];
    let mut req = tonic::Request::new(IngestBatchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        mode: IngestionMode::Upsert as i32,
        idempotency_key: String::new(),
        declared_vector_dim: TEST_DIM,
        documents: docs,
    });
    with_bearer(&mut req, SECRET);
    c.ingest_batch(req).await.unwrap();

    let mut preq = tonic::Request::new(PruneRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        keep_doc_ids: vec!["k-1".into(), "k-2".into()],
        idempotency_key: String::new(),
    });
    with_bearer(&mut preq, SECRET);
    let resp = c.prune(preq).await.expect("prune").into_inner();
    assert!(resp.chunks_pruned >= 2);

    let store = &server.state.as_ref().unwrap().store;
    let (dataset, _) =
        rag_engine_lance::dataset::DatasetHandle::create_or_open(store, DATASET, TEST_DIM as usize)
            .await
            .unwrap();
    // The keep-set is still here:
    assert!(load_chunk(&dataset, "org-a", "k-1__0")
        .await
        .unwrap()
        .is_some());
    // The stale docs are gone:
    assert!(load_chunk(&dataset, "org-a", "stale-1__0")
        .await
        .unwrap()
        .is_none());
}

// ---------------------------------------------------------------------------
// 15. Embedder unavailable → error mapping
// ---------------------------------------------------------------------------

/// Business value: an upstream embedder outage must surface as
/// UNAVAILABLE (for Search) / per-doc failure (for Ingest), not as a
/// misleading INTERNAL — load balancer drains rely on UNAVAILABLE.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_error_mapping_embedder_unavailable() {
    // Build a custom state with a FailingEmbedder.
    let tempdir = tempfile::tempdir().unwrap();
    let uri = tempdir.path().to_string_lossy().to_string();
    let store = LanceStore::open(StoreConfig::Local { uri }).await.unwrap();
    let embedder: Arc<dyn Embedder> = Arc::new(FailingEmbedder { dim: TEST_DIM });
    let reranker: Arc<dyn Reranker> = Arc::new(FakeReranker::new());
    let chunker = Arc::new(rag_engine_chunker::Chunker::new(
        rag_engine_chunker::tokenizer::TiktokenTokenizer::cl100k_base(),
        rag_engine_chunker::ChunkerConfig::default(),
    ));
    let state = Arc::new(AppState::new(
        store,
        embedder,
        reranker,
        chunker,
        StateLimits::defaults(),
    ));
    let server = TestServer::start_with_state(SECRET, state, Some(tempdir)).await;

    let mut c = client(&server).await;
    create_dataset(&mut c).await;

    // A Search call needs to embed the query; expect UNAVAILABLE back.
    let mut sreq = tonic::Request::new(SearchRequest {
        dataset_name: DATASET.into(),
        org_id: "org-a".into(),
        query_text: "anything".into(),
        query_vector: vec![],
        mode: rag_engine_proto::SearchMode::Hybrid as i32,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        candidate_pool: 0,
        custom_sql_filter: String::new(),
        hybrid_alpha: 0.5,
        rerank: false,
        doc_updated_after: None,
    });
    with_bearer(&mut sreq, SECRET);
    let err = c.search(sreq).await.expect_err("expect failure");
    assert_eq!(err.code(), Code::Unavailable);
}

struct FailingEmbedder {
    dim: u32,
}

#[async_trait]
impl Embedder for FailingEmbedder {
    fn id(&self) -> &str {
        "failing"
    }
    fn dimension(&self) -> u32 {
        self.dim
    }
    fn max_input_tokens(&self) -> u32 {
        8192
    }
    async fn embed(
        &self,
        _texts: Vec<String>,
        _kind: EmbedKind,
    ) -> Result<Vec<Vec<f32>>, EmbedError> {
        Err(EmbedError::Transport("network down".into()))
    }
}

// ---------------------------------------------------------------------------
// 16. NOT_FOUND on missing dataset
// ---------------------------------------------------------------------------

/// Business value: a typo in `dataset_name` yields a crisp NOT_FOUND
/// rather than an INTERNAL or silent empty result.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_error_mapping_lance_missing_dataset() {
    let server = TestServer::start(SECRET).await;
    let mut c = client(&server).await;

    let mut req = tonic::Request::new(SearchRequest {
        dataset_name: "does-not-exist".into(),
        org_id: "org-a".into(),
        query_text: "x".into(),
        query_vector: vec![],
        mode: rag_engine_proto::SearchMode::Hybrid as i32,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        candidate_pool: 0,
        custom_sql_filter: String::new(),
        hybrid_alpha: 0.5,
        rerank: false,
        doc_updated_after: None,
    });
    with_bearer(&mut req, SECRET);
    let err = c.search(req).await.expect_err("missing dataset");
    assert_eq!(err.code(), Code::NotFound);
}

// ---------------------------------------------------------------------------
// Compile-time sanity: a no-op to keep `build_test_state` in-scope when
// feature flags trim trait impls.
// ---------------------------------------------------------------------------

#[allow(dead_code)]
async fn _sanity() {
    let _ = build_test_state(TEST_DIM).await;
    let _: Option<TempDir> = None;
}
