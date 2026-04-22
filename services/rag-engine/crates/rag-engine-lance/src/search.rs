//! Retrieval: hybrid vector + BM25 with ACL/org filter.
//!
//! Three modes mirror the proto `SearchMode`:
//!   * Vector-only: `nearest_to(...).only_if(filter)`
//!   * BM25-only:   `full_text_search(...).only_if(filter)`
//!   * Hybrid:      both, fused via Lance's built-in RRF reranker.
//!
//! The ACL filter composes as:
//!
//! ```text
//! org_id = <org> AND
//! (is_public = true OR array_has_any(acl, [...])) AND
//! [optional] doc_updated_at >= <ts>
//! ```
//!
//! Public docs are ALWAYS visible to the requesting org when
//! `include_public` is set — this matches Onyx PUBLIC_DOC_PAT.

use std::collections::BTreeMap;

use arrow_array::cast::AsArray;
use arrow_array::{
    Array, BooleanArray, FixedSizeListArray, Float32Array, Float64Array, ListArray, RecordBatch,
    StringArray, TimestampMillisecondArray, UInt32Array,
};
use chrono::{DateTime, Utc};
use futures::TryStreamExt;
use lance_index::scalar::FullTextSearchQuery;
use lancedb::query::{ExecutableQuery, QueryBase, QueryExecutionOptions};

use crate::dataset::DatasetHandle;
use crate::error::{LanceStoreError, Result};
use crate::filter;
use crate::schema::col;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SearchMode {
    Hybrid,
    VectorOnly,
    Bm25Only,
}

#[derive(Debug, Clone)]
pub struct SearchParams {
    pub org_id: String,
    pub query_text: String,
    /// Required when mode = Hybrid or VectorOnly.
    pub query_vector: Option<Vec<f32>>,
    pub mode: SearchMode,
    /// ACL tokens the caller is a member of. Combined with `include_public`
    /// via an OR. If both are empty, only rows with `is_public = true`
    /// are visible.
    pub acl_any_of: Vec<String>,
    pub include_public: bool,
    pub limit: usize,
    /// Optional recency filter.
    pub doc_updated_after: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone)]
pub struct SearchHit {
    pub chunk_id: String,
    pub doc_id: String,
    pub chunk_index: u32,
    pub content: String,
    pub blurb: Option<String>,
    pub title_prefix: Option<String>,
    pub doc_updated_at: Option<DateTime<Utc>>,
    pub metadata: BTreeMap<String, String>,
    pub score: f64,
}

/// Build the SQL filter fragment for ACL + org + optional recency.
fn build_filter(p: &SearchParams) -> Result<String> {
    let mut clauses: Vec<String> = Vec::new();
    clauses.push(filter::eq_str(col::ORG_ID, &p.org_id));

    // ACL: public OR acl-match.
    let mut acl_sub: Vec<String> = Vec::new();
    if p.include_public {
        acl_sub.push(format!("{} = true", col::IS_PUBLIC));
    }
    if let Some(c) = filter::array_has_any(col::ACL, &p.acl_any_of) {
        acl_sub.push(c);
    }
    if let Some(acl_clause) = filter::or_all(acl_sub) {
        clauses.push(acl_clause);
    } else {
        // Neither acl_any_of nor include_public — the caller gets
        // NOTHING. We encode that explicitly as `1 = 0` so LanceDB
        // short-circuits rather than returning everything.
        clauses.push("1 = 0".to_string());
    }

    if let Some(ts) = p.doc_updated_after {
        clauses.push(format!(
            "{} >= arrow_cast({}, 'Timestamp(Millisecond, Some(\"UTC\"))')",
            col::DOC_UPDATED_AT,
            ts.timestamp_millis()
        ));
    }

    filter::and_all(clauses).ok_or_else(|| LanceStoreError::Internal("empty filter".into()))
}

