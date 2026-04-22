//! End-to-end tests for the `rag-engine-lance` crate.
//!
//! All tests boot a real MinIO (via testcontainers) and a real LanceDB
//! backed by that MinIO. No mocking.
//!
//! The comment on each test describes the BUSINESS VALUE it secures —
//! per TESTING.md, tests that only verify framework behavior are banned.

mod common;

use std::collections::BTreeMap;
use std::time::Instant;

use anyhow::Result;
use chrono::{TimeZone, Utc};
use rag_engine_lance::chunk::ChunkRow;
use rag_engine_lance::dataset::DatasetHandle;
use rag_engine_lance::delete;
use rag_engine_lance::ingest::{upsert_chunks, IngestionMode};
use rag_engine_lance::search::{
    load_chunk, search, vector_bytes_for_chunk, SearchMode, SearchParams,
};
use rag_engine_lance::update::{update_acl, UpdateAclEntry};

use common::{init_tracing_once, make_vector, minio_harness};

const DIM: usize = 128; // small dim for faster tests; tranche-2F uses 2560.
const DIM_LARGE: usize = 2560;
const DATASET: &str = "rag_chunks__test__d128";

/// Business value: proves schema was accepted by LanceDB and the dataset is
/// readable — this is the foundation for every other operation.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_create_dataset_creates_readable_table() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;

    let (ds, created) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;
    assert!(created, "first call should create the table");
    assert_eq!(ds.vector_dim(), DIM);
    assert_eq!(ds.name(), DATASET);
    assert!(DatasetHandle::exists(&h.store, DATASET).await?);
    Ok(())
}

/// Business value: startup path runs "ensure dataset" on every boot;
/// second call must not blow away the existing table.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_create_dataset_idempotent() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;

    let (ds1, created1) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;
    assert!(created1);
    let row = ChunkRow::new("org-a", "d1", 0, "hello", make_vector(DIM, 1));
    upsert_chunks(&ds1, "org-a", IngestionMode::Upsert, &[row]).await?;

    let (_ds2, created2) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;
    assert!(!created2, "second call should NOT re-create");

    // Row survived the re-open.
    let loaded = load_chunk(&ds1, "org-a", "d1__0").await?;
    assert!(loaded.is_some());
    Ok(())
}

/// Business value: prevents silent mixing of dimensionalities within one
/// dataset (the Phase 0 one-model-per-dataset invariant).
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_create_dataset_rejects_schema_mismatch() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;

    DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;
    let res = DatasetHandle::create_or_open(&h.store, DATASET, 2560).await;
    let err_string = match res {
        Ok(_) => "Ok(<dataset>)".to_string(),
        Err(ref e) => format!("{e}"),
    };
    assert!(
        matches!(
            res,
            Err(rag_engine_lance::LanceStoreError::SchemaMismatch(_))
        ),
        "expected SchemaMismatch error, got: {err_string}"
    );
    Ok(())
}

/// Business value: high-volume ingest writes are the whole reason this
/// service exists; we verify the common large-batch path round-trips
/// with varied ACL shapes (empty, single, multi, high-cardinality).
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_upsert_chunks_writes_and_reads_back() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let rows: Vec<ChunkRow> = (0..1000)
        .map(|i| {
            let mut r = ChunkRow::new(
                "org-a",
                format!("doc-{}", i / 10),
                (i % 10) as u32,
                format!("chunk content number {i}"),
                make_vector(DIM, i as u64),
            );
            // Vary ACL shapes: empty, single, multiple tokens.
            r.acl = match i % 4 {
                0 => vec![],
                1 => vec!["user:alice".into()],
                2 => vec!["user:alice".into(), "group:eng".into()],
                _ => vec![
                    "user:bob".into(),
                    "group:sales".into(),
                    "group:global".into(),
                ],
            };
            r.is_public = i % 5 == 0;
            r
        })
        .collect();

    let stats = upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows).await?;
    assert_eq!(stats.rows_written, 1000);

    let count = ds.row_count().await?;
    assert_eq!(count, 1000);

    // Spot-check a row: content round-trips exactly, ACL is preserved.
    let loaded = load_chunk(&ds, "org-a", "doc-0__2").await?.unwrap();
    assert_eq!(loaded.content, "chunk content number 2");
    assert_eq!(
        loaded.acl,
        vec!["user:alice".to_string(), "group:eng".into()]
    );
    Ok(())
}

