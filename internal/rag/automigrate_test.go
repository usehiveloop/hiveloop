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
func freshMigrate(t *testing.T) *gorm.DB {
	t.Helper()
	return testhelpers.ConnectTestDB(t)
}

// --------------------------------------------------------------------------
// 1. TestAutoMigrate_CreatesEveryTable
// --------------------------------------------------------------------------

func TestAutoMigrate_CreatesEveryTable(t *testing.T) {
	db := freshMigrate(t)

	var names []string
	if err := db.Raw(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name LIKE 'rag_%'
		ORDER BY table_name
	`).Scan(&names).Error; err != nil {
		t.Fatalf("query rag_* tables: %v", err)
	}

	expected := []string{
		"rag_document_by_sources",
		"rag_documents",
		"rag_embedding_models",
		"rag_external_identities",
		"rag_external_user_groups",
		"rag_hierarchy_node_by_sources",
		"rag_hierarchy_nodes",
		"rag_index_attempt_errors",
		"rag_index_attempts",
		"rag_public_external_user_groups",
		"rag_search_settings",
		"rag_sources",
		"rag_sync_records",
		"rag_sync_states",
		"rag_user_external_user_groups",
	}
	sort.Strings(names)
	sort.Strings(expected)

	if strings.Join(names, ",") != strings.Join(expected, ",") {
		t.Fatalf("rag_* tables mismatch\n  got:      %v\n  expected: %v",
			names, expected)
	}
}

// --------------------------------------------------------------------------
// 2. TestAutoMigrate_CreatesEveryExpectedIndex
// --------------------------------------------------------------------------
//
// Uses a must-exist-set (plus shape assertions for the load-bearing
// partial/GIN indexes). We deliberately accept "extra" indexes that gorm
// may auto-create for FK columns — those are benign. The business-value
// assertion is that the hand-crafted indexes the plan mandates are
// present with the correct shape.

func TestAutoMigrate_CreatesEveryExpectedIndex(t *testing.T) {
	db := freshMigrate(t)

	type idx struct {
		IndexName string
		IndexDef  string
	}
	var rows []idx
	if err := db.Raw(`
		SELECT indexname AS index_name, indexdef AS index_def
		FROM pg_indexes
		WHERE schemaname = 'public' AND tablename LIKE 'rag_%'
	`).Scan(&rows).Error; err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	byName := make(map[string]string, len(rows))
	for _, r := range rows {
		byName[r.IndexName] = r.IndexDef
	}

	// Must-exist set. Every entry is a hand-authored index from the
	// migration suite. If any is missing, a production query (sync
	// loop, watchdog, stale-sweep, scheduler scan) degrades to a
	// sequential scan at scale.
	mustExist := []string{
		// Document + hierarchy
		"idx_rag_document_needs_sync",
		"idx_rag_document_ext_emails",
		"idx_rag_document_ext_group_ids",
		"idx_rag_document_last_modified",
		"idx_rag_document_org",
		"idx_rag_hierarchy_node_org",
		"idx_rag_hierarchy_node_source_type",
		"idx_rag_doc_source_source",
		"idx_rag_doc_source_counts",
		"idx_rag_hier_source_source",
		"uq_rag_hierarchy_node_raw_id_source",
		// Index attempts + sync records
		"idx_rag_index_attempt_latest_for_source",
		"idx_rag_index_attempt_source_model_updated",
		"idx_rag_index_attempt_source_model_poll",
		"idx_rag_index_attempt_active_coord",
		"idx_rag_index_attempt_heartbeat",
		"idx_rag_sync_record_entity_type_start",
		"idx_rag_sync_record_entity_type_status",
		// Sync state
		"idx_rag_sync_state_org_status",
		"idx_rag_sync_state_last_pruned",
		"uq_rag_sync_state_rag_source_id",
		// External groups
		"idx_rag_user_external_group_source_stale",
		"idx_rag_user_external_group_stale",
		"idx_rag_public_external_group_source_stale",
		"idx_rag_public_external_group_stale",
		"uq_rag_external_user_group_source_ext",
		// External identity
		"uq_rag_external_identity_user_source",
		"uq_rag_external_identity_provider_ext_id_org",
		// RAG sources
		"uq_rag_sources_in_connection",
		"idx_rag_sources_org_status",
		"idx_rag_sources_needs_ingest",
		"idx_rag_sources_last_pruned",
	}
	for _, name := range mustExist {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing required index %q", name)
		}
	}

	// Shape assertions for the load-bearing partial/GIN indexes.
	checkContains := func(name, needle string) {
		def, ok := byName[name]
		if !ok {
			return // reported by must-exist loop above
		}
		if !strings.Contains(def, needle) {
			t.Errorf("index %q def missing %q\n  def = %s", name, needle, def)
		}
	}
	// Partial WHERE clauses — Postgres normalises whitespace and adds
	// parens when re-pretty-printing.
	checkContains("idx_rag_document_needs_sync",
		"WHERE ((last_modified > last_synced) OR (last_synced IS NULL))")
	checkContains("idx_rag_document_ext_emails", "USING gin")
	checkContains("idx_rag_document_ext_group_ids", "USING gin")
	checkContains("idx_rag_index_attempt_heartbeat", "WHERE")
	checkContains("idx_rag_index_attempt_heartbeat", "'in_progress'")
}

// --------------------------------------------------------------------------
// 3. TestAutoMigrate_AllFKConstraintsInPlace
// --------------------------------------------------------------------------

type fkRow struct {
	ConstraintName string `gorm:"column:constraint_name"`
	TableName      string `gorm:"column:table_name"`
	ColumnName     string `gorm:"column:column_name"`
	ForeignTable   string `gorm:"column:foreign_table"`
	ForeignColumn  string `gorm:"column:foreign_column"`
	DeleteRule     string `gorm:"column:delete_rule"`
}

func TestAutoMigrate_AllFKConstraintsInPlace(t *testing.T) {
	db := freshMigrate(t)

	var rows []fkRow
	if err := db.Raw(`
		SELECT tc.constraint_name,
		       tc.table_name,
		       kcu.column_name,
		       ccu.table_name  AS foreign_table,
		       ccu.column_name AS foreign_column,
		       rc.delete_rule
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		JOIN information_schema.constraint_column_usage ccu
		  ON tc.constraint_name = ccu.constraint_name
		JOIN information_schema.referential_constraints rc
		  ON tc.constraint_name = rc.constraint_name
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_name LIKE 'rag_%'
	`).Scan(&rows).Error; err != nil {
		t.Fatalf("query FKs: %v", err)
	}

	// Build a (table, column) → (refTable, deleteRule) map.
	type key struct{ table, col string }
	type val struct{ refTable, rule string }
	got := make(map[key]val, len(rows))
	for _, r := range rows {
		got[key{r.TableName, r.ColumnName}] = val{r.ForeignTable, r.DeleteRule}
	}

	expected := map[key]val{
		// Document + hierarchy
		{"rag_documents", "org_id"}:                              {"orgs", "CASCADE"},
		{"rag_documents", "parent_hierarchy_node_id"}:            {"rag_hierarchy_nodes", "SET NULL"},
		{"rag_hierarchy_nodes", "org_id"}:                        {"orgs", "CASCADE"},
		{"rag_hierarchy_nodes", "document_id"}:                   {"rag_documents", "SET NULL"},
		{"rag_hierarchy_nodes", "parent_id"}:                     {"rag_hierarchy_nodes", "SET NULL"},
		{"rag_document_by_sources", "document_id"}:               {"rag_documents", "CASCADE"},
		{"rag_document_by_sources", "rag_source_id"}:             {"rag_sources", "CASCADE"},
		{"rag_hierarchy_node_by_sources", "hierarchy_node_id"}:   {"rag_hierarchy_nodes", "CASCADE"},
		{"rag_hierarchy_node_by_sources", "rag_source_id"}:       {"rag_sources", "CASCADE"},
		// Index attempts + sync records
		{"rag_index_attempts", "org_id"}:                 {"orgs", "CASCADE"},
		{"rag_index_attempts", "rag_source_id"}:          {"rag_sources", "CASCADE"},
		{"rag_index_attempt_errors", "org_id"}:           {"orgs", "CASCADE"},
		{"rag_index_attempt_errors", "rag_source_id"}:    {"rag_sources", "CASCADE"},
		{"rag_index_attempt_errors", "index_attempt_id"}: {"rag_index_attempts", "CASCADE"},
		{"rag_sync_records", "org_id"}:                   {"orgs", "CASCADE"},
		// Sync state + search settings
		{"rag_sync_states", "org_id"}:                 {"orgs", "CASCADE"},
		{"rag_sync_states", "rag_source_id"}:          {"rag_sources", "CASCADE"},
		{"rag_search_settings", "org_id"}:             {"orgs", "CASCADE"},
		{"rag_search_settings", "embedding_model_id"}: {"rag_embedding_models", "RESTRICT"},
		// External groups
		{"rag_external_user_groups", "org_id"}:               {"orgs", "CASCADE"},
		{"rag_external_user_groups", "rag_source_id"}:        {"rag_sources", "CASCADE"},
		{"rag_user_external_user_groups", "user_id"}:         {"users", "CASCADE"},
		{"rag_user_external_user_groups", "rag_source_id"}:   {"rag_sources", "CASCADE"},
		{"rag_public_external_user_groups", "rag_source_id"}: {"rag_sources", "CASCADE"},
		// External identity
		{"rag_external_identities", "org_id"}:         {"orgs", "CASCADE"},
		{"rag_external_identities", "user_id"}:        {"users", "CASCADE"},
		{"rag_external_identities", "rag_source_id"}:  {"rag_sources", "CASCADE"},
		// RAG sources
		{"rag_sources", "org_id"}:           {"orgs", "CASCADE"},
		{"rag_sources", "in_connection_id"}: {"in_connections", "CASCADE"},
		{"rag_sources", "creator_id"}:       {"users", "SET NULL"},
	}

	for k, wantV := range expected {
		gotV, ok := got[k]
		if !ok {
			t.Errorf("missing FK: %s(%s) → ?", k.table, k.col)
			continue
		}
		if gotV.refTable != wantV.refTable {
			t.Errorf("FK %s(%s) references wrong table: got %s, want %s",
				k.table, k.col, gotV.refTable, wantV.refTable)
		}
		if gotV.rule != wantV.rule {
			t.Errorf("FK %s(%s) delete rule: got %s, want %s",
				k.table, k.col, gotV.rule, wantV.rule)
		}
	}
}

// --------------------------------------------------------------------------
// 4. TestAutoMigrate_Idempotent
// --------------------------------------------------------------------------

func TestAutoMigrate_Idempotent(t *testing.T) {
	db := freshMigrate(t)

	// Count indexes before second run.
	countIndexes := func() int {
		var n int64
		if err := db.Raw(`
			SELECT COUNT(*) FROM pg_indexes
			WHERE schemaname='public' AND tablename LIKE 'rag_%'
		`).Scan(&n).Error; err != nil {
			t.Fatalf("count indexes: %v", err)
		}
		return int(n)
	}
	before := countIndexes()

	if err := rag.AutoMigrate(db); err != nil {
		t.Fatalf("second rag.AutoMigrate call errored: %v", err)
	}

	after := countIndexes()
	if before != after {
		t.Fatalf("idempotence broken: index count changed %d → %d", before, after)
	}
}

// --------------------------------------------------------------------------
// 5. TestAutoMigrate_OrgFullCascadeDelete
//
// GDPR-critical: deleting an org must wipe every RAG row that org owns,
// whether directly (org_id FK) or transitively (via in_connection_id FK
// that itself cascades from org).
// --------------------------------------------------------------------------

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

// --------------------------------------------------------------------------
// 6. TestAutoMigrate_SeedsEmbeddingRegistry
// --------------------------------------------------------------------------

func TestAutoMigrate_SeedsEmbeddingRegistry(t *testing.T) {
	db := freshMigrate(t)

	type row struct {
		ID        string
		Provider  string
		Dimension int
	}
	var got []row
	if err := db.Raw(`
		SELECT id, provider, dimension FROM rag_embedding_models
		ORDER BY id
	`).Scan(&got).Error; err != nil {
		t.Fatalf("query rag_embedding_models: %v", err)
	}

	expected := []row{
		{"openai:text-embedding-3-large", "openai", 3072},
		{"openai:text-embedding-3-small", "openai", 1536},
		{"siliconflow:qwen3-embedding-0.6b", "siliconflow", 1024},
		{"siliconflow:qwen3-embedding-4b", "siliconflow", 2560},
		{"siliconflow:qwen3-embedding-8b", "siliconflow", 4096},
	}

	if len(got) != len(expected) {
		t.Fatalf("seed row count: got %d, want %d\n  got=%v", len(got), len(expected), got)
	}
	for i, want := range expected {
		if got[i] != want {
			t.Errorf("row[%d]: got %+v, want %+v", i, got[i], want)
		}
	}
}

// --------------------------------------------------------------------------
// 7. TestAutoMigrate_PartialIndexActuallyUsed_Document
// --------------------------------------------------------------------------

func TestAutoMigrate_PartialIndexActuallyUsed_Document(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)

	now := time.Now()
	past := now.Add(-1 * time.Hour)

	// Insert 100 docs with mixed sync states — only ~1/3 match the
	// partial-index predicate, which matches realistic production skew.
	for i := 0; i < 100; i++ {
		d := &ragmodel.RAGDocument{
			ID:           "partial-doc-" + uuid.NewString(),
			OrgID:        org.ID,
			SemanticID:   "Doc",
			LastModified: now,
		}
		switch i % 3 {
		case 0:
			// last_synced = NULL → matches predicate
			d.LastSynced = nil
		case 1:
			// last_synced in the past → last_modified > last_synced → matches
			d.LastSynced = &past
		case 2:
			// last_synced = now → last_modified == last_synced → misses
			d.LastSynced = &now
		}
		if err := db.Create(d).Error; err != nil {
			t.Fatalf("create doc %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_documents").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	plan := explainNoSeqScan(t, db, `
		EXPLAIN (FORMAT JSON)
		SELECT id FROM rag_documents
		WHERE last_modified > last_synced OR last_synced IS NULL
	`)
	if !strings.Contains(plan, "idx_rag_document_needs_sync") {
		t.Fatalf("planner did not use idx_rag_document_needs_sync; plan=%s", plan)
	}
}

// --------------------------------------------------------------------------
// 8. TestAutoMigrate_PartialIndexActuallyUsed_Watchdog
// --------------------------------------------------------------------------

func TestAutoMigrate_PartialIndexActuallyUsed_Watchdog(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)
	src := testhelpers.NewTestRAGSource(t, db, org.ID, conn.ID)

	stale := time.Now().Add(-1 * time.Hour)
	fresh := time.Now()

	// 1000 rows with ~5% in-progress. At that skew the partial
	// heartbeat index (size ~= 50 entries) is noticeably smaller than
	// the full-table status index (size = 1000 entries), so the planner
	// reliably picks it once enable_seqscan is off.
	for i := 0; i < 1000; i++ {
		a := &ragmodel.RAGIndexAttempt{
			OrgID:       org.ID,
			RAGSourceID: src.ID,
			Status:      ragmodel.IndexingStatusSuccess, // majority branch, misses partial
		}
		if i%20 == 0 {
			a.Status = ragmodel.IndexingStatusInProgress
			if i%40 == 0 {
				a.LastProgressTime = &stale
			} else {
				a.LastProgressTime = &fresh
			}
		}
		if err := db.Create(a).Error; err != nil {
			t.Fatalf("create attempt %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_index_attempts").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	plan := explainNoSeqScan(t, db, `
		EXPLAIN (FORMAT JSON)
		SELECT id FROM rag_index_attempts
		WHERE status = 'in_progress'
		  AND last_progress_time < NOW() - INTERVAL '30 minutes'
	`)
	if !strings.Contains(plan, "idx_rag_index_attempt_heartbeat") {
		t.Fatalf("planner did not use idx_rag_index_attempt_heartbeat; plan=%s", plan)
	}
}

// --------------------------------------------------------------------------
// 9. TestAutoMigrate_GINIndexActuallyUsed
// --------------------------------------------------------------------------

func TestAutoMigrate_GINIndexActuallyUsed(t *testing.T) {
	db := freshMigrate(t)

	org := testhelpers.NewTestOrg(t, db)

	for i := 0; i < 100; i++ {
		emails := pq.StringArray{}
		if i%5 == 0 {
			emails = pq.StringArray{"alice@example.com"}
		} else {
			emails = pq.StringArray{"bob-" + uuid.NewString() + "@example.com"}
		}
		d := &ragmodel.RAGDocument{
			ID:                 "gin-doc-" + uuid.NewString(),
			OrgID:              org.ID,
			SemanticID:         "Doc",
			LastModified:       time.Now(),
			ExternalUserEmails: emails,
		}
		if err := db.Create(d).Error; err != nil {
			t.Fatalf("create doc %d: %v", i, err)
		}
	}
	if err := db.Exec("ANALYZE rag_documents").Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	// The plan agent noted @> is the GIN-preferred operator; ANY(...)
	// doesn't use GIN. We use the production shape here.
	plan := explainNoSeqScan(t, db, `
		EXPLAIN (FORMAT JSON)
		SELECT id FROM rag_documents
		WHERE external_user_emails @> ARRAY['alice@example.com']::text[]
	`)
	if !strings.Contains(plan, "idx_rag_document_ext_emails") {
		t.Fatalf("planner did not use idx_rag_document_ext_emails; plan=%s", plan)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// explainNoSeqScan runs the given EXPLAIN-wrapped query inside a tx with
// enable_seqscan disabled — with <1000 rows the planner may still prefer
// seq scan even when the index is valid. Disabling seq scan forces the
// planner to use the best available index, which is the correct business
// assertion: "when the query is forced through the indexed path, the
// right index is chosen". If no usable index exists, the plan still
// reports Seq Scan (enable_seqscan is a soft discouragement, not a
// hard ban).
func explainNoSeqScan(t *testing.T, db *gorm.DB, sql string) string {
	t.Helper()

	var planJSON string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL enable_seqscan = off").Error; err != nil {
			return err
		}
		var plans []struct {
			QueryPlan string `gorm:"column:QUERY PLAN"`
		}
		if err := tx.Raw(sql).Scan(&plans).Error; err != nil {
			return err
		}
		if len(plans) == 0 {
			return nil
		}
		planJSON = plans[0].QueryPlan
		return nil
	})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	return planJSON
}