pub async fn search(dataset: &DatasetHandle, params: &SearchParams) -> Result<Vec<SearchHit>> {
    if params.limit == 0 {
        return Ok(Vec::new());
    }
    if matches!(params.mode, SearchMode::Hybrid | SearchMode::VectorOnly) {
        match &params.query_vector {
            Some(v) if v.len() == dataset.vector_dim() => {}
            Some(v) => {
                return Err(LanceStoreError::VectorDimMismatch {
                    expected: dataset.vector_dim(),
                    got: v.len(),
                });
            }
            None => {
                return Err(LanceStoreError::InvalidArgument(
                    "query_vector required for vector / hybrid search".into(),
                ));
            }
        }
    }
    let filter_sql = build_filter(params)?;
    let table = dataset.table();

    let batches: Vec<RecordBatch> = match params.mode {
        SearchMode::VectorOnly => {
            let vec = params.query_vector.clone().unwrap();
            let stream = table
                .query()
                .only_if(filter_sql.clone())
                .nearest_to(vec)
                .map_err(LanceStoreError::LanceDb)?
                .limit(params.limit)
                .execute()
                .await
                .map_err(LanceStoreError::LanceDb)?;
            stream
                .try_collect()
                .await
                .map_err(LanceStoreError::LanceDb)?
        }
        SearchMode::Bm25Only => {
            let stream = table
                .query()
                .only_if(filter_sql.clone())
                .full_text_search(FullTextSearchQuery::new(params.query_text.clone()))
                .limit(params.limit)
                .execute()
                .await
                .map_err(LanceStoreError::LanceDb)?;
            stream
                .try_collect()
                .await
                .map_err(LanceStoreError::LanceDb)?
        }
        SearchMode::Hybrid => {
            let vec = params.query_vector.clone().unwrap();
            let stream = table
                .query()
                .only_if(filter_sql.clone())
                .full_text_search(FullTextSearchQuery::new(params.query_text.clone()))
                .nearest_to(vec)
                .map_err(LanceStoreError::LanceDb)?
                .limit(params.limit)
                .execute_hybrid(QueryExecutionOptions::default())
                .await
                .map_err(LanceStoreError::LanceDb)?;
            stream
                .try_collect()
                .await
                .map_err(LanceStoreError::LanceDb)?
        }
    };

    let mut hits: Vec<SearchHit> = Vec::new();
    for batch in &batches {
        rows_into_hits(batch, &mut hits)?;
    }
    if hits.len() > params.limit {
        hits.truncate(params.limit);
    }
    Ok(hits)
}