/// Business value: retry-safe ingest — replaying the same chunk must
/// not create a duplicate row.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_upsert_chunks_idempotent_same_chunk_id() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let row = ChunkRow::new("org-a", "doc-x", 0, "v1", make_vector(DIM, 42));
    upsert_chunks(
        &ds,
        "org-a",
        IngestionMode::Upsert,
        std::slice::from_ref(&row),
    )
    .await?;
    upsert_chunks(
        &ds,
        "org-a",
        IngestionMode::Upsert,
        std::slice::from_ref(&row),
    )
    .await?;
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &[row]).await?;
    let count = ds.row_count().await?;
    assert_eq!(count, 1, "repeated upsert of the same chunk_id stays at 1");

    // Content update through re-upsert: replaces cleanly.
    let replaced = ChunkRow::new("org-a", "doc-x", 0, "v2", make_vector(DIM, 43));
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &[replaced]).await?;
    let count = ds.row_count().await?;
    assert_eq!(count, 1);
    let loaded = load_chunk(&ds, "org-a", "doc-x__0").await?.unwrap();
    assert_eq!(loaded.content, "v2");
    Ok(())
}

/// Business value: hybrid retrieval has to return documents that are
/// both semantically close AND textually relevant — the whole point of
/// RAG. Here we seed BOTH signals and assert the top hits contain them.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_hybrid_search_returns_relevant_results() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let mut rows: Vec<ChunkRow> = Vec::new();
    // "target" row: contains the query token AND sits near the query vector.
    let mut target = ChunkRow::new(
        "org-a",
        "doc-target",
        0,
        "rhubarb is a stalky vegetable",
        make_vector(DIM, 1),
    );
    target.is_public = true;
    rows.push(target);
    for i in 0..50 {
        let mut r = ChunkRow::new(
            "org-a",
            format!("doc-{i}"),
            0,
            format!("unrelated filler sentence number {i}"),
            make_vector(DIM, (i + 100) as u64),
        );
        r.is_public = true;
        rows.push(r);
    }

    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows).await?;

    let params = SearchParams {
        org_id: "org-a".into(),
        query_text: "rhubarb".into(),
        query_vector: Some(make_vector(DIM, 1)),
        mode: SearchMode::Hybrid,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        doc_updated_after: None,
    };
    let hits = search(&ds, &params).await?;
    assert!(!hits.is_empty(), "expected hits");
    let top_ids: Vec<_> = hits.iter().map(|h| h.doc_id.clone()).collect();
    assert!(
        top_ids.contains(&"doc-target".to_string()),
        "expected doc-target in hybrid top-5, got {top_ids:?}"
    );
    Ok(())
}

