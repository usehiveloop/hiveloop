//! Batch ingest: rows -> Arrow RecordBatch -> LanceDB `merge_insert`.
//!
//! Two modes:
//!  * `Upsert`  — merge_insert on `id`; existing rows get fully replaced.
//!  * `Reindex` — first delete every existing chunk of each `doc_id`
//!    in the batch, then upsert. Matches the Onyx `Indexable.index`
//!    contract (`interfaces.py:220-226`): "when a document is reindexed
//!    here, it MUST clear all of the existing document chunks first".
//!
//! We explicitly set a HARD UPPER LIMIT of 2000 rows per Arrow batch.
//! The gRPC server layer (2F) will split bigger incoming RPCs into
//! sub-batches before calling us.

use std::collections::HashSet;
use std::sync::Arc;
use std::time::Instant;

use arrow_array::builder::{
    BooleanBuilder, FixedSizeListBuilder, Float32Builder, ListBuilder, StringBuilder,
    TimestampMillisecondBuilder, UInt32Builder,
};
use arrow_array::{RecordBatch, RecordBatchIterator};
use arrow_schema::Schema;

use crate::chunk::ChunkRow;
use crate::dataset::DatasetHandle;
use crate::error::{LanceStoreError, Result};
use crate::filter;
use crate::schema::{chunk_schema, col};

/// Upper bound on how many chunks a single `upsert_chunks` call may
/// process. The gRPC layer (tranche 2F) is the one that enforces/splits
/// incoming requests; this is belt-and-suspenders.
pub const MAX_CHUNKS_PER_CALL: usize = 2000;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum IngestionMode {
    /// Merge on `id`; existing rows get replaced.
    Upsert,
    /// Delete all existing rows with a matching `doc_id` first, then insert.
    Reindex,
}

#[derive(Debug, Clone, Default)]
pub struct IngestStats {
    pub rows_written: u64,
    pub rows_updated: u64,
    pub rows_deleted: u64,
    pub duration_ms: u64,
}

/// Upsert (or reindex) a batch of chunks into `dataset`.
///
/// Validates:
///   * every `vector.len()` matches `dataset.vector_dim()`
///   * row count <= `MAX_CHUNKS_PER_CALL`
///   * every row's `org_id` matches the `expected_org_id` (tenant isolation)
pub async fn upsert_chunks(
    dataset: &DatasetHandle,
    expected_org_id: &str,
    mode: IngestionMode,
    rows: &[ChunkRow],
) -> Result<IngestStats> {
    if rows.is_empty() {
        return Ok(IngestStats::default());
    }
    if rows.len() > MAX_CHUNKS_PER_CALL {
        return Err(LanceStoreError::InvalidArgument(format!(
            "batch size {} exceeds hard limit {}",
            rows.len(),
            MAX_CHUNKS_PER_CALL
        )));
    }

    let dim = dataset.vector_dim();
    for r in rows {
        if r.vector.len() != dim {
            return Err(LanceStoreError::VectorDimMismatch {
                expected: dim,
                got: r.vector.len(),
            });
        }
        if r.org_id != expected_org_id {
            return Err(LanceStoreError::InvalidArgument(format!(
                "chunk org_id `{}` != expected `{}`",
                r.org_id, expected_org_id
            )));
        }
    }

    let started = Instant::now();
    let mut stats = IngestStats::default();

    // Reindex: wipe every doc_id we're about to re-ingest.
    if matches!(mode, IngestionMode::Reindex) {
        let mut doc_ids: Vec<String> = rows
            .iter()
            .map(|r| r.doc_id.clone())
            .collect::<HashSet<_>>()
            .into_iter()
            .collect();
        doc_ids.sort();

        let org_clause = filter::eq_str(col::ORG_ID, expected_org_id);
        if let Some(doc_clause) = filter::in_list(col::DOC_ID, &doc_ids) {
            let predicate = filter::and_all(vec![org_clause, doc_clause]).expect("non-empty");
            dataset
                .table()
                .delete(&predicate)
                .await
                .map_err(LanceStoreError::LanceDb)?;
        }
    }

    let schema = chunk_schema(dim)?;
    let batch = build_record_batch(schema.clone(), rows)?;
    let reader = Box::new(RecordBatchIterator::new(
        vec![Ok(batch)].into_iter(),
        schema,
    ));

    // merge_insert on `id` gives us upsert semantics (insert if new, replace if match).
    let mut merge = dataset.table().merge_insert(&[col::ID]);
    merge
        .when_matched_update_all(None)
        .when_not_matched_insert_all();
    let res = merge
        .execute(reader)
        .await
        .map_err(LanceStoreError::LanceDb)?;

    stats.rows_written = res.num_inserted_rows;
    stats.rows_updated = res.num_updated_rows;
    stats.rows_deleted = res.num_deleted_rows;
    stats.duration_ms = started.elapsed().as_millis() as u64;
    Ok(stats)
}

