//! Metadata-only update — the op that killed the lance-go spike.
//!
//! # Why this file is load-bearing
//!
//! Phase 3's perm-sync fires thousands of ACL updates per org per day.
//! Re-embedding every chunk on every ACL flip is not viable (SiliconFlow
//! charges per token; end-to-end latency would exceed the connector's
//! polling interval). So we need to rewrite ONLY the `acl` +
//! `is_public` columns on matched rows, and leave `vector` / `content`
//! / `doc_updated_at` etc. byte-identical.
//!
//! The Go binding couldn't do this: the Rust FFI only exposed scalar
//! update values, and `list<string>` couldn't even be round-tripped on
//! read. The Rust `lancedb` crate's `UpdateBuilder` takes an arbitrary
//! DataFusion SQL expression per column, so we can emit something like:
//!
//! ```sql
//! SET acl = ['a','b','c'], is_public = true
//! WHERE org_id = '...' AND doc_id = '...'
//! ```
//!
//! The array literal `['a','b','c']` is a DataFusion list constructor.
//! Tests in this crate assert BYTE-IDENTICAL vector preservation across
//! an update, which is the strictest possible preservation contract.

use crate::dataset::DatasetHandle;
use crate::error::{LanceStoreError, Result};
use crate::filter::{self, escape_sql_str};
use crate::schema::col;

/// One pending ACL update: a `doc_id` and the new values to apply to
/// ALL chunks of that doc.
#[derive(Debug, Clone)]
pub struct UpdateAclEntry {
    pub doc_id: String,
    /// Full replacement (not patch) — whatever is in this vec becomes
    /// the new ACL list for every chunk of `doc_id`.
    pub acl: Vec<String>,
    pub is_public: bool,
}

#[derive(Debug, Clone, Default)]
pub struct UpdateAclStats {
    pub docs_updated: u64,
    pub chunks_updated: u64,
}

/// Apply ACL updates to every chunk of the given docs.
///
/// All entries must be for the same `expected_org_id`; callers pass the
/// org explicitly and we include it in the WHERE clause for tenant
/// isolation.
pub async fn update_acl(
    dataset: &DatasetHandle,
    expected_org_id: &str,
    entries: &[UpdateAclEntry],
) -> Result<UpdateAclStats> {
    if entries.is_empty() {
        return Ok(UpdateAclStats::default());
    }
    let mut stats = UpdateAclStats::default();

    // We do per-doc updates because each doc gets a different ACL value.
    // For the same ACL across many docs we could batch with a CASE, but
    // Phase 2's perm-sync caller already hands us per-doc values.
    for e in entries {
        let acl_literal = acl_array_literal(&e.acl);
        let is_public_literal = if e.is_public { "true" } else { "false" };

        let predicate = filter::and_all(vec![
            filter::eq_str(col::ORG_ID, expected_org_id),
            filter::eq_str(col::DOC_ID, &e.doc_id),
        ])
        .expect("non-empty");

        let res = dataset
            .table()
            .update()
            .only_if(predicate)
            .column(col::ACL, acl_literal)
            .column(col::IS_PUBLIC, is_public_literal)
            .execute()
            .await
            .map_err(LanceStoreError::LanceDb)?;

        stats.chunks_updated = stats.chunks_updated.saturating_add(res.rows_updated);
        if res.rows_updated > 0 {
            stats.docs_updated = stats.docs_updated.saturating_add(1);
        }
    }

    Ok(stats)
}

/// Build a DataFusion array literal: `['a', 'b', 'c']`.
///
/// For an empty ACL, we emit `make_array()::list(utf8)` which
/// DataFusion parses as an empty string list (rather than the ambiguous
/// `[]` which can't type-infer).
fn acl_array_literal(values: &[String]) -> String {
    if values.is_empty() {
        // DataFusion cannot infer the element type from an empty `[]`.
        // Cast from a typed empty array produced by make_array over a
        // tiny sentinel and then filtered, OR use the verbose typed
        // construction. The simplest trick that actually parses:
        // arrow_cast('[]', 'List(Utf8)').
        // We use the arrow_cast form because make_array() with no
        // args errors.
        return "arrow_cast(make_array(''), 'List(Utf8)')[0:0]".to_string();
    }
    let items = values
        .iter()
        .map(|v| format!("'{}'", escape_sql_str(v)))
        .collect::<Vec<_>>()
        .join(", ");
    format!("[{items}]")
}