/// Business value: tenant isolation. A search scoped to org A must
/// never return rows owned by org B — this is a SECURITY invariant.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_hybrid_search_respects_org_filter() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let mut rows: Vec<ChunkRow> = Vec::new();
    for i in 0..20 {
        let mut r = ChunkRow::new(
            "org-a",
            format!("a-doc-{i}"),
            0,
            format!("alpha content {i}"),
            make_vector(DIM, i as u64),
        );
        r.is_public = true;
        rows.push(r);
    }
    let rows_a = rows.clone();
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows_a).await?;

    let mut rows_b: Vec<ChunkRow> = Vec::new();
    for i in 0..20 {
        let mut r = ChunkRow::new(
            "org-b",
            format!("b-doc-{i}"),
            0,
            format!("alpha content {i}"),
            make_vector(DIM, (i + 1000) as u64),
        );
        r.is_public = true;
        rows_b.push(r);
    }
    upsert_chunks(&ds, "org-b", IngestionMode::Upsert, &rows_b).await?;

    // Search as org-a — zero org-b leakage allowed.
    let params = SearchParams {
        org_id: "org-a".into(),
        query_text: "alpha".into(),
        query_vector: Some(make_vector(DIM, 0)),
        mode: SearchMode::Hybrid,
        acl_any_of: vec![],
        include_public: true,
        limit: 40,
        doc_updated_after: None,
    };
    let hits = search(&ds, &params).await?;
    assert!(!hits.is_empty());
    for h in &hits {
        assert!(
            h.doc_id.starts_with("a-doc-"),
            "tenant leak: org-a search returned {}",
            h.doc_id
        );
    }
    Ok(())
}

/// Business value: core ACL enforcement. Rows with a non-empty ACL and
/// `is_public = false` must only be returned when the caller's
/// `acl_any_of` intersects the row's ACL.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_hybrid_search_respects_acl_filter() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let mut rows = Vec::new();
    let mut r_alice = ChunkRow::new(
        "org-a",
        "doc-alice",
        0,
        "secret only alice can read",
        make_vector(DIM, 1),
    );
    r_alice.acl = vec!["user:alice".into()];
    rows.push(r_alice);

    let mut r_bob = ChunkRow::new(
        "org-a",
        "doc-bob",
        0,
        "secret only bob can read",
        make_vector(DIM, 2),
    );
    r_bob.acl = vec!["user:bob".into()];
    rows.push(r_bob);

    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows).await?;

    // Alice searches.
    let params = SearchParams {
        org_id: "org-a".into(),
        query_text: "secret".into(),
        query_vector: Some(make_vector(DIM, 1)),
        mode: SearchMode::Hybrid,
        acl_any_of: vec!["user:alice".into()],
        include_public: false,
        limit: 10,
        doc_updated_after: None,
    };
    let hits = search(&ds, &params).await?;
    let ids: Vec<_> = hits.iter().map(|h| h.doc_id.as_str()).collect();
    assert!(
        ids.contains(&"doc-alice"),
        "alice should see her own doc, got {ids:?}"
    );
    assert!(
        !ids.contains(&"doc-bob"),
        "alice must NOT see bob's doc (ACL leak), got {ids:?}"
    );
    Ok(())
}

/// Business value: PUBLIC_DOC_PAT semantics. `include_public` must
/// surface public rows even when the caller has no matching ACL tokens.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_hybrid_search_public_docs_always_visible() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let mut pub_row = ChunkRow::new(
        "org-a",
        "doc-pub",
        0,
        "the sky is blue",
        make_vector(DIM, 1),
    );
    pub_row.is_public = true;
    let mut priv_row = ChunkRow::new("org-a", "doc-priv", 0, "confidential", make_vector(DIM, 2));
    priv_row.acl = vec!["user:alice".into()];
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &[pub_row, priv_row]).await?;

    // Stranger with no ACL tokens.
    let params = SearchParams {
        org_id: "org-a".into(),
        query_text: "sky".into(),
        query_vector: Some(make_vector(DIM, 1)),
        mode: SearchMode::Hybrid,
        acl_any_of: vec![],
        include_public: true,
        limit: 10,
        doc_updated_after: None,
    };
    let hits = search(&ds, &params).await?;
    let ids: Vec<_> = hits.iter().map(|h| h.doc_id.as_str()).collect();
    assert!(ids.contains(&"doc-pub"));
    assert!(!ids.contains(&"doc-priv"));
    Ok(())
}

