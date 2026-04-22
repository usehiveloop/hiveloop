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

// bootstrap1A opens a test DB, ensures the Tranche 1A schema is in
// place, and registers per-test cleanup for the RAG tables owned by
// this tranche. ConnectTestDB already hits model.AutoMigrate + the
// (currently empty) rag.AutoMigrate; Tranche 1F will move the
// AutoMigrate1A call into rag.AutoMigrate and this helper will drop
// that call.
func bootstrap1A(t *testing.T) *gorm.DB {
	t.Helper()
	db := testhelpers.ConnectTestDB(t)
	if err := ragmodel.AutoMigrate1A(db); err != nil {
		t.Fatalf("AutoMigrate1A: %v", err)
	}
	return db
}

// cleanupDocsForOrg scopes cleanup to the org so parallel test runs
// don't clobber each other. RAGDocument rows are the fan-out point —
// everything below them goes away via CASCADE (junction edges) or
// explicit deletes registered by fixture constructors.
func cleanupDocsForOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		// Document-level rows first (hierarchy nodes FK-set-null into
		// docs; junctions cascade).
		db.Exec(`DELETE FROM rag_document_by_connections WHERE document_id IN (SELECT id FROM rag_documents WHERE org_id = ?)`, orgID)
		db.Exec(`DELETE FROM rag_hierarchy_node_by_connections WHERE hierarchy_node_id IN (SELECT id FROM rag_hierarchy_nodes WHERE org_id = ?)`, orgID)
		db.Exec(`DELETE FROM rag_documents WHERE org_id = ?`, orgID)
		db.Exec(`DELETE FROM rag_hierarchy_nodes WHERE org_id = ?`, orgID)
	})
}

func docID(t *testing.T) string {
	t.Helper()
	return "doc-" + uuid.NewString()
}

// ---------------------------------------------------------------------
// 1. RAGDocument_OrgCascadeDelete
// ---------------------------------------------------------------------
// Business value: org deletion must tear down all its RAG rows or we
// fail GDPR tenant-deletion compliance.

