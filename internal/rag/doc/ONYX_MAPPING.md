# Onyx → Hiveloop symbol mapping

A living table. Phase 0 seeds the data-layer and scaffolding rows;
later phases append search, embedding, pipeline, task, and
connector-level mappings as they land.

When reading Onyx source, assume the root is
`/Users/bahdcoder/code/onyx/` and paths below are relative to that.

---

## Data layer

| Onyx symbol | Onyx path:line | Hiveloop equivalent | Notes |
|---|---|---|---|
| `Document` | `backend/onyx/db/models.py:939-1063` | `internal/rag/model/document.go` `RAGDocument` | DEVIATION: add `OrgID`; drop schema-per-tenant |
| `HierarchyNode` | `backend/onyx/db/models.py:839-936` | `internal/rag/model/hierarchy_node.go` `RAGHierarchyNode` | verbatim field port |
| `DocumentByConnectorCredentialPair` | `backend/onyx/db/models.py:2512-2558` | `internal/rag/model/document_by_connection.go` `RAGDocumentByConnection` | key changed from `(doc, connector, credential)` to `(doc, in_connection)` |
| `HierarchyNodeByConnectorCredentialPair` | `backend/onyx/db/models.py:2480-2510` | `internal/rag/model/hierarchy_node_by_connection.go` `RAGHierarchyNodeByConnection` | same key change |
| `IndexAttempt` | `backend/onyx/db/models.py:2189-2343` | `internal/rag/model/index_attempt.go` `RAGIndexAttempt` | `SearchSettings` FK → `EmbeddingModel` FK |
| `IndexAttemptError` | `backend/onyx/db/models.py:2399-2438` | `internal/rag/model/index_attempt_error.go` `RAGIndexAttemptError` | verbatim |
| `SyncRecord` | `backend/onyx/db/models.py:2440-2478` | `internal/rag/model/sync_record.go` `RAGSyncRecord` | subset: drops `DOCUMENT_SET`, `USER_GROUP` sync types |
| `ConnectorCredentialPair` | `backend/onyx/db/models.py:723-837` | split across three tables | `InConnection` (identity), `RAGSyncState` (runtime), `RAGConnectionConfig` (schedule) |
| `Connector.refresh_freq / prune_freq` | `backend/onyx/db/models.py:1886-1890` | `RAGConnectionConfig.RefreshFreqSeconds / PruneFreqSeconds` | + Hiveloop additions `PermSyncFreqSeconds`, `ExternalGroupSyncFreqSeconds` |
| `SearchSettings` | `backend/onyx/db/models.py:2052-2187` | `internal/rag/model/search_settings.go` `RAGSearchSettings` | DEVIATION: per-org, drops live-switchover machinery |
| `User__ExternalUserGroupId` | `backend/onyx/db/models.py:4320-4350` | `internal/rag/model/user_external_user_group.go` `RAGUserExternalUserGroup` | verbatim |
| `PublicExternalUserGroup` | `backend/onyx/db/models.py:4352-4380` | `internal/rag/model/public_external_user_group.go` `RAGPublicExternalUserGroup` | verbatim |

## Enums

| Onyx enum | Onyx path:line | Hiveloop equivalent |
|---|---|---|
| `DocumentSource` | `backend/onyx/configs/constants.py:205-262` | `internal/rag/model/enums.go` `DocumentSource` |
| `HierarchyNodeType` | `backend/onyx/db/enums.py:306-340` | `internal/rag/model/enums.go` `HierarchyNodeType` |
| `IndexingStatus` | `backend/onyx/db/enums.py:38-62` | `internal/rag/model/enums.go` `IndexingStatus` (with `IsTerminal`, `IsSuccessful`) |
| `IndexingMode` | `backend/onyx/db/enums.py:88-91` | `internal/rag/model/enums.go` `IndexingMode` |
| `SyncType` | `backend/onyx/db/enums.py:101-111` | `internal/rag/model/enums.go` `SyncType` — **subset only** |
| `SyncStatus` | `backend/onyx/db/enums.py:113-127` | `internal/rag/model/enums.go` `SyncStatus` (with `IsTerminal`) |
| `ConnectorCredentialPairStatus` | `backend/onyx/db/enums.py:180-205` | `internal/rag/model/enums.go` `RAGConnectionStatus` |
| `AccessType` | `backend/onyx/db/enums.py:207-211` | `internal/rag/model/enums.go` `AccessType` |
| `ProcessingMode` | `backend/onyx/db/enums.py:93-98` | `internal/rag/model/enums.go` `ProcessingMode` |
| `EmbeddingPrecision` | `backend/onyx/db/enums.py:213-241` | `internal/rag/model/enums.go` `EmbeddingPrecision` |

## Access / ACL

| Onyx symbol | Onyx path:line | Hiveloop equivalent |
|---|---|---|
| `prefix_user_email` | `backend/onyx/access/utils.py` | `internal/rag/acl/prefix.go` |
| `prefix_user_group` | `backend/onyx/access/utils.py` | `internal/rag/acl/prefix.go` |
| `prefix_external_group` | `backend/onyx/access/utils.py` | `internal/rag/acl/prefix.go` |
| `build_ext_group_name_for_onyx` | `backend/onyx/access/utils.py` | `internal/rag/acl/prefix.go` |
| `PUBLIC_DOC_PAT` | `backend/onyx/configs/constants.py:27` | `internal/rag/acl/prefix.go` `PublicDocPat` const |
| `DocumentAccess.to_acl` | `backend/onyx/access/models.py:174-197` | `internal/rag/acl` ACL builder |

## Infrastructure

| Onyx concern | Onyx path | Hiveloop equivalent | Notes |
|---|---|---|---|
| Vector index | `backend/onyx/document_index/vespa/` (Vespa) | `internal/rag/vectorstore/` (LanceDB) | substitution per plan |
| Filestore | `backend/onyx/file_store/` | `internal/rag/filestore/` | same S3 backend as LanceDB |
| Indexing pipeline | `backend/onyx/indexing/indexing_pipeline.py` | `internal/rag/pipeline/` | Phase 3 |
| Embedder | `backend/onyx/indexing/embedder.py` | `internal/rag/embedder/` | Phase 2C — only mockable component |
| Chunker | `backend/onyx/indexing/chunking/` | `internal/rag/chunker/` | Phase 2 |
| Background tasks | `backend/onyx/background/celery/tasks/` | `internal/rag/tasks/` | Celery → asynq |
| Search pipeline | `backend/onyx/context/search/pipeline.py` | `internal/rag/search/` | scope limited to retrieval |
| Redis locks (scattered) | across `backend/onyx/background/celery/tasks/` | `internal/rag/locks/` | centralized in Hiveloop |
| User / auth | `backend/onyx/db/models.py:User` | `internal/model/User` + `OrgMembership` | existing Hiveloop model |
| External identity linking | `backend/onyx/db/users.py` helpers | `internal/rag/identity/` + extending `OAuthAccount` | Phase 1E |

## Not ported (explicit non-goals)

| Onyx symbol | Reason |
|---|---|
| `UserGroup` (EE) | Hiveloop already has `OrgMembership.Role`; doc-level ACLs are a separate axis |
| `DocumentSet` | Not part of Phase 1 scope |
| `Persona`, `Tool` (agent surface) | Hiveloop has its own agent subsystem |
| Schema-per-tenant middleware | `org_id` column is sufficient |
| Live embedder-switchover workflow | One model per org for the lifetime of its index |
