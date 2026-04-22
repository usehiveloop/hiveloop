//! Arrow schema builder for a chunk row, keyed by vector dimension.
//!
//! One (provider, model, dim) triple -> one dataset. We NEVER mix
//! dimensionalities in a single dataset.
//!
//! Schema (from the plan):
//! ```text
//! id: utf8 (non-null)
//! org_id: utf8 (non-null)
//! doc_id: utf8 (non-null)
//! chunk_index: uint32 (non-null)
//! content: utf8 (non-null, FTS-indexed)
//! title_prefix: utf8 (nullable)
//! blurb: utf8 (nullable)
//! vector: fixed_size_list<float32, DIM> (non-null)
//! acl: list<utf8> (non-null; may be empty)
//! is_public: bool (non-null)
//! doc_updated_at: timestamp<millis, UTC> (nullable)
//! metadata_keys: list<utf8> (non-null; parallel to values)
//! metadata_values: list<utf8> (non-null)
//! ```
//!
//! NOTE: metadata is encoded as two parallel `list<utf8>` columns rather
//! than a `map` type. LanceDB's Arrow map support through the SQL filter
//! layer is not as mature as list support (and we don't filter on
//! metadata in 2B), so we keep it simple. Reads rehydrate to a BTreeMap.

use std::sync::Arc;

use arrow_schema::{DataType, Field, Schema, TimeUnit};

use crate::error::{LanceStoreError, Result};

/// Column names used throughout the crate. Centralized so typos are caught at
/// compile time and so future renames are local.
pub mod col {
    pub const ID: &str = "id";
    pub const ORG_ID: &str = "org_id";
    pub const DOC_ID: &str = "doc_id";
    pub const CHUNK_INDEX: &str = "chunk_index";
    pub const CONTENT: &str = "content";
    pub const TITLE_PREFIX: &str = "title_prefix";
    pub const BLURB: &str = "blurb";
    pub const VECTOR: &str = "vector";
    pub const ACL: &str = "acl";
    pub const IS_PUBLIC: &str = "is_public";
    pub const DOC_UPDATED_AT: &str = "doc_updated_at";
    pub const METADATA_KEYS: &str = "metadata_keys";
    pub const METADATA_VALUES: &str = "metadata_values";
}

/// Build the Arrow schema for a chunk dataset with the given vector dim.
pub fn chunk_schema(vector_dim: usize) -> Result<Arc<Schema>> {
    if vector_dim == 0 || vector_dim > 8192 {
        return Err(LanceStoreError::InvalidArgument(format!(
            "vector_dim must be in 1..=8192, got {vector_dim}"
        )));
    }

    let vector_field = Field::new(
        col::VECTOR,
        DataType::FixedSizeList(
            Arc::new(Field::new("item", DataType::Float32, true)),
            vector_dim as i32,
        ),
        false,
    );

    let acl_field = Field::new(
        col::ACL,
        DataType::List(Arc::new(Field::new("item", DataType::Utf8, true))),
        false,
    );

    let metadata_keys = Field::new(
        col::METADATA_KEYS,
        DataType::List(Arc::new(Field::new("item", DataType::Utf8, true))),
        false,
    );
    let metadata_values = Field::new(
        col::METADATA_VALUES,
        DataType::List(Arc::new(Field::new("item", DataType::Utf8, true))),
        false,
    );

    Ok(Arc::new(Schema::new(vec![
        Field::new(col::ID, DataType::Utf8, false),
        Field::new(col::ORG_ID, DataType::Utf8, false),
        Field::new(col::DOC_ID, DataType::Utf8, false),
        Field::new(col::CHUNK_INDEX, DataType::UInt32, false),
        Field::new(col::CONTENT, DataType::Utf8, false),
        Field::new(col::TITLE_PREFIX, DataType::Utf8, true),
        Field::new(col::BLURB, DataType::Utf8, true),
        vector_field,
        acl_field,
        Field::new(col::IS_PUBLIC, DataType::Boolean, false),
        Field::new(
            col::DOC_UPDATED_AT,
            DataType::Timestamp(TimeUnit::Millisecond, Some("UTC".into())),
            true,
        ),
        metadata_keys,
        metadata_values,
    ])))
}

/// Extract the fixed vector dimension from a schema built by [`chunk_schema`].
pub fn vector_dim_of(schema: &Schema) -> Result<usize> {
    let field = schema
        .field_with_name(col::VECTOR)
        .map_err(|_| LanceStoreError::SchemaMismatch("missing `vector` column".into()))?;
    match field.data_type() {
        DataType::FixedSizeList(_, dim) => Ok(*dim as usize),
        other => Err(LanceStoreError::SchemaMismatch(format!(
            "`vector` must be FixedSizeList<f32, N>, got {other:?}"
        ))),
    }
}

/// Assert that the existing dataset's schema matches what we'd build for
/// the given `vector_dim`. We do this at `open_or_create` time.
pub fn assert_compatible(existing: &Schema, vector_dim: usize) -> Result<()> {
    let want = chunk_schema(vector_dim)?;
    if existing.fields().len() != want.fields().len() {
        return Err(LanceStoreError::SchemaMismatch(format!(
            "field count: existing={}, want={}",
            existing.fields().len(),
            want.fields().len()
        )));
    }
    // Check each field by name + data_type + nullability.
    for want_field in want.fields().iter() {
        let got = existing.field_with_name(want_field.name()).map_err(|_| {
            LanceStoreError::SchemaMismatch(format!(
                "missing column `{}` in existing dataset",
                want_field.name()
            ))
        })?;
        if got.data_type() != want_field.data_type() {
            return Err(LanceStoreError::SchemaMismatch(format!(
                "column `{}`: existing type {:?} != wanted {:?}",
                want_field.name(),
                got.data_type(),
                want_field.data_type()
            )));
        }
    }
    Ok(())
}
