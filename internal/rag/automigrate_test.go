package rag_test

import (
	"sort"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag"
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
		{"rag_documents", "org_id"}:                            {"orgs", "CASCADE"},
		{"rag_documents", "parent_hierarchy_node_id"}:          {"rag_hierarchy_nodes", "SET NULL"},
		{"rag_hierarchy_nodes", "org_id"}:                      {"orgs", "CASCADE"},
		{"rag_hierarchy_nodes", "document_id"}:                 {"rag_documents", "SET NULL"},
		{"rag_hierarchy_nodes", "parent_id"}:                   {"rag_hierarchy_nodes", "SET NULL"},
		{"rag_document_by_sources", "document_id"}:             {"rag_documents", "CASCADE"},
		{"rag_document_by_sources", "rag_source_id"}:           {"rag_sources", "CASCADE"},
		{"rag_hierarchy_node_by_sources", "hierarchy_node_id"}: {"rag_hierarchy_nodes", "CASCADE"},
		{"rag_hierarchy_node_by_sources", "rag_source_id"}:     {"rag_sources", "CASCADE"},
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
		{"rag_external_identities", "org_id"}:        {"orgs", "CASCADE"},
		{"rag_external_identities", "user_id"}:       {"users", "CASCADE"},
		{"rag_external_identities", "rag_source_id"}: {"rag_sources", "CASCADE"},
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