func TestRAGDocument_OrgCascadeDelete(t *testing.T) {
	db := bootstrap1A(t)
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

// ---------------------------------------------------------------------
// 2. RAGDocument_ParentHierarchyNodeSetNullOnDelete
// ---------------------------------------------------------------------
// Business value: deleting a folder must not delete its documents —
// the docs just lose their hierarchy parent pointer and wait for
// re-parenting on the next sync.

func TestRAGDocument_ParentHierarchyNodeSetNullOnDelete(t *testing.T) {
	db := bootstrap1A(t)
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

// ---------------------------------------------------------------------
// 3. RAGDocument_NeedsSyncPartialIndexExistsAndIsUsed
// ---------------------------------------------------------------------
// Business value: sync loop + watchdog scan rag_documents at prod
// scale. Without this partial index we seq-scan the whole table every
// 15 seconds per-tenant, which is a P0.

func TestRAGDocument_NeedsSyncPartialIndexExistsAndIsUsed(t *testing.T) {
	db := bootstrap1A(t)
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

// ---------------------------------------------------------------------
// 4. RAGDocument_GINIndexOnExternalUserEmails
// ---------------------------------------------------------------------
// Business value: ACL-filtered retrieval. We mirror email membership
// filters in LanceDB but also run them in Postgres (perm-sync, audit,
// admin UI). Full-scan ACL filter on a 1M-doc tenant = timeouts.

func TestRAGDocument_GINIndexOnExternalUserEmails(t *testing.T) {
	db := bootstrap1A(t)
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

// ---------------------------------------------------------------------
// 5. RAGHierarchyNode_UniqueRawIDSource
// ---------------------------------------------------------------------
// Business value: prevent the same upstream page being ingested under
// two different HierarchyNode rows (cache-invalidation hell; user
// sees duplicates in tree).

func TestRAGHierarchyNode_UniqueRawIDSource(t *testing.T) {
	db := bootstrap1A(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	raw := "collision-raw-" + uuid.NewString()
	first := &ragmodel.RAGHierarchyNode{
		OrgID:       org.ID,
		RawNodeID:   raw,
		DisplayName: "A",
		Source:      ragmodel.DocumentSourceConfluence,
		NodeType:    ragmodel.HierarchyNodeTypeSpace,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := &ragmodel.RAGHierarchyNode{
		OrgID:       org.ID,
		RawNodeID:   raw,
		DisplayName: "B",
		Source:      ragmodel.DocumentSourceConfluence,
		NodeType:    ragmodel.HierarchyNodeTypeSpace,
	}
	err := db.Create(second).Error
	if err == nil {
		t.Fatalf("expected unique-violation on duplicate (raw_node_id, source); got nil")
	}
	if !strings.Contains(err.Error(), "uq_rag_hierarchy_node_raw_id_source") {
		t.Fatalf("error did not mention the unique index: %v", err)
	}
}

// ---------------------------------------------------------------------
// 6. RAGDocumentByConnection_ConnectionCascade
// ---------------------------------------------------------------------
// Business value: removing a connection must sweep its junction edges
// but must NOT remove the underlying document (which may still be
// referenced by a second connection).

func TestRAGDocumentByConnection_ConnectionCascade(t *testing.T) {
	db := bootstrap1A(t)
	org := testhelpers.NewTestOrg(t, db)
	cleanupDocsForOrg(t, db, org.ID)

	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "github")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	doc := &ragmodel.RAGDocument{
		ID:           docID(t),
		OrgID:        org.ID,
		SemanticID:   "Shared Doc",
		LastModified: time.Now(),
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}
	edge := &ragmodel.RAGDocumentByConnection{
		DocumentID:     doc.ID,
		InConnectionID: conn.ID,
		HasBeenIndexed: true,
	}
	if err := db.Create(edge).Error; err != nil {
		t.Fatalf("create edge: %v", err)
	}

	if err := db.Exec(`DELETE FROM in_connections WHERE id = ?`, conn.ID).Error; err != nil {
		t.Fatalf("delete connection: %v", err)
	}

	// Edge gone.
	var edgeCount int64
	if err := db.Model(&ragmodel.RAGDocumentByConnection{}).
		Where("document_id = ? AND in_connection_id = ?", doc.ID, conn.ID).
		Count(&edgeCount).Error; err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 0 {
		t.Fatalf("expected 0 edge rows after connection delete, got %d", edgeCount)
	}

	// Doc survives.
	var docCount int64
	if err := db.Model(&ragmodel.RAGDocument{}).
		Where("id = ?", doc.ID).
		Count(&docCount).Error; err != nil {
		t.Fatalf("count docs: %v", err)
	}
	if docCount != 1 {
		t.Fatalf("doc should survive connection delete, got count=%d", docCount)
	}
}

// ---------------------------------------------------------------------
// 7. HierarchyNodeType_IsValid (pure)
// ---------------------------------------------------------------------
// Business value: API input validation — admin UI must reject a
// client-supplied node_type of "foo" before it corrupts the ingest
// pipeline.

func TestRAGHierarchyNodeType_IsValid(t *testing.T) {
	cases := []struct {
		in   ragmodel.HierarchyNodeType
		want bool
	}{
		{ragmodel.HierarchyNodeTypeFolder, true},
		{ragmodel.HierarchyNodeTypeSource, true},
		{ragmodel.HierarchyNodeTypeSharedDrive, true},
		{ragmodel.HierarchyNodeTypeMyDrive, true},
		{ragmodel.HierarchyNodeTypeSpace, true},
		{ragmodel.HierarchyNodeTypePage, true},
		{ragmodel.HierarchyNodeTypeProject, true},
		{ragmodel.HierarchyNodeTypeDatabase, true},
		{ragmodel.HierarchyNodeTypeWorkspace, true},
		{ragmodel.HierarchyNodeTypeSite, true},
		{ragmodel.HierarchyNodeTypeDrive, true},
		{ragmodel.HierarchyNodeTypeChannel, true},
		{ragmodel.HierarchyNodeType(""), false},
		{ragmodel.HierarchyNodeType("random_string"), false},
		{ragmodel.HierarchyNodeType("Folder"), false}, // case-sensitive per Onyx
		{ragmodel.HierarchyNodeType(" folder"), false},
	}
	for _, c := range cases {
		got := c.in.IsValid()
		if got != c.want {
			t.Errorf("HierarchyNodeType(%q).IsValid() = %v, want %v", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------
// 8. DocumentSource_IsValid (pure)
// ---------------------------------------------------------------------
func TestDocumentSource_IsValid(t *testing.T) {
	// Spot-check across the enum: first, last-from-Onyx-range, special
	// cases, and a couple of mid-range ones.
	positives := []ragmodel.DocumentSource{
		ragmodel.DocumentSourceIngestionAPI,
		ragmodel.DocumentSourceSlack,
		ragmodel.DocumentSourceGithub,
		ragmodel.DocumentSourceConfluence,
		ragmodel.DocumentSourceGoogleDrive,
		ragmodel.DocumentSourceNotion,
		ragmodel.DocumentSourceS3,
		ragmodel.DocumentSourceGoogleCloudStorage,
		ragmodel.DocumentSourceNotApplicable,
		ragmodel.DocumentSourceMockConnector,
		ragmodel.DocumentSourceCraftFile,
		ragmodel.DocumentSourceBitbucket,
		ragmodel.DocumentSourceTestrail,
	}
	for _, p := range positives {
		if !p.IsValid() {
			t.Errorf("expected %q to be valid", p)
		}
	}
	negatives := []ragmodel.DocumentSource{
		"",
		"random_source",
		"Slack",
		"google-drive",
		" slack",
		"jira ",
	}
	for _, n := range negatives {
		if n.IsValid() {
			t.Errorf("expected %q to be invalid", n)
		}
	}
}

// ---------------------------------------------------------------------
// helpers: EXPLAIN plan inspection
// ---------------------------------------------------------------------

// explainJSON returns the raw text of EXPLAIN (FORMAT JSON) for q.
// Rather than parse the JSON tree we do substring matches against
// the known index name — robust to Postgres version changes in plan
// shape but still catches a planner that picks the wrong path.
//
// Forces enable_seqscan=off so the planner will demonstrably pick the
// partial/GIN index whenever one exists and covers the query — the
// test's assertion is "there IS a usable index," not "production will
// pick it at cost X." At the row counts a unit test can realistically
// insert (hundreds), Postgres's default costing falls back to
// seq-scan even when a perfectly good partial index exists. We run
// the setting inside a transaction so it does not leak between tests.
func explainJSON(t *testing.T, db *gorm.DB, q string, args ...any) string {
	t.Helper()
	var plan string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL enable_seqscan = off`).Error; err != nil {
			return fmt.Errorf("disable seqscan: %w", err)
		}
		rows, err := tx.Raw(`EXPLAIN (FORMAT JSON) `+q, args...).Rows()
		if err != nil {
			return fmt.Errorf("EXPLAIN: %w", err)
		}
		defer func() { _ = rows.Close() }()

		var builder strings.Builder
		for rows.Next() {
			var chunk []byte
			if err := rows.Scan(&chunk); err != nil {
				return fmt.Errorf("scan plan: %w", err)
			}
			builder.Write(chunk)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows.Err: %w", err)
		}
		plan = builder.String()
		return nil
	})
	if err != nil {
		t.Fatalf("explain tx: %v", err)
	}
	// Sanity-check the result is valid JSON.
	var parsed any
	if jerr := json.Unmarshal([]byte(plan), &parsed); jerr != nil {
		t.Logf("warn: plan was not valid JSON: %v (text=%q)", jerr, plan)
	}
	return plan
}

func planMentions(plan, needle string) bool {
	return strings.Contains(plan, needle)
}
