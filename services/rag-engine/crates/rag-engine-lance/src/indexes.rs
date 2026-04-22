//! Index builders: scalar BTree/LabelList/Bitmap, FTS tantivy, and
//! vector ANN (IVF_PQ).
//!
//! The FTS + scalar indexes are built at dataset-create time (they're
//! cheap on an empty table). The vector ANN index is deferred until the
//! caller explicitly builds it — typically after reaching a row-count
//! threshold — because IVF_PQ needs non-trivial training data.

use lancedb::index::scalar::{
    BTreeIndexBuilder, BitmapIndexBuilder, FtsIndexBuilder, LabelListIndexBuilder,
};
use lancedb::index::vector::IvfPqIndexBuilder;
use lancedb::index::Index;
use lancedb::Table;

use crate::error::{LanceStoreError, Result};
use crate::schema::col;

/// Build the scalar indexes we rely on for search pushdown.
///
/// - `org_id`, `doc_id`: BTree (equality + range)
/// - `is_public`: Bitmap (2 distinct values)
/// - `acl`: LabelList (supports `array_has` / `array_has_any`)
pub async fn create_scalar_indexes(table: &Table) -> Result<()> {
    table
        .create_index(&[col::ORG_ID], Index::BTree(BTreeIndexBuilder::default()))
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    table
        .create_index(&[col::DOC_ID], Index::BTree(BTreeIndexBuilder::default()))
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    table
        .create_index(
            &[col::IS_PUBLIC],
            Index::Bitmap(BitmapIndexBuilder::default()),
        )
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    table
        .create_index(
            &[col::ACL],
            Index::LabelList(LabelListIndexBuilder::default()),
        )
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(())
}

/// Build the FTS index on the `content` column (tantivy under the hood).
pub async fn create_fts_index(table: &Table) -> Result<()> {
    table
        .create_index(&[col::CONTENT], Index::FTS(FtsIndexBuilder::default()))
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(())
}

/// Build the vector ANN index (IVF_PQ). Call once after ingest crosses
/// the configured row threshold.
///
/// `num_partitions = max(256, rows / 10_000)`; `num_sub_vectors = max(1, dim / 16)`.
/// These heuristics are taken from the Phase 2 plan; the embed layer
/// fixes `dim` per dataset.
pub async fn build_ann_index(table: &Table, dim: usize, row_count: usize) -> Result<()> {
    let num_partitions = std::cmp::max(256, row_count as u32 / 10_000);
    let num_sub_vectors = std::cmp::max(1, (dim / 16) as u32);
    let builder = IvfPqIndexBuilder::default()
        .num_partitions(num_partitions)
        .num_sub_vectors(num_sub_vectors);
    table
        .create_index(&[col::VECTOR], Index::IvfPq(builder))
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(())
}