/// Business value: THE OP THAT KILLED THE LANCE-GO SPIKE. An ACL
/// update must rewrite only the `acl` + `is_public` columns — NEVER
/// the vector. We assert BYTE-IDENTICAL preservation by capturing the
/// raw f32 bytes before and after.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_update_acl_does_not_touch_vector() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let mut row = ChunkRow::new("org-a", "doc-x", 0, "body", make_vector(DIM, 7));
    row.acl = vec!["user:old".into()];
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &[row.clone()]).await?;

    let before = vector_bytes_for_chunk(&ds, "org-a", "doc-x__0").await?;
    let before = before.expect("vector should exist pre-update");

    // Apply the perm-sync style update.
    let entry = UpdateAclEntry {
        doc_id: "doc-x".into(),
        acl: vec!["user:new".into(), "group:eng".into()],
        is_public: true,
    };
    let stats = update_acl(&ds, "org-a", &[entry]).await?;
    assert!(stats.chunks_updated >= 1);
    assert_eq!(stats.docs_updated, 1);

    let after = vector_bytes_for_chunk(&ds, "org-a", "doc-x__0").await?;
    let after = after.expect("vector should still exist post-update");

    assert_eq!(
        before.len(),
        after.len(),
        "vector length changed across ACL update"
    );
    assert_eq!(
        before, after,
        "VECTOR BYTES CHANGED across metadata-only update — this was the Phase 0 blocker"
    );

    // Also confirm the ACL actually updated.
    let loaded = load_chunk(&ds, "org-a", "doc-x__0").await?.unwrap();
    assert_eq!(loaded.acl, vec!["user:new".to_string(), "group:eng".into()]);
    assert!(loaded.is_public);
    Ok(())
}

/// Business value: metadata-only update must leave content, metadata,
/// doc_updated_at unchanged. If any of these drift, perm-sync would
/// corrupt indexed text.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_update_acl_preserves_other_fields() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let ts = Utc.with_ymd_and_hms(2025, 6, 1, 12, 0, 0).unwrap();
    let mut meta = BTreeMap::new();
    meta.insert("source".into(), "confluence".into());
    meta.insert("page_id".into(), "ABC-123".into());

    let row = ChunkRow {
        id: "doc-y__3".into(),
        org_id: "org-a".into(),
        doc_id: "doc-y".into(),
        chunk_index: 3,
        content: "original body text".into(),
        title_prefix: Some("Page Title".into()),
        blurb: Some("Short summary".into()),
        vector: make_vector(DIM, 21),
        acl: vec!["user:a".into()],
        is_public: false,
        doc_updated_at: Some(ts),
        metadata: meta.clone(),
    };
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &[row]).await?;

    let entry = UpdateAclEntry {
        doc_id: "doc-y".into(),
        acl: vec!["user:b".into()],
        is_public: false,
    };
    update_acl(&ds, "org-a", &[entry]).await?;

    let loaded = load_chunk(&ds, "org-a", "doc-y__3").await?.unwrap();
    assert_eq!(loaded.content, "original body text");
    assert_eq!(loaded.doc_updated_at, Some(ts));
    assert_eq!(loaded.metadata, meta);
    assert_eq!(loaded.acl, vec!["user:b".to_string()]);
    Ok(())
}

/// Business value: document deletion from Onyx Deletable.delete_single
/// contract (`interfaces.py:251-265`). Every chunk of the doc must
/// disappear — half-deleted docs surface torn content in search.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_delete_by_doc_id_removes_all_chunks() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    // Doc "a": 5 chunks; doc "b": 3 chunks.
    let mut rows = Vec::new();
    for i in 0..5 {
        rows.push(ChunkRow::new(
            "org-a",
            "doc-a",
            i,
            format!("a chunk {i}"),
            make_vector(DIM, i as u64),
        ));
    }
    for i in 0..3 {
        rows.push(ChunkRow::new(
            "org-a",
            "doc-b",
            i,
            format!("b chunk {i}"),
            make_vector(DIM, (i + 100) as u64),
        ));
    }
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows).await?;

    let removed = delete::delete_by_doc_id(&ds, "org-a", &["doc-a".into()]).await?;
    assert_eq!(removed, 5);
    let remaining = ds.row_count().await?;
    assert_eq!(remaining, 3);
    Ok(())
}

