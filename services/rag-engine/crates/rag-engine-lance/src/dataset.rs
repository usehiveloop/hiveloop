//! Dataset lifecycle: create, open, exists, drop.
//!
//! A "dataset" in LanceDB is what Onyx/Vespa would call an "index" — one
//! per (provider, model, dim). The name encodes those; the crate itself
//! is naming-agnostic (it gets a string).

use std::sync::Arc;

use arrow_array::{RecordBatch, RecordBatchIterator};
use arrow_schema::Schema;
use lancedb::Table;

use crate::error::{LanceStoreError, Result};
use crate::indexes;
use crate::schema::{assert_compatible, chunk_schema, vector_dim_of};
use crate::store::LanceStore;

/// A handle to a single dataset (LanceDB table). Cheap to clone.
#[derive(Clone)]
pub struct DatasetHandle {
    #[allow(dead_code)]
    store: LanceStore,
    name: String,
    vector_dim: usize,
    table: Arc<Table>,
}

impl DatasetHandle {
    /// Ensure a dataset exists with the given `(name, vector_dim)`. If
    /// it already exists with a MATCHING schema, returns `(handle,
    /// false)`. If it already exists with a DIFFERENT vector_dim,
    /// returns `SchemaMismatch`. Otherwise creates and returns
    /// `(handle, true)`.
    pub async fn create_or_open(
        store: &LanceStore,
        name: &str,
        vector_dim: usize,
    ) -> Result<(Self, bool)> {
        let conn = store.conn();
        let table_names = conn
            .table_names()
            .execute()
            .await
            .map_err(LanceStoreError::LanceDb)?;

        if table_names.iter().any(|n| n == name) {
            let table = conn
                .open_table(name)
                .execute()
                .await
                .map_err(LanceStoreError::LanceDb)?;
            let existing_schema = table.schema().await.map_err(LanceStoreError::LanceDb)?;
            assert_compatible(&existing_schema, vector_dim)?;
            let actual_dim = vector_dim_of(&existing_schema)?;
            if actual_dim != vector_dim {
                return Err(LanceStoreError::SchemaMismatch(format!(
                    "vector_dim: existing={actual_dim}, requested={vector_dim}"
                )));
            }
            Ok((
                Self {
                    store: store.clone(),
                    name: name.to_string(),
                    vector_dim,
                    table: Arc::new(table),
                },
                false,
            ))
        } else {
            let schema = chunk_schema(vector_dim)?;
            let table = create_empty(conn, name, schema.clone()).await?;
            // Build the scalar and FTS indexes at create time so lookups are
            // fast from the first query. Vector ANN is deferred until ingest
            // exceeds the configured row threshold (built on first search).
            indexes::create_scalar_indexes(&table).await?;
            indexes::create_fts_index(&table).await?;
            Ok((
                Self {
                    store: store.clone(),
                    name: name.to_string(),
                    vector_dim,
                    table: Arc::new(table),
                },
                true,
            ))
        }
    }

    /// Drop the dataset. Safety: the caller is responsible for
    /// establishing consent (the gRPC surface requires `confirm=true`).
    pub async fn drop(store: &LanceStore, name: &str) -> Result<bool> {
        let conn = store.conn();
        let names = conn
            .table_names()
            .execute()
            .await
            .map_err(LanceStoreError::LanceDb)?;
        if !names.iter().any(|n| n == name) {
            return Ok(false);
        }
        conn.drop_table(name, &[])
            .await
            .map_err(LanceStoreError::LanceDb)?;
        Ok(true)
    }

    /// Check whether the dataset exists. Cheap: lists table names.
    pub async fn exists(store: &LanceStore, name: &str) -> Result<bool> {
        let conn = store.conn();
        let names = conn
            .table_names()
            .execute()
            .await
            .map_err(LanceStoreError::LanceDb)?;
        Ok(names.iter().any(|n| n == name))
    }

    pub fn name(&self) -> &str {
        &self.name
    }

    pub fn vector_dim(&self) -> usize {
        self.vector_dim
    }

    pub(crate) fn table(&self) -> &Arc<Table> {
        &self.table
    }

    /// Total number of rows across all orgs in this dataset. Used for
    /// admin-facing metrics and for deciding when the ANN index is
    /// worth building.
    pub async fn row_count(&self) -> Result<u64> {
        let n = self
            .table
            .count_rows(None)
            .await
            .map_err(LanceStoreError::LanceDb)?;
        Ok(n as u64)
    }

    /// Row count for a single org (used to check tenant isolation in tests
    /// and admin reports).
    pub async fn row_count_for_org(&self, org_id: &str) -> Result<u64> {
        let predicate = crate::filter::eq_str(crate::schema::col::ORG_ID, org_id);
        let n = self
            .table
            .count_rows(Some(predicate))
            .await
            .map_err(LanceStoreError::LanceDb)?;
        Ok(n as u64)
    }
}

/// Create an empty Lance table with the given schema.
async fn create_empty(
    conn: &lancedb::Connection,
    name: &str,
    schema: Arc<Schema>,
) -> Result<Table> {
    // Lance requires a batch reader; we hand it an empty one keyed to the
    // target schema. `create_empty_table` would also work but then the
    // validation behavior differs slightly on re-open; this form is more
    // deterministic.
    let empty_batches: Vec<std::result::Result<RecordBatch, arrow_schema::ArrowError>> = Vec::new();
    let reader: Box<dyn arrow_array::RecordBatchReader + Send> =
        Box::new(RecordBatchIterator::new(empty_batches.into_iter(), schema));
    let table = conn
        .create_table(name, reader)
        .execute()
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(table)
}
