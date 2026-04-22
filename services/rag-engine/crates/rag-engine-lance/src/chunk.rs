//! A single chunk row — the materialized unit we write to LanceDB.
//!
//! Mirrors the schema documented in [`crate::schema`] and, via the gRPC
//! layer, the `SearchHit` / `DocumentToIngest` messages in the proto.

use std::collections::BTreeMap;

use chrono::{DateTime, Utc};

/// One chunk's worth of data. All fields are OPAQUE to this crate —
/// ACL tokens, org ids, etc. are strings we store and filter on.
#[derive(Debug, Clone)]
pub struct ChunkRow {
    /// `"<doc_id>__<chunk_index>"` — unique per (dataset, doc, chunk).
    pub id: String,
    pub org_id: String,
    pub doc_id: String,
    pub chunk_index: u32,
    pub content: String,
    pub title_prefix: Option<String>,
    pub blurb: Option<String>,
    /// Dense vector, `dim` floats; the dataset schema enforces `dim`.
    pub vector: Vec<f32>,
    /// Opaque ACL tokens (e.g. `user_email:a@b.c`, `group:eng`).
    /// May be empty. When empty, only rows where `is_public` is true can
    /// be retrieved by a non-matching searcher.
    pub acl: Vec<String>,
    pub is_public: bool,
    pub doc_updated_at: Option<DateTime<Utc>>,
    /// String-string metadata. Typed metadata lives in Postgres.
    pub metadata: BTreeMap<String, String>,
}

impl ChunkRow {
    /// Convenience constructor; builds `id` from `(doc_id, chunk_index)`.
    pub fn new(
        org_id: impl Into<String>,
        doc_id: impl Into<String>,
        chunk_index: u32,
        content: impl Into<String>,
        vector: Vec<f32>,
    ) -> Self {
        let doc_id = doc_id.into();
        let id = format!("{}__{}", doc_id, chunk_index);
        Self {
            id,
            org_id: org_id.into(),
            doc_id,
            chunk_index,
            content: content.into(),
            title_prefix: None,
            blurb: None,
            vector,
            acl: Vec::new(),
            is_public: false,
            doc_updated_at: None,
            metadata: BTreeMap::new(),
        }
    }
}
