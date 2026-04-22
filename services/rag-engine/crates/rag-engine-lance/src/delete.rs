//! Delete operations.
//!
//! * `delete_by_doc_id`  — remove every chunk for a set of doc ids in an org
//! * `delete_by_org`     — nuke an entire tenant's rows in this dataset
//! * `prune`             — delete every doc NOT in the provided keep-set

use crate::dataset::DatasetHandle;
use crate::error::{LanceStoreError, Result};
use crate::filter;
use crate::schema::col;

/// Delete every chunk whose `(org_id, doc_id)` is in the provided set.
/// Returns `0` and is a no-op if `doc_ids` is empty.
pub async fn delete_by_doc_id(
    dataset: &DatasetHandle,
    expected_org_id: &str,
    doc_ids: &[String],
) -> Result<u64> {
    if doc_ids.is_empty() {
        return Ok(0);
    }
    let predicate = filter::and_all(vec![
        filter::eq_str(col::ORG_ID, expected_org_id),
        filter::in_list(col::DOC_ID, doc_ids).expect("non-empty above"),
    ])
    .expect("non-empty");

    // Count before, then delete. LanceDB's `delete` doesn't return a row
    // count, so we run a filtered count_rows first.
    let before = dataset
        .table()
        .count_rows(Some(predicate.clone()))
        .await
        .map_err(LanceStoreError::LanceDb)? as u64;
    dataset
        .table()
        .delete(&predicate)
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(before)
}

/// Delete every chunk owned by `org_id` in this dataset.
pub async fn delete_by_org(dataset: &DatasetHandle, org_id: &str) -> Result<u64> {
    let predicate = filter::eq_str(col::ORG_ID, org_id);
    let before = dataset
        .table()
        .count_rows(Some(predicate.clone()))
        .await
        .map_err(LanceStoreError::LanceDb)? as u64;
    dataset
        .table()
        .delete(&predicate)
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(before)
}

/// Delete every chunk whose `doc_id` is NOT in `keep`. REFUSES an empty
/// `keep` list — the "empty set == delete everything" footgun has bitten
/// people in production.
pub async fn prune(dataset: &DatasetHandle, expected_org_id: &str, keep: &[String]) -> Result<u64> {
    if keep.is_empty() {
        return Err(LanceStoreError::InvalidArgument(
            "prune requires non-empty keep_doc_ids (use delete_by_org to wipe a tenant)".into(),
        ));
    }
    let keep_clause = filter::in_list(col::DOC_ID, keep).expect("non-empty above");
    let predicate = format!(
        "{} AND NOT ({})",
        filter::eq_str(col::ORG_ID, expected_org_id),
        keep_clause
    );
    let before = dataset
        .table()
        .count_rows(Some(predicate.clone()))
        .await
        .map_err(LanceStoreError::LanceDb)? as u64;
    dataset
        .table()
        .delete(&predicate)
        .await
        .map_err(LanceStoreError::LanceDb)?;
    Ok(before)
}