/// Pull hits out of one Lance result batch.
fn rows_into_hits(batch: &RecordBatch, out: &mut Vec<SearchHit>) -> Result<()> {
    let schema = batch.schema();
    let id = batch
        .column(schema.index_of(col::ID)?)
        .as_any()
        .downcast_ref::<StringArray>()
        .ok_or_else(|| LanceStoreError::Internal("id not utf8".into()))?;
    let doc = batch
        .column(schema.index_of(col::DOC_ID)?)
        .as_any()
        .downcast_ref::<StringArray>()
        .ok_or_else(|| LanceStoreError::Internal("doc_id not utf8".into()))?;
    let chunk_idx = batch
        .column(schema.index_of(col::CHUNK_INDEX)?)
        .as_any()
        .downcast_ref::<UInt32Array>()
        .ok_or_else(|| LanceStoreError::Internal("chunk_index not u32".into()))?;
    let content = batch
        .column(schema.index_of(col::CONTENT)?)
        .as_any()
        .downcast_ref::<StringArray>()
        .ok_or_else(|| LanceStoreError::Internal("content not utf8".into()))?;
    let title = batch
        .column(schema.index_of(col::TITLE_PREFIX)?)
        .as_any()
        .downcast_ref::<StringArray>();
    let blurb = batch
        .column(schema.index_of(col::BLURB)?)
        .as_any()
        .downcast_ref::<StringArray>();
    let updated = batch
        .column(schema.index_of(col::DOC_UPDATED_AT)?)
        .as_any()
        .downcast_ref::<TimestampMillisecondArray>();
    let meta_k = batch
        .column(schema.index_of(col::METADATA_KEYS)?)
        .as_any()
        .downcast_ref::<ListArray>();
    let meta_v = batch
        .column(schema.index_of(col::METADATA_VALUES)?)
        .as_any()
        .downcast_ref::<ListArray>();
    // Score column — Lance provides `_score` (BM25 relevance) or `_distance` (vector).
    // For hybrid search the rereanker leaves `_relevance_score` on the batch.
    let score_col = schema
        .fields()
        .iter()
        .position(|f| {
            f.name() == "_relevance_score" || f.name() == "_score" || f.name() == "_distance"
        })
        .and_then(|idx| {
            let arr = batch.column(idx);
            if let Some(a) = arr.as_any().downcast_ref::<Float32Array>() {
                Some(ScoreCol::F32(a.clone()))
            } else {
                arr.as_any()
                    .downcast_ref::<Float64Array>()
                    .map(|a| ScoreCol::F64(a.clone()))
            }
        });

    for i in 0..batch.num_rows() {
        let metadata = match (meta_k, meta_v) {
            (Some(k), Some(v)) if i < k.len() && !k.is_null(i) && !v.is_null(i) => {
                let ks = k.value(i);
                let vs = v.value(i);
                let ks = ks
                    .as_any()
                    .downcast_ref::<StringArray>()
                    .ok_or_else(|| LanceStoreError::Internal("metadata_keys item".into()))?;
                let vs = vs
                    .as_any()
                    .downcast_ref::<StringArray>()
                    .ok_or_else(|| LanceStoreError::Internal("metadata_values item".into()))?;
                let mut m = BTreeMap::new();
                for j in 0..ks.len().min(vs.len()) {
                    m.insert(ks.value(j).to_string(), vs.value(j).to_string());
                }
                m
            }
            _ => BTreeMap::new(),
        };

        out.push(SearchHit {
            chunk_id: id.value(i).to_string(),
            doc_id: doc.value(i).to_string(),
            chunk_index: chunk_idx.value(i),
            content: content.value(i).to_string(),
            blurb: title_or_none(blurb, i),
            title_prefix: title_or_none(title, i),
            doc_updated_at: match updated {
                Some(arr) if !arr.is_null(i) => {
                    DateTime::<Utc>::from_timestamp_millis(arr.value(i))
                }
                _ => None,
            },
            metadata,
            score: score_col.as_ref().map(|s| s.get(i)).unwrap_or(0.0),
        });
    }
    Ok(())
}

fn title_or_none(arr: Option<&StringArray>, i: usize) -> Option<String> {
    match arr {
        Some(a) if !a.is_null(i) => Some(a.value(i).to_string()),
        _ => None,
    }
}

#[derive(Clone)]
enum ScoreCol {
    F32(Float32Array),
    F64(Float64Array),
}

impl ScoreCol {
    fn get(&self, i: usize) -> f64 {
        match self {
            ScoreCol::F32(a) => a.value(i) as f64,
            ScoreCol::F64(a) => a.value(i),
        }
    }
}

/// Read back the vector bytes for a specific `chunk_id`. Used by tests
/// to assert metadata-only updates do not touch the vector column.
///
/// Returns the raw f32 slice rendered as little-endian bytes for
/// byte-identical comparison.
pub async fn vector_bytes_for_chunk(
    dataset: &DatasetHandle,
    org_id: &str,
    chunk_id: &str,
) -> Result<Option<Vec<u8>>> {
    let predicate = filter::and_all(vec![
        filter::eq_str(col::ORG_ID, org_id),
        filter::eq_str(col::ID, chunk_id),
    ])
    .expect("non-empty");
    let stream = dataset
        .table()
        .query()
        .only_if(predicate)
        .limit(1)
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    let batches: Vec<RecordBatch> = stream
        .try_collect()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    for b in batches {
        if b.num_rows() == 0 {
            continue;
        }
        let vcol = b
            .column(b.schema().index_of(col::VECTOR)?)
            .as_any()
            .downcast_ref::<FixedSizeListArray>()
            .ok_or_else(|| LanceStoreError::Internal("vector not FixedSizeList".into()))?;
        let values = vcol.values();
        let f32s = values
            .as_any()
            .downcast_ref::<Float32Array>()
            .ok_or_else(|| LanceStoreError::Internal("vector item not f32".into()))?;
        let dim = vcol.value_length() as usize;
        // Row 0 is what we want (limit(1) above). Vector arrays are laid out as
        // flat f32s of length `num_rows * dim`.
        let mut bytes = Vec::with_capacity(dim * 4);
        for j in 0..dim {
            bytes.extend_from_slice(&f32s.value(j).to_le_bytes());
        }
        return Ok(Some(bytes));
    }
    Ok(None)
}