/// Business value: GDPR cascade. When an org is deleted from Postgres,
/// the vector store MUST wipe all their chunks — stale tenant data is
/// a regulatory incident.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_delete_by_org_wipes_tenant() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let rows_a: Vec<_> = (0..10)
        .map(|i| {
            ChunkRow::new(
                "org-a",
                format!("a-{i}"),
                0,
                format!("a content {i}"),
                make_vector(DIM, i as u64),
            )
        })
        .collect();
    let rows_b: Vec<_> = (0..10)
        .map(|i| {
            ChunkRow::new(
                "org-b",
                format!("b-{i}"),
                0,
                format!("b content {i}"),
                make_vector(DIM, (i + 100) as u64),
            )
        })
        .collect();
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows_a).await?;
    upsert_chunks(&ds, "org-b", IngestionMode::Upsert, &rows_b).await?;

    let removed = delete::delete_by_org(&ds, "org-a").await?;
    assert_eq!(removed, 10);
    let remaining = ds.row_count().await?;
    assert_eq!(remaining, 10, "org-b's rows must survive");
    Ok(())
}

/// Business value: FTS must actually match exact keywords (the Op 5
/// primitive that failed in the Go spike).
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn test_fts_finds_exact_keyword() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let (ds, _) = DatasetHandle::create_or_open(&h.store, DATASET, DIM).await?;

    let mut rows = Vec::new();
    for (i, body) in [
        "the quick brown fox",
        "a lazy dog slept here",
        "xylophone resounds through the hall",
        "rhubarb crumble is a dessert",
    ]
    .iter()
    .enumerate()
    {
        let mut r = ChunkRow::new(
            "org-a",
            format!("doc-{i}"),
            0,
            body.to_string(),
            make_vector(DIM, i as u64),
        );
        r.is_public = true;
        rows.push(r);
    }
    upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows).await?;

    let params = SearchParams {
        org_id: "org-a".into(),
        query_text: "xylophone".into(),
        query_vector: None,
        mode: SearchMode::Bm25Only,
        acl_any_of: vec![],
        include_public: true,
        limit: 5,
        doc_updated_after: None,
    };
    let hits = search(&ds, &params).await?;
    assert!(!hits.is_empty(), "FTS should find 'xylophone'");
    assert_eq!(hits[0].doc_id, "doc-2");
    Ok(())
}

/// Business value: ingest-path SLO. The proto contract states batches
/// of up to 1000 docs complete in ≤10s. Chunks of 2560-dim vectors
/// exercise the realistic payload shape.
#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
async fn test_large_batch_under_10s() -> Result<()> {
    init_tracing_once();
    let h = minio_harness().await?;
    let name = "rag_chunks__test__d2560";
    let (ds, _) = DatasetHandle::create_or_open(&h.store, name, DIM_LARGE).await?;

    let rows: Vec<ChunkRow> = (0..1000)
        .map(|i| {
            let mut r = ChunkRow::new(
                "org-a",
                format!("doc-{}", i / 5),
                (i % 5) as u32,
                format!("content {i}"),
                make_vector(DIM_LARGE, i as u64),
            );
            r.is_public = true;
            r
        })
        .collect();

    let start = Instant::now();
    let stats = upsert_chunks(&ds, "org-a", IngestionMode::Upsert, &rows).await?;
    let elapsed = start.elapsed();

    assert_eq!(stats.rows_written, 1000);
    assert!(
        elapsed.as_secs_f64() < 10.0,
        "1000x{DIM_LARGE}-dim upsert took {:.2}s — over the 10s SLO",
        elapsed.as_secs_f64()
    );
    Ok(())
}