/// Build a RecordBatch that conforms to `schema` from `rows`.
fn build_record_batch(schema: Arc<Schema>, rows: &[ChunkRow]) -> Result<RecordBatch> {
    let n = rows.len();
    let dim = match schema
        .field_with_name(col::VECTOR)
        .map_err(LanceStoreError::Arrow)?
        .data_type()
    {
        arrow_schema::DataType::FixedSizeList(_, d) => *d as usize,
        _ => unreachable!("schema invariant"),
    };

    let mut id_b = StringBuilder::with_capacity(n, n * 24);
    let mut org_b = StringBuilder::with_capacity(n, n * 36);
    let mut doc_b = StringBuilder::with_capacity(n, n * 24);
    let mut chunk_idx_b = UInt32Builder::with_capacity(n);
    let mut content_b = StringBuilder::with_capacity(n, n * 256);
    let mut title_b = StringBuilder::with_capacity(n, n * 64);
    let mut blurb_b = StringBuilder::with_capacity(n, n * 128);
    let mut vec_b = FixedSizeListBuilder::with_capacity(Float32Builder::new(), dim as i32, n)
        .with_field(Arc::new(arrow_schema::Field::new(
            "item",
            arrow_schema::DataType::Float32,
            true,
        )));
    let mut acl_b = ListBuilder::new(StringBuilder::new()).with_field(Arc::new(
        arrow_schema::Field::new("item", arrow_schema::DataType::Utf8, true),
    ));
    let mut pub_b = BooleanBuilder::with_capacity(n);
    let mut updated_b = TimestampMillisecondBuilder::with_capacity(n).with_data_type(
        arrow_schema::DataType::Timestamp(arrow_schema::TimeUnit::Millisecond, Some("UTC".into())),
    );
    let mut meta_k_b = ListBuilder::new(StringBuilder::new()).with_field(Arc::new(
        arrow_schema::Field::new("item", arrow_schema::DataType::Utf8, true),
    ));
    let mut meta_v_b = ListBuilder::new(StringBuilder::new()).with_field(Arc::new(
        arrow_schema::Field::new("item", arrow_schema::DataType::Utf8, true),
    ));

    for r in rows {
        id_b.append_value(&r.id);
        org_b.append_value(&r.org_id);
        doc_b.append_value(&r.doc_id);
        chunk_idx_b.append_value(r.chunk_index);
        content_b.append_value(&r.content);
        match r.title_prefix.as_deref() {
            Some(s) => title_b.append_value(s),
            None => title_b.append_null(),
        }
        match r.blurb.as_deref() {
            Some(s) => blurb_b.append_value(s),
            None => blurb_b.append_null(),
        }
        for f in &r.vector {
            vec_b.values().append_value(*f);
        }
        vec_b.append(true);
        for tok in &r.acl {
            acl_b.values().append_value(tok);
        }
        acl_b.append(true);
        pub_b.append_value(r.is_public);
        match r.doc_updated_at {
            Some(t) => updated_b.append_value(t.timestamp_millis()),
            None => updated_b.append_null(),
        }
        for (k, v) in &r.metadata {
            meta_k_b.values().append_value(k);
            meta_v_b.values().append_value(v);
        }
        meta_k_b.append(true);
        meta_v_b.append(true);
    }

    let columns: Vec<Arc<dyn arrow_array::Array>> = vec![
        Arc::new(id_b.finish()),
        Arc::new(org_b.finish()),
        Arc::new(doc_b.finish()),
        Arc::new(chunk_idx_b.finish()),
        Arc::new(content_b.finish()),
        Arc::new(title_b.finish()),
        Arc::new(blurb_b.finish()),
        Arc::new(vec_b.finish()),
        Arc::new(acl_b.finish()),
        Arc::new(pub_b.finish()),
        Arc::new(updated_b.finish()),
        Arc::new(meta_k_b.finish()),
        Arc::new(meta_v_b.finish()),
    ];
    RecordBatch::try_new(schema, columns).map_err(LanceStoreError::Arrow)
}