/// Fetch the full ChunkRow fields (content, metadata, etc.) for a
/// specific chunk_id. Used by tests to validate preservation of
/// non-ACL columns through an update.
pub async fn load_chunk(
    dataset: &DatasetHandle,
    org_id: &str,
    chunk_id: &str,
) -> Result<Option<LoadedChunk>> {
    let predicate = filter::and_all(vec![
        filter::eq_str(col::ORG_ID, org_id),
        filter::eq_str(col::ID, chunk_id),
    ])
    .expect("non-empty");
    let stream = dataset
        .table()
        .query()
        .only_if(predicate)
        .limit(1)
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    let batches: Vec<RecordBatch> = stream
        .try_collect()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    for b in batches {
        if b.num_rows() == 0 {
            continue;
        }
        let content = b
            .column(b.schema().index_of(col::CONTENT)?)
            .as_any()
            .downcast_ref::<StringArray>()
            .unwrap()
            .value(0)
            .to_string();
        let updated = b
            .column(b.schema().index_of(col::DOC_UPDATED_AT)?)
            .as_any()
            .downcast_ref::<TimestampMillisecondArray>()
            .and_then(|arr| {
                if arr.is_null(0) {
                    None
                } else {
                    DateTime::<Utc>::from_timestamp_millis(arr.value(0))
                }
            });
        let is_public = b
            .column(b.schema().index_of(col::IS_PUBLIC)?)
            .as_any()
            .downcast_ref::<BooleanArray>()
            .unwrap()
            .value(0);
        let acl_col = b
            .column(b.schema().index_of(col::ACL)?)
            .as_any()
            .downcast_ref::<ListArray>()
            .unwrap();
        let acl_item = acl_col.value(0);
        let acl_arr = acl_item.as_string::<i32>();
        let mut acl_vals = Vec::with_capacity(acl_arr.len());
        for i in 0..acl_arr.len() {
            if !acl_arr.is_null(i) {
                acl_vals.push(acl_arr.value(i).to_string());
            }
        }

        let meta_k = b
            .column(b.schema().index_of(col::METADATA_KEYS)?)
            .as_any()
            .downcast_ref::<ListArray>()
            .unwrap();
        let meta_v = b
            .column(b.schema().index_of(col::METADATA_VALUES)?)
            .as_any()
            .downcast_ref::<ListArray>()
            .unwrap();
        let mut metadata = BTreeMap::new();
        if !meta_k.is_null(0) && !meta_v.is_null(0) {
            let ks = meta_k.value(0);
            let vs = meta_v.value(0);
            let ks = ks.as_any().downcast_ref::<StringArray>().unwrap();
            let vs = vs.as_any().downcast_ref::<StringArray>().unwrap();
            for j in 0..ks.len().min(vs.len()) {
                metadata.insert(ks.value(j).to_string(), vs.value(j).to_string());
            }
        }

        return Ok(Some(LoadedChunk {
            content,
            acl: acl_vals,
            is_public,
            doc_updated_at: updated,
            metadata,
        }));
    }
    Ok(None)
}

/// Subset of chunk fields used by tests.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LoadedChunk {
    pub content: String,
    pub acl: Vec<String>,
    pub is_public: bool,
    pub doc_updated_at: Option<DateTime<Utc>>,
    pub metadata: BTreeMap<String, String>,
}
