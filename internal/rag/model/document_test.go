package model_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// bootstrapDocs opens a test DB with the full RAG schema migrated.
// testhelpers.ConnectTestDB calls rag.AutoMigrate internally, which
// creates every rag_* table, index, constraint, and FK this package
// needs.
func bootstrapDocs(t *testing.T) *gorm.DB {
	t.Helper()
	return testhelpers.ConnectTestDB(t)
}

func cleanupDocsForOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		// Document-level rows first (hierarchy nodes FK-set-null into
		// docs; junctions cascade).
		db.Exec(`DELETE FROM rag_document_by_sources WHERE document_id IN (SELECT id FROM rag_documents WHERE org_id = ?)`, orgID)
		db.Exec(`DELETE FROM rag_hierarchy_node_by_sources WHERE hierarchy_node_id IN (SELECT id FROM rag_hierarchy_nodes WHERE org_id = ?)`, orgID)
		db.Exec(`DELETE FROM rag_documents WHERE org_id = ?`, orgID)
		db.Exec(`DELETE FROM rag_hierarchy_nodes WHERE org_id = ?`, orgID)
	})
}

func docID(t *testing.T) string {
	t.Helper()
	return "doc-" + uuid.NewString()
}

func TestRAGDocument_OrgCascadeDelete(t *testing.T) {
	db := bootstrapDocs(t)
	org := testhelpers.NewTestOrg(t, db)

	doc := &ragmodel.RAGDocument{
		ID:           docID(t),
		OrgID:        org.ID,
		SemanticID:   "Cascade Target",
		LastModified: time.Now(),
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Deleting the org must cascade into rag_documents.
	if err := db.Exec(`DELETE FROM orgs WHERE id = ?`, org.ID).Error; err != nil {
		t.Fatalf("delete org: %v", err)
	}

	var found ragmodel.RAGDocument
	err := db.First(&found, "id = ?", doc.ID).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound after org cascade, got err=%v (doc=%+v)", err, found)
	}
}

func TestRAGDocument_ParentHierarchyNodeSetNullOnDelete(t *testing.T) {
	db := bootstrapDocs(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	node := &ragmodel.RAGHierarchyNode{
		OrgID:       org.ID,
		RawNodeID:   "raw-" + uuid.NewString(),
		DisplayName: "Engineering",
		Source:      ragmodel.DocumentSourceGoogleDrive,
		NodeType:    ragmodel.HierarchyNodeTypeSharedDrive,
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	doc := &ragmodel.RAGDocument{
		ID:                    docID(t),
		OrgID:                 org.ID,
		SemanticID:            "Child Doc",
		LastModified:          time.Now(),
		ParentHierarchyNodeID: &node.ID,
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	if err := db.Exec(`DELETE FROM rag_hierarchy_nodes WHERE id = ?`, node.ID).Error; err != nil {
		t.Fatalf("delete node: %v", err)
	}

	var got ragmodel.RAGDocument
	if err := db.First(&got, "id = ?", doc.ID).Error; err != nil {
		t.Fatalf("doc vanished instead of surviving the folder delete: %v", err)
	}
	if got.ParentHierarchyNodeID != nil {
		t.Fatalf("expected parent_hierarchy_node_id to be NULL after folder delete, got %d", *got.ParentHierarchyNodeID)
	}
}

func TestRAGDocument_NeedsSyncPartialIndexExistsAndIsUsed(t *testing.T) {
	db := bootstrapDocs(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	// Part A: the partial index exists with the expected WHERE clause.
	var indexdef string
	if err := db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_rag_document_needs_sync'`,
	).Scan(&indexdef).Error; err != nil {
		t.Fatalf("pg_indexes lookup: %v", err)
	}
	if indexdef == "" {
		t.Fatalf("idx_rag_document_needs_sync not found in pg_indexes")
	}
	// Postgres canonicalises the expression with parens; match the
	// required semantics, not the exact original spelling.
	lower := strings.ToLower(indexdef)
	if !strings.Contains(lower, "last_modified > last_synced") ||
		!strings.Contains(lower, "last_synced is null") {
		t.Fatalf("idx_rag_document_needs_sync has wrong WHERE clause: %s", indexdef)
	}

	// Part B: load enough rows in varied sync states that the planner
	// has reason to prefer the partial index over a seq-scan, then
	// ANALYZE + EXPLAIN.
	now := time.Now()
	older := now.Add(-time.Hour)
	for i := 0; i < 120; i++ {
		var lastSynced *time.Time
		switch i % 3 {
		case 0:
			// needs sync: never synced.
			lastSynced = nil
		case 1:
			// up-to-date.
			cp := now.Add(time.Minute)
			lastSynced = &cp
		case 2:
			// needs sync: synced before last_modified.
			cp := older.Add(-time.Hour)
			lastSynced = &cp
		}
		row := &ragmodel.RAGDocument{
			ID:           fmt.Sprintf("doc-sync-%d-%s", i, uuid.NewString()),
			OrgID:        org.ID,
			SemanticID:   fmt.Sprintf("row-%d", i),
			LastModified: now,
			LastSynced:   lastSynced,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create row %d: %v", i, err)
		}
	}

	if err := db.Exec(`ANALYZE rag_documents`).Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	plan := explainJSON(t, db,
		`SELECT id FROM rag_documents WHERE last_modified > last_synced OR last_synced IS NULL`)
	if !planMentions(plan, "idx_rag_document_needs_sync") {
		t.Fatalf("planner did not use idx_rag_document_needs_sync: %s", plan)
	}
}

func TestRAGDocument_GINIndexOnExternalUserEmails(t *testing.T) {
	db := bootstrapDocs(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	var indexdef string
	if err := db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_rag_document_ext_emails'`,
	).Scan(&indexdef).Error; err != nil {
		t.Fatalf("pg_indexes lookup: %v", err)
	}
	if indexdef == "" {
		t.Fatalf("idx_rag_document_ext_emails not found")
	}
	if !strings.Contains(strings.ToLower(indexdef), "using gin") {
		t.Fatalf("expected GIN method, got: %s", indexdef)
	}

	// Seed enough rows to coax the planner off a seq-scan.
	target := "target-" + uuid.NewString() + "@example.com"
	for i := 0; i < 200; i++ {
		emails := pq.StringArray{
			fmt.Sprintf("alice-%d@example.com", i),
			fmt.Sprintf("bob-%d@example.com", i),
		}
		if i == 42 {
			emails = append(emails, target)
		}
		row := &ragmodel.RAGDocument{
			ID:                 fmt.Sprintf("doc-gin-%d-%s", i, uuid.NewString()),
			OrgID:              org.ID,
			SemanticID:         fmt.Sprintf("row-%d", i),
			LastModified:       time.Now(),
			ExternalUserEmails: emails,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create row %d: %v", i, err)
		}
	}
	if err := db.Exec(`ANALYZE rag_documents`).Error; err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	// The `<arr> @> ARRAY[?]` form is what GIN on array columns
	// naturally optimizes for and is semantically equivalent to
	// `? = ANY(<arr>)` — same filter, better planner affinity.
	// We assert with both to pin behaviour; the GIN index must be
	// used at least in the @> form that production code will use.
	planContains := explainJSON(t, db,
		`SELECT id FROM rag_documents WHERE external_user_emails @> ARRAY[?]::text[]`,
		target)
	if !planMentions(planContains, "idx_rag_document_ext_emails") {
		t.Fatalf("GIN index not used for @> filter: %s", planContains)
	}
}

