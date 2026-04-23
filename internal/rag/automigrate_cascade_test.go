package rag_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// freshMigrate returns a *gorm.DB with the full RAG schema migrated.
//
// We deliberately DO NOT drop tables between tests: parallel test
// packages share the same Postgres test DB, and dropping rag_* tables
// here would race against their in-flight migrations and inserts.
// testhelpers.ConnectTestDB runs model.AutoMigrate + rag.AutoMigrate
// once per connection; if the schema were ever out of shape that
// migration would fail. Schema-shape assertions below use
// information_schema / pg_indexes and row-level assertions are scoped
// to per-test orgs.
func TestAutoMigrate_OrgFullCascadeDelete(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	// RAGDocument
	doc := &ragmodel.RAGDocument{
		ID:           "test-doc-" + uuid.NewString(),
		OrgID:        org.ID,
		SemanticID:   "Test Doc",
		LastModified: time.Now(),
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// RAGHierarchyNode
	node := &ragmodel.RAGHierarchyNode{
		OrgID:       org.ID,
		RawNodeID:   "raw-" + uuid.NewString(),
		DisplayName: "Folder",
		Source:      ragmodel.DocumentSourceGithub,
		NodeType:    ragmodel.HierarchyNodeTypeFolder,
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	// RAGDocumentBySource
	dbc := &ragmodel.RAGDocumentBySource{
		DocumentID:     doc.ID,
		RAGSourceID:    src.ID,
		HasBeenIndexed: false,
	}
	if err := db.Create(dbc).Error; err != nil {
		t.Fatalf("create doc-by-source: %v", err)
	}

	// RAGHierarchyNodeBySource
	hbc := &ragmodel.RAGHierarchyNodeBySource{
		HierarchyNodeID: node.ID,
		RAGSourceID:     src.ID,
	}
	if err := db.Create(hbc).Error; err != nil {
		t.Fatalf("create hier-by-source: %v", err)
	}

	// RAGIndexAttempt + error
	attempt := &ragmodel.RAGIndexAttempt{
		OrgID:       org.ID,
		RAGSourceID: src.ID,
		Status:      ragmodel.IndexingStatusNotStarted,
	}
	if err := db.Create(attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	iaErr := &ragmodel.RAGIndexAttemptError{
		OrgID:          org.ID,
		IndexAttemptID: attempt.ID,
		RAGSourceID:    src.ID,
		FailureMessage: "synthetic failure for cascade test",
	}
	if err := db.Create(iaErr).Error; err != nil {
		t.Fatalf("create attempt error: %v", err)
	}

	// RAGSyncRecord
	sr := &ragmodel.RAGSyncRecord{
		OrgID:         org.ID,
		EntityID:      conn.ID,
		SyncType:      ragmodel.SyncTypePruning,
		SyncStatus:    ragmodel.SyncStatusInProgress,
		SyncStartTime: time.Now(),
	}
	if err := db.Create(sr).Error; err != nil {
		t.Fatalf("create sync record: %v", err)
	}

	// RAGSyncState
	ss := &ragmodel.RAGSyncState{
		OrgID:          org.ID,
		RAGSourceID:    src.ID,
		Status:         ragmodel.RAGConnectionStatusActive,
		AccessType:     ragmodel.AccessTypePrivate,
		ProcessingMode: ragmodel.ProcessingModeRegular,
	}
	if err := db.Create(ss).Error; err != nil {
		t.Fatalf("create sync state: %v", err)
	}

	// RAGSearchSettings — need a real embedding model id from seed.
	settings := &ragmodel.RAGSearchSettings{
		OrgID:              org.ID,
		EmbeddingModelID:   "siliconflow:qwen3-embedding-4b",
		EmbeddingDim:       2560,
		EmbeddingPrecision: ragmodel.EmbeddingPrecisionFloat,
		IndexName:          "rag_chunks__siliconflow_qwen3_embedding_4b__2560",
	}
	if err := db.Create(settings).Error; err != nil {
		t.Fatalf("create search settings: %v", err)
	}

	// RAGExternalUserGroup
	eug := &ragmodel.RAGExternalUserGroup{
		OrgID:               org.ID,
		RAGSourceID:         src.ID,
		ExternalUserGroupID: "github_backend-cascade-" + uuid.NewString(),
		DisplayName:         "Backend",
	}
	if err := db.Create(eug).Error; err != nil {
		t.Fatalf("create external user group: %v", err)
	}

	// RAGUserExternalUserGroup
	uug := &ragmodel.RAGUserExternalUserGroup{
		UserID:              user.ID,
		ExternalUserGroupID: eug.ExternalUserGroupID,
		RAGSourceID:         src.ID,
	}
	if err := db.Create(uug).Error; err != nil {
		t.Fatalf("create user-external-group: %v", err)
	}

	// RAGPublicExternalUserGroup
	pug := &ragmodel.RAGPublicExternalUserGroup{
		ExternalUserGroupID: "github_public-cascade-" + uuid.NewString(),
		RAGSourceID:         src.ID,
	}
	if err := db.Create(pug).Error; err != nil {
		t.Fatalf("create public-external-group: %v", err)
	}

	// RAGExternalIdentity
	ident := &ragmodel.RAGExternalIdentity{
		OrgID:              org.ID,
		UserID:             user.ID,
		RAGSourceID:        src.ID,
		Provider:           "github",
		ExternalUserID:     "gh-" + uuid.NewString(),
		ExternalUserEmails: pq.StringArray{"alice@example.com"},
	}
	if err := db.Create(ident).Error; err != nil {
		t.Fatalf("create external identity: %v", err)
	}

	// Snapshot the src.ID before deleting org — after cascade,
	// rag_sources cascades off org, which cascades into every RAG
	// junction table that keys off rag_source_id.
	srcID := src.ID

	// Hiveloop's own orgs → org_memberships FK does NOT cascade
	// (owned by internal/model, not our concern). Remove memberships
	// first; RAG's own org cascade is what this test is exercising.
	if err := db.Exec("DELETE FROM org_memberships WHERE org_id = ?", org.ID).Error; err != nil {
		t.Fatalf("delete org memberships: %v", err)
	}

	// The bomb: delete the org.
	if err := db.Exec("DELETE FROM orgs WHERE id = ?", org.ID).Error; err != nil {
		t.Fatalf("delete org: %v", err)
	}

	// Now verify every RAG row is gone. Query by org_id where it exists,
	// by rag_source_id where it doesn't.
	checks := []struct {
		table string
		where string
		args  []any
	}{
		{"rag_documents", "org_id = ?", []any{org.ID}},
		{"rag_hierarchy_nodes", "org_id = ?", []any{org.ID}},
		{"rag_document_by_sources", "rag_source_id = ?", []any{srcID}},
		{"rag_hierarchy_node_by_sources", "rag_source_id = ?", []any{srcID}},
		{"rag_index_attempts", "org_id = ?", []any{org.ID}},
		{"rag_index_attempt_errors", "org_id = ?", []any{org.ID}},
		{"rag_sync_records", "org_id = ?", []any{org.ID}},
		{"rag_sync_states", "org_id = ?", []any{org.ID}},
		{"rag_sources", "org_id = ?", []any{org.ID}},
		{"rag_search_settings", "org_id = ?", []any{org.ID}},
		{"rag_external_user_groups", "org_id = ?", []any{org.ID}},
		{"rag_user_external_user_groups", "rag_source_id = ?", []any{srcID}},
		{"rag_public_external_user_groups", "rag_source_id = ?", []any{srcID}},
		{"rag_external_identities", "org_id = ?", []any{org.ID}},
	}
	for _, c := range checks {
		var n int64
		sql := "SELECT COUNT(*) FROM " + c.table + " WHERE " + c.where
		if err := db.Raw(sql, c.args...).Scan(&n).Error; err != nil {
			t.Fatalf("post-delete count on %s: %v", c.table, err)
		}
		if n != 0 {
			t.Errorf("%s: %d row(s) survived org cascade delete", c.table, n)
		}
	}
}

